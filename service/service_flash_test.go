// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

const testDistribution = "debian"

type flashHTTP struct {
	archive          []byte
	archiveErr       error
	manifestErr      error
	manifestRequests atomic.Int64
	archiveRequests  atomic.Int64
}

func (h *flashHTTP) release() registry.Release {
	sum := sha256.Sum256(h.archive)
	return registry.Release{
		Board: registry.UnoQ.ID, OS: testDistribution, Version: "20250915-173",
		Url: "https://images.example/image.tar", FileName: "image.tar",
		Sha256: hex.EncodeToString(sum[:]),
	}
}

func (h *flashHTTP) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	status := http.StatusOK
	var data []byte
	switch req.URL.String() {
	case "https://downloads.arduino.cc/debian-im/Stable/info.json":
		h.manifestRequests.Add(1)
		if h.manifestErr != nil {
			return nil, h.manifestErr
		}
		rel := h.release()
		var err error
		data, err = json.Marshal(map[string]any{
			"releases": []map[string]string{{
				"board": registry.UnoQ.Label, "os": "Debian GNU/Linux 13 (trixie)",
				"version": rel.Version, "url": rel.Url, "sha256": rel.Sha256,
			}},
		})
		if err != nil {
			return nil, err
		}
	case "https://downloads.arduino.cc/ubuntu-im/Stable/info.json":
		h.manifestRequests.Add(1)
		status = http.StatusNotFound
	case "https://images.example/image.tar":
		h.archiveRequests.Add(1)
		if h.archiveErr != nil {
			return nil, h.archiveErr
		}
		data = h.archive
	default:
		return nil, fmt.Errorf("unexpected HTTP request: %s", req.URL)
	}
	length := int64(len(data))
	if req.Method == http.MethodHead {
		data = nil
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header: http.Header{
			"Content-Length": []string{strconv.FormatInt(length, 10)},
		},
		ContentLength: length,
		Body:          io.NopCloser(bytes.NewReader(data)),
		Request:       req,
	}, nil
}

type flashStream struct {
	grpc.ServerStream
	ctx       context.Context
	responses []*flasher.FlashResponse
}

func (s *flashStream) Context() context.Context { return s.ctx }

func (s *flashStream) Send(response *flasher.FlashResponse) error {
	s.responses = append(s.responses, response)
	return nil
}

type flashFixture struct {
	server     *flasherServerImpl
	http       *flashHTTP
	imageStore *store.Store
	request    *flasher.FlashRequest
	stream     *flashStream
	flashCalls atomic.Int64
	flashErr   error
	extracted  string
}

func imageArchive(t *testing.T, payload string) []byte {
	t.Helper()
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "image/payload.txt", Mode: 0o600, Size: int64(len(payload))}))
	_, err := w.Write([]byte(payload))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return archive.Bytes()
}

func setupFlash(t *testing.T) *flashFixture {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", t.TempDir())
	case "darwin":
		t.Setenv("HOME", t.TempDir())
	default:
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
	}
	t.Setenv("ARDUINO_FLASHER_ADDITIONAL_URLS", "")
	t.Setenv("ARDUINO_FLASHER_ADDITIONAL_HEADERS", "")
	h := &flashHTTP{archive: imageArchive(t, "test payload")}
	original := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: h}
	t.Cleanup(func() { http.DefaultClient = original })
	c, err := store.Default()
	require.NoError(t, err)
	f := &flashFixture{
		server: NewFlasherServer().(*flasherServerImpl), http: h, imageStore: c,
		request: &flasher.FlashRequest{
			Board: registry.UnoQ.ID, Os: testDistribution, Version: h.release().Version,
			TempPath: t.TempDir(), Serial: "305419896", PreserveUser: true,
		},
		stream: &flashStream{ctx: context.Background()},
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.request.TempPath, "keep"), []byte("parent content"), 0o600))
	// Replace only the hardware boundary. Downloading, validation, extraction,
	// progress and cleanup all execute the real service implementation.
	f.server.flashBoard = func(ctx context.Context, serial string, dir *paths.Path, version, board string, preserve bool, rootSize uint64, cb updater.FlashCallback) error {
		f.flashCalls.Add(1)
		require.NoError(t, ctx.Err())
		require.Equal(t, f.request.Serial, serial)
		require.Equal(t, f.request.Version, version)
		require.Equal(t, f.request.Board, board)
		require.Equal(t, f.request.PreserveUser, preserve)
		require.Zero(t, rootSize)
		data, err := os.ReadFile(dir.Join("image", "payload.txt").String())
		require.NoError(t, err)
		require.NotEmpty(t, data)
		f.extracted = dir.String()
		parent := f.request.TempPath
		if parent == "" {
			userCache, err := os.UserCacheDir()
			require.NoError(t, err)
			parent = filepath.Join(userCache, "arduino-flasher-cli")
		}
		// MkTempDir returns a canonical path; on macOS t.TempDir is under the /var symlink.
		parent, err = filepath.EvalSymlinks(parent)
		require.NoError(t, err)
		relative, err := filepath.Rel(parent, dir.String())
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(relative, "grpc-flash-"))
		cb(updater.FlashEvent{Log: "test flash complete", Progress: 1, Total: 1})
		return f.flashErr
	}
	return f
}

func (f *flashFixture) seedStore(t *testing.T) string {
	t.Helper()
	rel := f.http.release()
	archive, _, err := f.imageStore.Get(context.Background(), rel, true, func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), f.http.archive, 0o600)
	})
	require.NoError(t, err)
	return archive
}

func requireScratchClean(t *testing.T, f *flashFixture) {
	t.Helper()
	entries, err := os.ReadDir(f.request.TempPath)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the existing parent content should remain")
	require.Equal(t, "keep", entries[0].Name())
	if f.extracted != "" {
		require.NoDirExists(t, f.extracted)
	}
}

func TestFlashKeepPolicyAndCleanup(t *testing.T) {
	for _, stored := range []bool{false, true} {
		for _, keep := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				t.Run(fmt.Sprintf("stored=%t/keep=%t/failure=%t", stored, keep, fail), func(t *testing.T) {
					f := setupFlash(t)
					f.request.Keep = keep
					if stored {
						f.seedStore(t)
					}
					if fail {
						f.flashErr = errors.New("test flashing failed")
					}
					err := f.server.Flash(f.request, f.stream)
					if fail {
						require.ErrorIs(t, err, f.flashErr)
					} else {
						require.NoError(t, err)
						require.NotNil(t, f.stream.responses[len(f.stream.responses)-1].GetResult())
					}
					require.Equal(t, int64(1), f.flashCalls.Load())
					require.Positive(t, f.http.manifestRequests.Load(), "stored images still require the online manifest")
					if stored {
						require.Zero(t, f.http.archiveRequests.Load())
					} else {
						require.Positive(t, f.http.archiveRequests.Load())
					}
					_, err = f.imageStore.Lookup(context.Background(), f.http.release())
					if stored || keep {
						require.NoError(t, err, "request cleanup must preserve retained archives even after failure")
					} else {
						require.ErrorIs(t, err, store.ErrMiss)
					}
					partial := filepath.Join(f.imageStore.Dir(), ".partial", f.http.release().FileName)
					if !stored && !keep && fail {
						require.FileExists(t, partial, "a failed flash keeps the download for a retry")
					} else {
						require.NoFileExists(t, partial)
					}
					requireScratchClean(t, f)
					requireFlashProgress(t, f.stream.responses, stored, !fail)
				})
			}
		}
	}
}

func TestFlashFailedDownload(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep=%t", keep), func(t *testing.T) {
			f := setupFlash(t)
			f.request.Keep = keep
			f.http.archiveErr = errors.New("download interrupted")
			require.Error(t, f.server.Flash(f.request, f.stream))
			require.Zero(t, f.flashCalls.Load())
			_, err := f.imageStore.Lookup(context.Background(), f.http.release())
			require.ErrorIs(t, err, store.ErrMiss)
			inventory, err := f.imageStore.List()
			require.NoError(t, err)
			require.Empty(t, inventory.Images)
			requireScratchClean(t, f)
			var failed bool
			for _, response := range f.stream.responses {
				if end := response.GetDownloadProgress().GetEnd(); end != nil {
					failed = !end.Success
				}
			}
			require.True(t, failed)
		})
	}
}

func TestFlashStoredRequiresOnlineManifest(t *testing.T) {
	f := setupFlash(t)
	f.seedStore(t)
	f.http.manifestErr = errors.New("manifest unavailable")
	require.ErrorIs(t, f.server.Flash(f.request, f.stream), f.http.manifestErr)
	require.Zero(t, f.http.archiveRequests.Load())
	require.Zero(t, f.flashCalls.Load())
	_, err := f.imageStore.Lookup(context.Background(), f.http.release())
	require.NoError(t, err)
	requireScratchClean(t, f)
}

func TestFlashDefaultTempParent(t *testing.T) {
	f := setupFlash(t)
	f.seedStore(t)
	parent := f.request.TempPath
	f.request.TempPath = ""
	require.NoError(t, f.server.Flash(f.request, f.stream))
	require.NoDirExists(t, f.extracted)
	userCache, err := os.UserCacheDir()
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Join(userCache, "arduino-flasher-cli"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "images", entries[0].Name())
	f.request.TempPath = parent
	requireScratchClean(t, f)
}

func TestFlashExtractionFailure(t *testing.T) {
	f := setupFlash(t)
	f.http.archive = []byte("not an archive")
	f.seedStore(t)
	require.ErrorContains(t, f.server.Flash(f.request, f.stream), "could not extract archive")
	require.Zero(t, f.flashCalls.Load())
	_, err := f.imageStore.Lookup(context.Background(), f.http.release())
	require.NoError(t, err, "a failed request must not delete a stored archive")
	requireScratchClean(t, f)
}

func TestFlashTempParentInsideStore(t *testing.T) {
	f := setupFlash(t)
	archive := f.seedStore(t)
	f.request.TempPath = filepath.Dir(archive)
	require.NoError(t, f.server.Flash(f.request, f.stream))
	require.Zero(t, f.http.archiveRequests.Load())
	_, err := f.imageStore.Lookup(context.Background(), f.http.release())
	require.NoError(t, err, "even an explicit temp parent inside the image store must preserve stored files")
	entries, err := os.ReadDir(f.request.TempPath)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), "grpc-flash-"), "request scratch directories must be removed")
	}
	require.NoDirExists(t, f.extracted)
}

func TestFlashInvalidRequest(t *testing.T) {
	for _, field := range []string{"nil request", "missing board", "unknown board", "missing version", "missing OS"} {
		t.Run(field, func(t *testing.T) {
			f := setupFlash(t)
			switch field {
			case "nil request":
				f.request = nil
			case "missing board":
				f.request.Board = ""
			case "unknown board":
				f.request.Board = "unknown"
			case "missing version":
				f.request.Version = ""
			case "missing OS":
				f.request.Os = ""
			}
			require.Error(t, f.server.Flash(f.request, f.stream))
			require.Zero(t, f.http.manifestRequests.Load())
			require.Zero(t, f.flashCalls.Load())
		})
	}
}

func requireFlashProgress(t *testing.T, responses []*flasher.FlashResponse, stored, completed bool) {
	t.Helper()
	var starts, ends, updates int
	var extractionComplete, flashComplete, result bool
	for _, response := range responses {
		if download := response.GetDownloadProgress(); download != nil {
			if download.GetStart() != nil {
				starts++
			}
			if download.GetUpdate() != nil {
				updates++
			}
			if end := download.GetEnd(); end != nil {
				ends++
				require.True(t, end.Success)
				if stored {
					require.Equal(t, "Using stored image", end.Message)
				}
			}
		}
		if progress := response.GetExtractionProgress(); progress != nil && progress.Completed {
			extractionComplete = true
		}
		if progress := response.GetFlashProgress(); progress != nil && progress.Completed {
			flashComplete = true
		}
		result = result || response.GetResult() != nil
	}
	require.Equal(t, 1, starts)
	require.Equal(t, 1, ends)
	require.Equal(t, stored, updates == 0)
	require.True(t, extractionComplete)
	require.Equal(t, completed, flashComplete)
	require.Equal(t, completed, result)
}

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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
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
	sendErr   error
}

func (s *flashStream) Context() context.Context { return s.ctx }

func (s *flashStream) Send(response *flasher.FlashResponse) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.responses = append(s.responses, response)
	return nil
}

type flashFixture struct {
	server     *flasherServerImpl
	http       *flashHTTP
	imageCache *cache.Cache
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
	c, err := cache.Default()
	require.NoError(t, err)
	f := &flashFixture{
		server: NewFlasherServer().(*flasherServerImpl), http: h, imageCache: c,
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
		relative, err := filepath.Rel(parent, dir.String())
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(relative, "grpc-flash-"))
		cb(updater.FlashEvent{Log: "test flash complete", Progress: 1, Total: 1})
		return f.flashErr
	}
	return f
}

func (f *flashFixture) seedCache(t *testing.T) string {
	t.Helper()
	rel := f.http.release()
	archive, err := f.imageCache.Store(context.Background(), rel, func(_ context.Context, dir string) error {
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

func TestFlashCachePolicyAndCleanup(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, keep := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				t.Run(fmt.Sprintf("cached=%t/cache=%t/failure=%t", cached, keep, fail), func(t *testing.T) {
					f := setupFlash(t)
					f.request.Cache = keep
					if cached {
						f.seedCache(t)
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
					require.Positive(t, f.http.manifestRequests.Load(), "cached images still require the online manifest")
					if cached {
						require.Zero(t, f.http.archiveRequests.Load())
					} else {
						require.Positive(t, f.http.archiveRequests.Load())
					}
					_, err = f.imageCache.Lookup(context.Background(), f.http.release())
					if cached || keep {
						require.NoError(t, err, "request cleanup must preserve retained archives even after failure")
					} else {
						require.ErrorIs(t, err, cache.ErrMiss)
					}
					requireScratchClean(t, f)
					requireFlashProgress(t, f.stream.responses, cached, !fail)
				})
			}
		}
	}
}

func TestFlashFailedDownload(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", keep), func(t *testing.T) {
			f := setupFlash(t)
			f.request.Cache = keep
			f.http.archiveErr = errors.New("download interrupted")
			require.Error(t, f.server.Flash(f.request, f.stream))
			require.Zero(t, f.flashCalls.Load())
			_, err := f.imageCache.Lookup(context.Background(), f.http.release())
			require.ErrorIs(t, err, cache.ErrMiss)
			inventory, err := f.imageCache.List(context.Background())
			require.NoError(t, err)
			require.Empty(t, inventory.Images)
			require.Zero(t, inventory.TotalSize, "failed cache writes must leave no partial files")
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

func TestFlashRejectsCorruptCache(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", keep), func(t *testing.T) {
			f := setupFlash(t)
			archive := f.seedCache(t)
			require.NoError(t, os.WriteFile(archive, []byte("corrupt image"), 0o600))
			f.request.Cache = keep
			require.NoError(t, f.server.Flash(f.request, f.stream))
			require.Positive(t, f.http.archiveRequests.Load())
			require.Equal(t, int64(1), f.flashCalls.Load())
			_, err := f.imageCache.Lookup(context.Background(), f.http.release())
			if keep {
				require.NoError(t, err, "retaining a fresh download repairs the corrupt entry")
			} else {
				require.ErrorIs(t, err, cache.ErrMiss)
			}
			requireScratchClean(t, f)
		})
	}
}

func TestFlashUsesCurrentManifestChecksum(t *testing.T) {
	f := setupFlash(t)
	oldRelease := f.http.release()
	f.seedCache(t)
	f.http.archive = imageArchive(t, "updated image with the same version")
	f.request.Cache = true
	require.NoError(t, f.server.Flash(f.request, f.stream))
	require.Positive(t, f.http.archiveRequests.Load(), "a different online checksum must invalidate a cached version")
	_, err := f.imageCache.Lookup(context.Background(), f.http.release())
	require.NoError(t, err)
	_, err = f.imageCache.Lookup(context.Background(), oldRelease)
	require.NoError(t, err, "the earlier cache entry is not owned by this request")
	requireScratchClean(t, f)
}

func TestFlashCacheRequiresOnlineManifest(t *testing.T) {
	f := setupFlash(t)
	f.seedCache(t)
	f.http.manifestErr = errors.New("manifest unavailable")
	require.ErrorIs(t, f.server.Flash(f.request, f.stream), f.http.manifestErr)
	require.Zero(t, f.http.archiveRequests.Load())
	require.Zero(t, f.flashCalls.Load())
	_, err := f.imageCache.Lookup(context.Background(), f.http.release())
	require.NoError(t, err)
	requireScratchClean(t, f)
}

func TestFlashDefaultTempParent(t *testing.T) {
	f := setupFlash(t)
	f.seedCache(t)
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
	f.seedCache(t)
	require.ErrorContains(t, f.server.Flash(f.request, f.stream), "could not extract archive")
	require.Zero(t, f.flashCalls.Load())
	_, err := f.imageCache.Lookup(context.Background(), f.http.release())
	require.NoError(t, err, "a failed request must not delete a cached archive")
	requireScratchClean(t, f)
}

func TestFlashTempParentInsideCache(t *testing.T) {
	f := setupFlash(t)
	archive := f.seedCache(t)
	f.request.TempPath = filepath.Dir(archive)
	require.NoError(t, f.server.Flash(f.request, f.stream))
	require.Zero(t, f.http.archiveRequests.Load())
	_, err := f.imageCache.Lookup(context.Background(), f.http.release())
	require.NoError(t, err, "even an explicit temp parent inside the cache must preserve cached files")
	entries, err := os.ReadDir(f.request.TempPath)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, entry.IsDir(), "only cache files should remain, not request scratch directories")
	}
	require.NoDirExists(t, f.extracted)
}

func TestFlashDisconnectedStream(t *testing.T) {
	f := setupFlash(t)
	f.stream.sendErr = errors.New("client disconnected")
	require.ErrorIs(t, f.server.Flash(f.request, f.stream), f.stream.sendErr)
	require.Zero(t, f.http.archiveRequests.Load())
	require.Zero(t, f.flashCalls.Load())
	requireScratchClean(t, f)
}

func TestFlashCanceledRequest(t *testing.T) {
	f := setupFlash(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.stream.ctx = ctx
	require.ErrorIs(t, f.server.Flash(f.request, f.stream), context.Canceled)
	require.Zero(t, f.http.manifestRequests.Load())
	require.Zero(t, f.flashCalls.Load())
	requireScratchClean(t, f)
}

func TestFlashInvalidRequest(t *testing.T) {
	for _, field := range []string{"nil request", "missing board", "unknown board", "missing version", "missing OS", "stream"} {
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
			case "stream":
				require.Error(t, f.server.Flash(f.request, nil))
				return
			}
			require.Error(t, f.server.Flash(f.request, f.stream))
			require.Zero(t, f.http.manifestRequests.Load())
			require.Zero(t, f.flashCalls.Load())
		})
	}
}

func TestFlashGRPCSharedCache(t *testing.T) {
	f := setupFlash(t)
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	flasher.RegisterFlasherServer(server, f.server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.NewClient("passthrough:///flash-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}))
	require.NoError(t, err)
	defer conn.Close()
	f.request.Cache = true
	stream, err := flasher.NewFlasherClient(conn).Flash(ctx, f.request)
	require.NoError(t, err)
	var responses []*flasher.FlashResponse
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		responses = append(responses, response)
	}
	requireFlashProgress(t, responses, false, true)
	require.Equal(t, int64(1), f.flashCalls.Load())
	_, err = f.imageCache.Lookup(context.Background(), f.http.release())
	require.NoError(t, err, "the cache flag must survive gRPC serialization")
	requireScratchClean(t, f)
}

func TestFlashRequestCacheDefaultsToFalse(t *testing.T) {
	for _, keep := range []bool{false, true} {
		request := &flasher.FlashRequest{Board: "unoq", Version: "latest", Os: testDistribution, Cache: keep}
		data, err := proto.Marshal(request)
		require.NoError(t, err)
		var decoded flasher.FlashRequest
		require.NoError(t, proto.Unmarshal(data, &decoded))
		require.Equal(t, keep, decoded.GetCache())
	}
}

func requireFlashProgress(t *testing.T, responses []*flasher.FlashResponse, cached, completed bool) {
	t.Helper()
	var starts, ends, updates int
	var extractionComplete, flashComplete, result bool
	for _, response := range responses {
		if download := response.GetDownloadProgress(); download != nil {
			if download.GetStart() != nil {
				starts++
			}
			if update := download.GetUpdate(); update != nil {
				updates++
				if cached {
					require.Equal(t, update.TotalSize, update.Downloaded)
				}
			}
			if end := download.GetEnd(); end != nil {
				ends++
				require.True(t, end.Success)
				if cached {
					require.Equal(t, "Using cached image", end.Message)
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
	require.Positive(t, updates)
	require.True(t, extractionComplete)
	require.Equal(t, completed, flashComplete)
	require.Equal(t, completed, result)
}

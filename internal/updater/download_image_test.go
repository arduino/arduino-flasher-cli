// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

type archiveTransport struct {
	contents []byte
	requests int
	err      error
}

func (s *archiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.requests++
	if s.err != nil {
		return nil, s.err
	}
	if req.URL.String() != "https://images.example/image.tar" {
		return nil, fmt.Errorf("unexpected request: %s", req.URL)
	}
	body := s.contents
	if req.Method == http.MethodHead {
		body = nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Length": []string{strconv.Itoa(len(s.contents))},
		},
		ContentLength: int64(len(s.contents)),
		Body:          io.NopCloser(bytes.NewReader(body)),
		Request:       req,
	}, nil
}

func setupImageDownload(t *testing.T) (*cache.Cache, registry.Release, *archiveTransport) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", t.TempDir())
	case "darwin":
		t.Setenv("HOME", t.TempDir())
	default:
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
	}
	t.Setenv("ARDUINO_FLASHER_ADDITIONAL_HEADERS", "")
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "image/payload.txt", Mode: 0o600, Size: int64(len("test payload"))}))
	_, err := w.Write([]byte("test payload"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	checksum := sha256.Sum256(archive.Bytes())
	rel := registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Url: "https://images.example/image.tar", FileName: "image.tar",
		Sha256: hex.EncodeToString(checksum[:]),
	}
	transport := &archiveTransport{contents: archive.Bytes()}
	original := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() { http.DefaultClient = original })
	imageCache, err := cache.Default()
	require.NoError(t, err)
	return imageCache, rel, transport
}

func seedImageCache(t *testing.T, imageCache *cache.Cache, rel registry.Release, contents []byte) string {
	t.Helper()
	archive, err := imageCache.Store(context.Background(), rel, func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), contents, 0o600)
	})
	require.NoError(t, err)
	return archive
}

func requireExtractedPayload(t *testing.T, dir *paths.Path) {
	t.Helper()
	data, err := os.ReadFile(dir.Join("image", "payload.txt").String())
	require.NoError(t, err)
	require.Equal(t, "test payload", string(data))
}

func TestDownloadAndExtractReusesCache(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", keep), func(t *testing.T) {
			imageCache, rel, transport := setupImageDownload(t)
			archive := seedImageCache(t, imageCache, rel, transport.contents)
			scratch := paths.New(t.TempDir())
			require.NoError(t, DownloadAndExtract(context.Background(), rel, scratch, keep))
			requireExtractedPayload(t, scratch)
			require.Zero(t, transport.requests, "reusing an archive must not contact its server")
			require.NoDirExists(t, filepath.Join(filepath.Dir(archive), "image"), "extraction must stay outside the cache")
			require.NoError(t, scratch.RemoveAll())
			_, err := imageCache.Lookup(context.Background(), rel)
			require.NoError(t, err, "cleaning scratch space must not delete cached archives")
		})
	}
}

func TestDownloadAndExtractFreeSpace(t *testing.T) {
	for _, keep := range []bool{false, true} {
		for _, tc := range []struct {
			name         string
			state        string
			freeGiB      uint64
			wantRequired uint64
			wantSuccess  bool
		}{
			{"cached with 11 GiB", "cached", 11, 10, true},
			{"cached with exactly 10 GiB", "cached", 10, 10, true},
			{"cached below 10 GiB", "cached", 9, 10, false},
			{"missing with 11 GiB", "missing", 11, 12, false},
			{"corrupt with 11 GiB", "corrupt", 11, 12, false},
			{"missing with exactly 12 GiB", "missing", 12, 12, true},
		} {
			t.Run(fmt.Sprintf("%s/cache=%t", tc.name, keep), func(t *testing.T) {
				c, rel, transport := setupImageDownload(t)
				var archive string
				if tc.state != "missing" {
					archive = seedImageCache(t, c, rel, transport.contents)
					if tc.state == "corrupt" {
						require.NoError(t, os.WriteFile(archive, []byte("corrupted archive"), 0o600))
					}
				}
				scratch := paths.New(t.TempDir())
				spaceErr := errors.New("insufficient scratch space")
				var checkedRequired uint64
				var checkedOperation string
				checks := 0
				checkSpace := func(dir *paths.Path, requiredGiB uint64, what string) error {
					checks++
					require.Equal(t, scratch.String(), dir.String())
					require.Zero(t, transport.requests, "space must be checked before a download starts")
					require.NoDirExists(t, scratch.Join("image").String(), "space must be checked before extraction")
					checkedRequired, checkedOperation = requiredGiB, what
					if tc.freeGiB < requiredGiB {
						return spaceErr
					}
					return nil
				}
				err := downloadAndExtract(context.Background(), rel, scratch, keep, checkSpace)
				if tc.wantSuccess {
					require.NoError(t, err)
					requireExtractedPayload(t, scratch)
					if tc.state == "cached" {
						require.Zero(t, transport.requests)
					} else {
						require.Positive(t, transport.requests)
					}
				} else {
					require.ErrorIs(t, err, spaceErr)
					require.Zero(t, transport.requests, "insufficient space must stop before downloading")
					require.NoDirExists(t, scratch.Join("image").String())
				}
				require.Equal(t, 1, checks)
				require.Equal(t, tc.wantRequired, checkedRequired)
				operation := "download and extraction"
				if tc.state == "cached" {
					operation = "extraction"
					cached, err := c.Lookup(context.Background(), rel)
					require.NoError(t, err, "a space check must preserve the cached archive")
					require.Equal(t, archive, cached)
				}
				require.Equal(t, operation, checkedOperation)
			})
		}
	}
}

func TestDownloadAndExtractCachesOnlyWhenRequested(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", keep), func(t *testing.T) {
			imageCache, rel, transport := setupImageDownload(t)
			scratch := paths.New(t.TempDir())
			require.NoError(t, DownloadAndExtract(context.Background(), rel, scratch, keep))
			requireExtractedPayload(t, scratch)
			require.NotZero(t, transport.requests)
			archive, err := imageCache.Lookup(context.Background(), rel)
			if keep {
				require.NoError(t, err)
				require.NoFileExists(t, scratch.Join(rel.FileName).String(), "a retained archive belongs in the cache")
			} else {
				require.ErrorIs(t, err, cache.ErrMiss)
				require.NoDirExists(t, imageCache.Dir())
				require.FileExists(t, scratch.Join(rel.FileName).String())
			}
			require.NoError(t, scratch.RemoveAll())
			if keep {
				require.FileExists(t, archive)
			}
		})
	}
}

func TestDownloadAndExtractRejectsCorruptCache(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", keep), func(t *testing.T) {
			imageCache, rel, transport := setupImageDownload(t)
			archive := seedImageCache(t, imageCache, rel, transport.contents)
			require.NoError(t, os.WriteFile(archive, []byte("corrupt archive"), 0o600))
			scratch := paths.New(t.TempDir())
			require.NoError(t, DownloadAndExtract(context.Background(), rel, scratch, keep))
			requireExtractedPayload(t, scratch)
			require.NotZero(t, transport.requests, "a corrupt cached image must be downloaded again")
			_, err := imageCache.Lookup(context.Background(), rel)
			if keep {
				require.NoError(t, err, "opt-in caching replaces the corrupt entry")
			} else {
				require.ErrorIs(t, err, cache.ErrMiss, "without opt-in, the fresh download stays temporary")
			}
		})
	}
}

func TestDownloadAndExtractFailureKeepsCachedArchive(t *testing.T) {
	imageCache, rel, transport := setupImageDownload(t)
	archive := seedImageCache(t, imageCache, rel, transport.contents)
	scratch := paths.New(t.TempDir())
	// A file blocking the archive's directory forces extraction to fail.
	require.NoError(t, os.WriteFile(scratch.Join("image").String(), []byte("blocked"), 0o600))
	require.Error(t, DownloadAndExtract(context.Background(), rel, scratch, false))
	require.Zero(t, transport.requests)
	require.NoError(t, scratch.RemoveAll())
	cached, err := imageCache.Lookup(context.Background(), rel)
	require.NoError(t, err)
	require.Equal(t, archive, cached)
}

func TestDownloadAndExtractFailedDownloadIsNotCached(t *testing.T) {
	imageCache, rel, transport := setupImageDownload(t)
	transport.err = errors.New("connection lost")
	scratch := paths.New(t.TempDir())
	err := DownloadAndExtract(context.Background(), rel, scratch, true)
	require.ErrorContains(t, err, "connection lost")
	_, err = imageCache.Lookup(context.Background(), rel)
	require.ErrorIs(t, err, cache.ErrMiss)
	entries, err := os.ReadDir(imageCache.Dir())
	require.NoError(t, err)
	require.Empty(t, entries, "a failed retained download leaves no cache entry")
	require.NoDirExists(t, scratch.Join("image").String())
}

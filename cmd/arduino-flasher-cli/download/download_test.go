// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package download

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

// imageTransport serves both published indexes and the archive in memory. No
// request can reach the network, including on a cache hit or a custom download.
type imageTransport struct {
	contents         string
	manifestRequests int
	archiveRequests  int
	downloads        int
}

func (s *imageTransport) release() registry.Release {
	checksum := sha256.Sum256([]byte(s.contents))
	return registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Url: "https://images.example/image.tar.zst", FileName: "image.tar.zst",
		Sha256: hex.EncodeToString(checksum[:]),
	}
}

func (s *imageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	status := http.StatusOK
	var body string
	switch req.URL.String() {
	case "https://downloads.arduino.cc/debian-im/Stable/info.json":
		s.manifestRequests++
		rel := s.release()
		data, err := json.Marshal(map[string]any{
			"releases": []map[string]string{{
				"version": rel.Version, "url": rel.Url, "sha256": rel.Sha256,
				"board": registry.UnoQ.Label, "os": "Debian GNU/Linux 13 (trixie)",
			}},
		})
		if err != nil {
			return nil, err
		}
		body = string(data)
	case "https://downloads.arduino.cc/ubuntu-im/Stable/info.json":
		s.manifestRequests++
		status = http.StatusNotFound
	case "https://images.example/image.tar.zst":
		s.archiveRequests++
		if req.Method == http.MethodGet {
			s.downloads++
		}
		body = s.contents
	default:
		return nil, fmt.Errorf("unexpected HTTP request: %s %s", req.Method, req.URL)
	}
	length := int64(len(body))
	if req.Method == http.MethodHead {
		body = ""
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header: http.Header{
			"Content-Length": []string{strconv.FormatInt(length, 10)},
		},
		ContentLength: length,
		Body:          io.NopCloser(strings.NewReader(body)),
		Request:       req,
	}, nil
}

func setupDownload(t *testing.T) (*imageTransport, *cache.Cache) {
	t.Helper()
	// Keep the real user cache untouched on every supported host OS.
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
	transport := &imageTransport{contents: "test image"}
	original := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() { http.DefaultClient = original })
	imageCache, err := cache.Default()
	require.NoError(t, err)
	return transport, imageCache
}

func executeDownload(t *testing.T, args ...string) {
	t.Helper()
	cmd := NewDownloadCmd()
	cmd.SetArgs(args)
	require.NoError(t, cmd.ExecuteContext(context.Background()))
}

func TestDownloadCachesByDefault(t *testing.T) {
	transport, imageCache := setupDownload(t)
	executeDownload(t, "unoq")
	require.Equal(t, 1, transport.downloads)
	archive, err := imageCache.Lookup(context.Background(), transport.release())
	require.NoError(t, err)
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	require.Equal(t, transport.contents, string(data))
	manifestRequests, archiveRequests := transport.manifestRequests, transport.archiveRequests

	// An explicit version resolves to the very same cached archive as latest.
	executeDownload(t, "unoq", "--version", transport.release().Version)
	require.Greater(t, transport.manifestRequests, manifestRequests, "cache hits still consult the online manifest")
	require.Equal(t, archiveRequests, transport.archiveRequests, "a valid cached archive needs no HEAD or GET request")
	require.Equal(t, 1, transport.downloads)
}

func TestDownloadRepairsCorruptCache(t *testing.T) {
	transport, imageCache := setupDownload(t)
	executeDownload(t, "unoq")
	archive, err := imageCache.Lookup(context.Background(), transport.release())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archive, []byte("corrupted!"), 0o600))

	executeDownload(t, "unoq")
	require.Equal(t, 2, transport.downloads)
	_, err = imageCache.Lookup(context.Background(), transport.release())
	require.NoError(t, err, "a corrupt archive must be replaced by a verified download")
}

func TestDownloadChecksUpdatedManifest(t *testing.T) {
	transport, imageCache := setupDownload(t)
	executeDownload(t, "unoq")
	original := transport.release()

	// The publisher replaces an image without changing its version or URL.
	transport.contents = "updated image"
	executeDownload(t, "unoq")
	require.Equal(t, 2, transport.downloads)
	archive, err := imageCache.Lookup(context.Background(), transport.release())
	require.NoError(t, err)
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	require.Equal(t, transport.contents, string(data))
	previous, err := imageCache.Lookup(context.Background(), original)
	require.NoError(t, err)
	require.NotEqual(t, previous, archive, "the old checksum cannot validate the new release")
}

func TestDownloadExplicitDestinationBypassesCache(t *testing.T) {
	for _, destination := range []string{"custom directory", "current directory"} {
		t.Run(destination, func(t *testing.T) {
			transport, imageCache := setupDownload(t)
			dir := t.TempDir()
			argument := dir
			if destination == "current directory" {
				t.Chdir(dir)
				argument = "."
			}
			executeDownload(t, "unoq", "--dest-dir", argument)
			data, err := os.ReadFile(filepath.Join(dir, transport.release().FileName))
			require.NoError(t, err)
			require.Equal(t, transport.contents, string(data))
			require.NoDirExists(t, imageCache.Dir(), "an explicit destination must not populate the cache")
		})
	}
}

func TestDownloadExplicitDestinationWithCachedImage(t *testing.T) {
	transport, imageCache := setupDownload(t)
	executeDownload(t, "unoq")
	cached, err := imageCache.Lookup(context.Background(), transport.release())
	require.NoError(t, err)

	dir := t.TempDir()
	executeDownload(t, "unoq", "--dest-dir", dir)
	require.FileExists(t, filepath.Join(dir, transport.release().FileName))
	archive, err := imageCache.Lookup(context.Background(), transport.release())
	require.NoError(t, err)
	require.Equal(t, cached, archive, "an explicit download must not move or remove a cached image")
}

func TestDownloadedImageCanBePreparedForFlash(t *testing.T) {
	transport, imageCache := setupDownload(t)
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "image/payload.txt", Mode: 0o600, Size: int64(len("test payload"))}))
	_, err := w.Write([]byte("test payload"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	transport.contents = archive.String()
	executeDownload(t, "unoq")
	archiveRequests := transport.archiveRequests

	// Exercise the same preparation used by flash, without invoking QDL.
	scratch := paths.New(t.TempDir())
	require.NoError(t, updater.DownloadAndExtract(context.Background(), transport.release(), scratch, false))
	data, err := os.ReadFile(scratch.Join("image", "payload.txt").String())
	require.NoError(t, err)
	require.Equal(t, "test payload", string(data))
	require.Equal(t, archiveRequests, transport.archiveRequests, "flash preparation must reuse the download command's cache")
	require.NoError(t, scratch.RemoveAll())
	_, err = imageCache.Lookup(context.Background(), transport.release())
	require.NoError(t, err)
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package download

import (
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

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
)

// imageTransport serves both published indexes and the archive in memory. No
// request can reach the network, including when the image is kept or for a custom download.
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

func setupDownload(t *testing.T) (*imageTransport, *store.Store) {
	t.Helper()
	// Keep the real image store untouched on every supported host OS.
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
	imageStore, err := store.Default()
	require.NoError(t, err)
	return transport, imageStore
}

func executeDownload(t *testing.T, args ...string) {
	t.Helper()
	cmd := NewDownloadCmd()
	cmd.SetArgs(args)
	require.NoError(t, cmd.ExecuteContext(context.Background()))
}

func TestDownloadKeepsByDefault(t *testing.T) {
	transport, imageStore := setupDownload(t)
	executeDownload(t, "unoq")
	require.Equal(t, 1, transport.downloads)
	archive, err := imageStore.Lookup(context.Background(), transport.release())
	require.NoError(t, err)
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	require.Equal(t, transport.contents, string(data))
	manifestRequests, archiveRequests := transport.manifestRequests, transport.archiveRequests

	// An explicit version resolves to the very same kept archive as latest.
	executeDownload(t, "unoq", "--version", transport.release().Version)
	require.Greater(t, transport.manifestRequests, manifestRequests, "kept images still need the online manifest")
	require.Equal(t, archiveRequests, transport.archiveRequests, "a valid kept archive needs no HEAD or GET request")
	require.Equal(t, 1, transport.downloads)
}

func TestDownloadExplicitDestinationBypassesStore(t *testing.T) {
	for _, destination := range []string{"custom directory", "current directory"} {
		t.Run(destination, func(t *testing.T) {
			transport, imageStore := setupDownload(t)
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
			require.NoDirExists(t, imageStore.Dir(), "an explicit destination must not populate the image store")
		})
	}
}

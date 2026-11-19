// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package list

import (
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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

type manifestTransport struct {
	releases registry.Releases
	requests []string
}

func (s *manifestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.requests = append(s.requests, req.URL.String())
	status := http.StatusOK
	var body []byte
	switch req.URL.String() {
	case "https://downloads.arduino.cc/debian-im/Stable/info.json":
		var releases []map[string]string
		for _, rel := range s.releases {
			board, ok := registry.BoardByID(rel.Board)
			if !ok {
				return nil, fmt.Errorf("unknown fixture board %s", rel.Board)
			}
			releases = append(releases, map[string]string{
				"board": board.Label, "os": rel.OS, "version": rel.Version,
				"url": rel.Url, "sha256": rel.Sha256,
			})
		}
		var err error
		body, err = json.Marshal(map[string]any{"releases": releases})
		if err != nil {
			return nil, err
		}
	case "https://downloads.arduino.cc/ubuntu-im/Stable/info.json":
		status = http.StatusNotFound
	default:
		return nil, fmt.Errorf("list must not download images: %s", req.URL)
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}, nil
}

func setupList(t *testing.T) (*cache.Cache, *manifestTransport, registry.Release) {
	t.Helper()
	// Tests use their own cache and in-memory manifests, never the user's images
	// or the network. Environment and HTTP globals prevent parallel subtests.
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
	sum := sha256.Sum256([]byte("test image"))
	rel := registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Url: "https://images.example/image.tar.zst", FileName: "image.tar.zst",
		Sha256: hex.EncodeToString(sum[:]),
	}
	transport := &manifestTransport{releases: registry.Releases{rel}}
	original := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() { http.DefaultClient = original })
	c, err := cache.Default()
	require.NoError(t, err)
	return c, transport, rel
}

func seedListCache(t *testing.T, c *cache.Cache, rel registry.Release) string {
	t.Helper()
	archive, err := c.Store(context.Background(), rel, func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), []byte("test image"), 0o600)
	})
	require.NoError(t, err)
	return archive
}

func executeList(t *testing.T, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	feedback.SetOut(&output)
	t.Cleanup(func() { feedback.SetOut(os.Stdout) })
	cmd := NewListCmd()
	cmd.SetArgs(args)
	require.NoError(t, cmd.ExecuteContext(context.Background()))
	return output.String()
}

func requireCacheMark(t *testing.T, output, version string, cached bool) {
	t.Helper()
	require.Contains(t, output, "CACHED")
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == version {
			// This fixture has one release per board/OS, so it always has a
			// LATEST mark. A cached release has a second mark in CACHED.
			marks := 1
			if cached {
				marks++
			}
			require.Equal(t, marks, strings.Count(line, "✓"), line)
			return
		}
	}
	t.Fatalf("version %s missing from output:\n%s", version, output)
}

func TestListCacheIdentity(t *testing.T) {
	const renamed = "different archive name"
	for _, change := range []string{"matching", renamed, "different checksum"} {
		t.Run(change, func(t *testing.T) {
			c, transport, rel := setupList(t)
			seedListCache(t, c, rel)
			online := rel
			// The archive name is the identity; Lookup checks the checksum before use.
			wantCached := change != renamed
			switch change {
			case renamed:
				online.Url = "https://images.example/renamed.tar.zst"
			case "different checksum":
				online.Sha256 = strings.Repeat("a", 64)
			}
			transport.releases = registry.Releases{online}
			output := executeList(t)
			requireCacheMark(t, output, online.Version, wantCached)
			require.Len(t, transport.requests, 2, "list should only read the two published indexes once")
		})
	}
}

func TestListUnavailableCachePreservesOnlineImages(t *testing.T) {
	c, _, rel := setupList(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Dir()), 0o750))
	require.NoError(t, os.WriteFile(c.Dir(), []byte("not a directory"), 0o600))
	requireCacheMark(t, executeList(t), rel.Version, false)
}

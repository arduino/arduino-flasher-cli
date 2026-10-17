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
	for _, change := range []string{"matching", "board", "os", "version", "checksum", "archive name", "case"} {
		t.Run(change, func(t *testing.T) {
			c, transport, rel := setupList(t)
			seedListCache(t, c, rel)
			online := rel
			switch change {
			case "board":
				online.Board = registry.VentunoQ.ID
			case "os":
				online.OS = "ubuntu"
			case "version":
				online.Version = "20250916-174"
			case "checksum":
				online.Sha256 = strings.Repeat("a", 64)
			case "archive name":
				online.Url = "https://images.example/renamed.tar.zst"
			case "case":
				online.OS = "Debian GNU/Linux 13 (trixie)"
				online.Sha256 = strings.ToUpper(online.Sha256)
			}
			transport.releases = registry.Releases{online}
			output := executeList(t)
			requireCacheMark(t, output, online.Version, change == "matching" || change == "case")
			require.Len(t, transport.requests, 2, "list should only read the two published indexes once")
		})
	}
}

func TestListMissingAndIncompleteCache(t *testing.T) {
	for _, state := range []string{"missing cache", "missing metadata", "missing archive", "wrong size"} {
		t.Run(state, func(t *testing.T) {
			c, _, rel := setupList(t)
			if state != "missing cache" {
				archive := seedListCache(t, c, rel)
				switch state {
				case "missing metadata":
					require.NoError(t, os.Remove(filepath.Join(filepath.Dir(archive), "meta.json")))
				case "missing archive":
					require.NoError(t, os.Remove(archive))
				case "wrong size":
					require.NoError(t, os.Truncate(archive, 3))
				}
			}
			requireCacheMark(t, executeList(t), rel.Version, false)
			if state == "missing cache" {
				require.NoDirExists(t, c.Dir(), "listing should not create the cache")
			}
		})
	}
}

func TestListDoesNotHashArchives(t *testing.T) {
	c, _, rel := setupList(t)
	archive := seedListCache(t, c, rel)
	require.NoError(t, os.WriteFile(archive, []byte("corrupted!"), 0o600))
	requireCacheMark(t, executeList(t), rel.Version, true)
	_, err := c.Lookup(context.Background(), rel)
	require.ErrorIs(t, err, cache.ErrMiss, "flashing still verifies same-size corruption")
}

func TestListUnavailableCachePreservesOnlineImages(t *testing.T) {
	c, _, rel := setupList(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Dir()), 0o750))
	require.NoError(t, os.WriteFile(c.Dir(), []byte("not a directory"), 0o600))
	requireCacheMark(t, executeList(t), rel.Version, false)
}

func TestListFiltersWithCacheMarks(t *testing.T) {
	c, transport, rel := setupList(t)
	seedListCache(t, c, rel)
	other := rel
	other.Board, other.OS = registry.VentunoQ.ID, "ubuntu"
	transport.releases = append(transport.releases, other)
	output := executeList(t, "unoq", "--os", "debian")
	requireCacheMark(t, output, rel.Version, true)
	require.NotContains(t, output, "ventunoq")
	require.NotContains(t, output, "ubuntu")
}

func TestListResultJSONIncludesCacheStatus(t *testing.T) {
	result := listResult{Releases: []listRelease{
		{Version: "cached", Cached: true},
		{Version: "not-cached", Cached: false},
	}}
	data, err := json.Marshal(result.Data())
	require.NoError(t, err)
	var decoded struct {
		Releases []struct {
			Version string `json:"version"`
			Cached  *bool  `json:"cached"`
		} `json:"releases"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Releases, 2)
	for i, release := range decoded.Releases {
		require.NotNil(t, release.Cached, "the JSON field must be present even when false")
		require.Equal(t, result.Releases[i].Cached, *release.Cached)
	}
}

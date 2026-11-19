// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	imagecache "github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

const (
	listCommand  = "list"
	clearCommand = "clear"
)

type offlineTransport struct {
	requests int
}

func (s *offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	s.requests++
	return nil, errors.New("network is unavailable")
}

func setupCacheCommand(t *testing.T) (*imagecache.Cache, *offlineTransport) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", t.TempDir())
	case "darwin":
		t.Setenv("HOME", t.TempDir())
	default:
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
	}
	transport := &offlineTransport{}
	original := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() { http.DefaultClient = original })
	c, err := imagecache.Default()
	require.NoError(t, err)
	return c, transport
}

func executeCache(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	feedback.SetOut(&output)
	t.Cleanup(func() { feedback.SetOut(os.Stdout) })
	cmd := NewCacheCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return output.String(), err
}

func seedCachedImage(t *testing.T, c *imagecache.Cache) string {
	t.Helper()
	contents := []byte("test image")
	sum := sha256.Sum256(contents)
	rel := registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Sha256: hex.EncodeToString(sum[:]), FileName: "image.tar.zst",
	}
	archive, err := c.Store(context.Background(), rel, func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), contents, 0o600)
	})
	require.NoError(t, err)
	return archive
}

func TestCacheListAndClearCommandsWorkOffline(t *testing.T) {
	c, transport := setupCacheCommand(t)
	archive := seedCachedImage(t, c)
	output, err := executeCache(t, listCommand)
	require.NoError(t, err)
	for _, expected := range []string{c.Dir(), "FILE", "SIZE", "DOWNLOADED", "image.tar.zst"} {
		require.Contains(t, output, expected)
	}
	output, err = executeCache(t, clearCommand)
	require.NoError(t, err)
	require.Contains(t, output, "Image cache cleared:")
	require.Contains(t, output, c.Dir())
	require.NoFileExists(t, archive)
	output, err = executeCache(t, listCommand)
	require.NoError(t, err)
	require.Contains(t, output, "No complete images are cached.")
	require.Zero(t, transport.requests)
}

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
	"encoding/json"
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
	for _, expected := range []string{c.Dir(), "BOARD", "OS", "VERSION", "SIZE", "DOWNLOADED", "unoq", "debian", "20250915-173", "Total cache size:"} {
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
	require.Contains(t, output, "Total cache size: 0 B")
	require.Zero(t, transport.requests)
}

func TestCacheCommandsHandleMissingDirectory(t *testing.T) {
	c, transport := setupCacheCommand(t)
	for _, command := range []string{listCommand, clearCommand, clearCommand} {
		_, err := executeCache(t, command)
		require.NoError(t, err)
		require.NoDirExists(t, c.Dir())
	}
	require.Zero(t, transport.requests)
}

func TestCacheCommandsRejectArgumentsBeforeDeleting(t *testing.T) {
	c, _ := setupCacheCommand(t)
	archive := seedCachedImage(t, c)
	for _, command := range []string{listCommand, clearCommand} {
		_, err := executeCache(t, command, "unexpected-version")
		require.Error(t, err)
		require.FileExists(t, archive)
	}
}

func TestCacheResultJSON(t *testing.T) {
	c, _ := setupCacheCommand(t)
	archive := seedCachedImage(t, c)
	inventory, err := c.List(context.Background())
	require.NoError(t, err)
	result := listResult{Directory: c.Dir(), Images: inventory.Images, TotalSize: inventory.TotalSize}
	data, err := json.Marshal(result.Data())
	require.NoError(t, err)
	var parsed listResult
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Equal(t, c.Dir(), parsed.Directory)
	require.Len(t, parsed.Images, 1)
	require.Equal(t, archive, parsed.Images[0].Path)
	require.Equal(t, "20250915-173", parsed.Images[0].Version)
	require.Positive(t, parsed.TotalSize)
	data, err = json.Marshal(clearResult{Directory: c.Dir(), Cleared: true}.Data())
	require.NoError(t, err)
	var cleared clearResult
	require.NoError(t, json.Unmarshal(data, &cleared))
	require.True(t, cleared.Cleared)
	require.Equal(t, c.Dir(), cleared.Directory)
}

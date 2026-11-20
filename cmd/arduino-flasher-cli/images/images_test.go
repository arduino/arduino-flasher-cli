// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package images

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
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
)

const (
	listCommand   = "list"
	removeCommand = "remove"
)

type offlineTransport struct {
	requests int
}

func (s *offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	s.requests++
	return nil, errors.New("network is unavailable")
}

func setupImagesCommand(t *testing.T) (*store.Store, *offlineTransport) {
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
	c, err := store.Default()
	require.NoError(t, err)
	return c, transport
}

func executeImages(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	feedback.SetOut(&output)
	t.Cleanup(func() { feedback.SetOut(os.Stdout) })
	cmd := NewImagesCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return output.String(), err
}

func seedKeptImage(t *testing.T, c *store.Store) string {
	t.Helper()
	contents := []byte("test image")
	sum := sha256.Sum256(contents)
	rel := registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Sha256: hex.EncodeToString(sum[:]), FileName: "image.tar.zst",
	}
	archive, _, err := c.Get(context.Background(), rel, true, func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), contents, 0o600)
	})
	require.NoError(t, err)
	return archive
}

func TestImagesCommandsWorkOffline(t *testing.T) {
	c, transport := setupImagesCommand(t)
	archive := seedKeptImage(t, c)
	require.NoError(t, os.WriteFile(filepath.Join(c.Dir(), ".partial", "other.tar.zst"), []byte("partial"), 0o600))
	output, err := executeImages(t, listCommand)
	require.NoError(t, err)
	for _, expected := range []string{c.Dir(), "FILE", "SIZE", "DOWNLOADED", "image.tar.zst", "Partial downloads: 7 B", "Other files: 0 B"} {
		require.Contains(t, output, expected)
	}
	_, err = executeImages(t, removeCommand)
	require.Error(t, err, "remove needs file names or --all")
	output, err = executeImages(t, removeCommand, "-a")
	require.NoError(t, err)
	require.Contains(t, output, "image.tar.zst")
	require.NoFileExists(t, archive)
	output, err = executeImages(t, listCommand)
	require.NoError(t, err)
	require.Contains(t, output, "No images are kept.")
	require.Zero(t, transport.requests)
}

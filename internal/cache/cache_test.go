// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

func releaseFor(contents string) registry.Release {
	checksum := sha256.Sum256([]byte(contents))
	return registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Sha256: hex.EncodeToString(checksum[:]), FileName: "image.tar.zst",
	}
}

func downloadContents(rel registry.Release, contents string) func(context.Context, string) error {
	return func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), []byte(contents), 0o600)
	}
}

func TestStoreAndReuse(t *testing.T) {
	ctx := context.Background()
	c := New(filepath.Join(t.TempDir(), "images"))
	rel := releaseFor("test image")
	_, err := c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)
	require.NoDirExists(t, c.Dir(), "looking up an image does not create the cache")

	archive, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(c.Dir(), rel.FileName), archive)
	reused, err := c.Lookup(ctx, rel)
	require.NoError(t, err)
	require.Equal(t, archive, reused)
}

func TestStoreResumesPartialDownload(t *testing.T) {
	ctx := context.Background()
	c := New(t.TempDir())
	rel := releaseFor("test image")
	connectionLost := errors.New("connection lost")
	_, err := c.Store(ctx, rel, func(ctx context.Context, dir string) error {
		require.NoError(t, downloadContents(rel, "test ")(ctx, dir))
		return connectionLost
	})
	require.ErrorIs(t, err, connectionLost)
	_, err = c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)
	require.FileExists(t, filepath.Join(c.Dir(), ".partial", rel.FileName), "an interrupted download is kept for resume")

	archive, err := c.Store(ctx, rel, func(_ context.Context, dir string) error {
		f, err := os.OpenFile(filepath.Join(dir, rel.FileName), os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString("image")
		return err
	})
	require.NoError(t, err)
	reused, err := c.Lookup(ctx, rel)
	require.NoError(t, err)
	require.Equal(t, archive, reused)
}

func TestStoreReplacesDamagedArchive(t *testing.T) {
	ctx := context.Background()
	c := New(t.TempDir())
	rel := releaseFor("test image")
	archive, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archive, []byte("corrupted!"), 0o600))
	_, err = c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)
	_, err = c.Store(ctx, rel, downloadContents(rel, "test image"))
	require.NoError(t, err)
	_, err = c.Lookup(ctx, rel)
	require.NoError(t, err)
}

func TestRejectUnsafeArchiveNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../image.tar.zst", `..\image.tar.zst`, "/image.tar.zst", `C:\image.tar.zst`} {
		t.Run(name, func(t *testing.T) {
			c := New(t.TempDir())
			rel := releaseFor("test image")
			rel.FileName = name
			called := false
			_, err := c.Store(context.Background(), rel, func(context.Context, string) error {
				called = true
				return nil
			})
			require.Error(t, err)
			require.False(t, called)
			entries, err := os.ReadDir(c.Dir())
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

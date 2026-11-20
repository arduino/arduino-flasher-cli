// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package store

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

func TestGetKeepsOnlyWhenRequested(t *testing.T) {
	ctx := context.Background()
	c := New(filepath.Join(t.TempDir(), "images"))
	rel := releaseFor("test image")
	_, err := c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)
	require.NoDirExists(t, c.Dir(), "looking up an image does not create the image store")

	archive, found, err := c.Get(ctx, rel, false, downloadContents(rel, "test image"))
	require.NoError(t, err)
	require.False(t, found)
	require.Equal(t, filepath.Join(c.Dir(), ".partial", rel.FileName), archive)
	_, err = c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)

	// A retry reuses the download, and keep moves it into the images directory.
	kept, found, err := c.Get(ctx, rel, true, func(context.Context, string) error {
		return errors.New("a retry must not download again")
	})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, filepath.Join(c.Dir(), rel.FileName), kept)
	c.Discard(kept)
	reused, err := c.Lookup(ctx, rel)
	require.NoError(t, err)
	require.Equal(t, kept, reused)
}

func TestGetResumesPartialDownload(t *testing.T) {
	ctx := context.Background()
	c := New(t.TempDir())
	rel := releaseFor("test image")
	connectionLost := errors.New("connection lost")
	_, _, err := c.Get(ctx, rel, true, func(ctx context.Context, dir string) error {
		require.NoError(t, downloadContents(rel, "test ")(ctx, dir))
		return connectionLost
	})
	require.ErrorIs(t, err, connectionLost)
	require.FileExists(t, filepath.Join(c.Dir(), ".partial", rel.FileName), "an interrupted download is kept for resume")

	archive, _, err := c.Get(ctx, rel, true, func(_ context.Context, dir string) error {
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

func TestGetReplacesDamagedArchive(t *testing.T) {
	ctx := context.Background()
	c := New(t.TempDir())
	rel := releaseFor("test image")
	archive, _, err := c.Get(ctx, rel, true, downloadContents(rel, "test image"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archive, []byte("corrupted!"), 0o600))
	_, err = c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)
	_, _, err = c.Get(ctx, rel, false, downloadContents(rel, "test image"))
	require.NoError(t, err)
	require.NoFileExists(t, archive, "a damaged kept archive is removed")
	_, _, err = c.Get(ctx, rel, true, downloadContents(rel, "test image"))
	require.NoError(t, err)
	_, err = c.Lookup(ctx, rel)
	require.NoError(t, err)
}

func TestPruneKeepsOnlyKeptImages(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	c := New(root)
	rel := releaseFor("test image")
	kept, _, err := c.Get(ctx, rel, true, downloadContents(rel, "test image"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(c.Dir(), ".partial", "partial.tar.zst"), []byte("partial"), 0o600))
	oldLayout := filepath.Join(c.Dir(), "204e82090918505a")
	require.NoError(t, os.MkdirAll(oldLayout, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(oldLayout, "image.tar.zst"), []byte("old"), 0o600))
	temp, err := TempDir("extract-", root)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(temp.Join("payload").String(), []byte("crash"), 0o600))

	listing, err := c.List()
	require.NoError(t, err)
	require.Len(t, listing.Images, 1)
	require.Equal(t, int64(len("partial")), listing.PartialSize)
	require.Equal(t, int64(len("old")+len("crash")), listing.OtherSize)

	require.NoError(t, c.Prune())
	require.FileExists(t, kept)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the images directory stays")
	entries, err = os.ReadDir(c.Dir())
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the kept image stays")
	listing, err = c.List()
	require.NoError(t, err)
	require.Zero(t, listing.PartialSize)
	require.Zero(t, listing.OtherSize)
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClearRemovesOnlyCacheContents(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	c := New(filepath.Join(parent, "images"))
	rel := releaseFor("test image")
	_, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
	require.NoError(t, err)
	partial := filepath.Join(c.Dir(), ".download-abandoned")
	require.NoError(t, os.Mkdir(partial, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(partial, "image.part"), []byte("partial"), 0o600))
	for _, sibling := range []string{"tmp", "custom-downloads"} {
		dir := filepath.Join(parent, sibling)
		require.NoError(t, os.Mkdir(dir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "keep"), []byte(sibling), 0o600))
	}

	require.NoError(t, c.Clear(ctx))
	entries, err := os.ReadDir(c.Dir())
	require.NoError(t, err)
	require.Empty(t, entries)
	for _, sibling := range []string{"tmp", "custom-downloads"} {
		data, err := os.ReadFile(filepath.Join(parent, sibling, "keep"))
		require.NoError(t, err)
		require.Equal(t, sibling, string(data))
	}
	_, err = c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)
	require.NoError(t, c.Clear(ctx), "clearing twice should be harmless")
	_, err = c.Store(ctx, rel, downloadContents(rel, "test image"))
	require.NoError(t, err, "the cache must remain usable after clearing")
}

func TestClearRejectsRelativePaths(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o750))
	marker := filepath.Join(parent, "keep")
	require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o600))
	t.Chdir(child)
	for _, dir := range []string{"", ".", "..", "images"} {
		require.Error(t, New(dir).Clear(context.Background()))
	}
	require.FileExists(t, marker)
}

func TestCanceledListAndClearLeaveFilesUntouched(t *testing.T) {
	c := New(t.TempDir())
	marker := filepath.Join(c.Dir(), "keep")
	require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.List(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, c.Clear(ctx), context.Canceled)
	require.FileExists(t, marker)
}

func TestListAndClearDoNotFollowSymlinks(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "images"))
	require.NoError(t, os.Mkdir(c.Dir(), 0o750))
	outside := t.TempDir()
	marker := filepath.Join(outside, "keep")
	require.NoError(t, os.WriteFile(marker, []byte("outside the image cache"), 0o600))
	link := filepath.Join(c.Dir(), "linked-directory")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	listing, err := c.List(context.Background())
	require.NoError(t, err)
	require.Empty(t, listing.Images)
	require.Zero(t, listing.TotalSize, "external files must not be counted")
	require.NoError(t, c.Clear(context.Background()))
	require.FileExists(t, marker, "clearing a link must preserve its target")
	_, err = os.Lstat(link)
	require.ErrorIs(t, err, os.ErrNotExist)

	rootLink := filepath.Join(t.TempDir(), "linked-cache")
	require.NoError(t, os.Symlink(outside, rootLink))
	_, err = New(rootLink).List(context.Background())
	require.Error(t, err)
	require.Error(t, New(rootLink).Clear(context.Background()))
	require.FileExists(t, marker)
}

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

func TestListIncludesImagesAndIncompleteDownloadSize(t *testing.T) {
	ctx := context.Background()
	c := New(filepath.Join(t.TempDir(), "images"))
	first := releaseFor("first image")
	second := releaseFor("second image")
	second.FileName = "second.tar.zst"
	firstPath, err := c.Store(ctx, first, downloadContents(first, "first image"))
	require.NoError(t, err)
	secondPath, err := c.Store(ctx, second, downloadContents(second, "second image"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(c.Dir(), ".partial", "third.tar.zst"), []byte("partial"), 0o600))

	listing, err := c.List(ctx)
	require.NoError(t, err)
	require.Len(t, listing.Images, 2, "a partial download is not listed")
	paths := []string{listing.Images[0].Path, listing.Images[1].Path}
	require.ElementsMatch(t, []string{firstPath, secondPath}, paths)
	for _, image := range listing.Images {
		require.Equal(t, filepath.Base(image.Path), image.FileName)
		require.False(t, image.DownloadedAt.IsZero())
	}
	require.Equal(t, uint64(len("first image")+len("second image")+len("partial")), listing.TotalSize)
}

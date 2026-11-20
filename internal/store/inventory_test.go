// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package store

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
	firstPath, _, err := c.Get(ctx, first, true, downloadContents(first, "first image"))
	require.NoError(t, err)
	secondPath, _, err := c.Get(ctx, second, true, downloadContents(second, "second image"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(c.Dir(), ".partial", "third.tar.zst"), []byte("partial"), 0o600))

	listing, err := c.List()
	require.NoError(t, err)
	require.Len(t, listing.Images, 2, "a partial download is not listed")
	require.Equal(t, int64(len("partial")), listing.PartialSize)
	require.Zero(t, listing.OtherSize)
	names := []string{listing.Images[0].FileName, listing.Images[1].FileName}
	require.ElementsMatch(t, []string{filepath.Base(firstPath), filepath.Base(secondPath)}, names)
	for _, image := range listing.Images {
		require.False(t, image.DownloadedAt.IsZero())
	}
}

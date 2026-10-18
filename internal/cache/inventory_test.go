// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

func TestListIncludesImagesAndIncompleteDownloadSize(t *testing.T) {
	ctx := context.Background()
	c := New(filepath.Join(t.TempDir(), "images"))
	first := releaseFor("first image")
	second := releaseFor("second image")
	second.Board, second.OS = registry.VentunoQ.ID, "ubuntu"
	firstPath, err := c.Store(ctx, first, downloadContents(first, "first image"))
	require.NoError(t, err)
	secondPath, err := c.Store(ctx, second, downloadContents(second, "second image"))
	require.NoError(t, err)
	firstMeta, err := os.ReadFile(filepath.Join(filepath.Dir(firstPath), metadataFile))
	require.NoError(t, err)
	secondMeta, err := os.ReadFile(filepath.Join(filepath.Dir(secondPath), metadataFile))
	require.NoError(t, err)
	partial := filepath.Join(c.Dir(), ".download-abandoned")
	require.NoError(t, os.Mkdir(partial, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(partial, "image.part"), []byte("partial"), 0o600))

	listing, err := c.List(ctx)
	require.NoError(t, err)
	require.Len(t, listing.Images, 2)
	require.Equal(t, first.Board, listing.Images[0].Board)
	require.Equal(t, first.Version, listing.Images[0].Version)
	require.Equal(t, firstPath, listing.Images[0].Path)
	require.Equal(t, int64(len("first image")), listing.Images[0].Size)
	require.False(t, listing.Images[0].DownloadedAt.IsZero())
	require.Equal(t, secondPath, listing.Images[1].Path)
	expectedSize := len("first image") + len("second image") + len(firstMeta) + len(secondMeta) + len("partial")
	require.Equal(t, uint64(expectedSize), listing.TotalSize)
	// Listing neither removes partial files nor rewrites stored metadata.
	unchanged, err := os.ReadFile(filepath.Join(filepath.Dir(firstPath), metadataFile))
	require.NoError(t, err)
	require.Equal(t, firstMeta, unchanged)
	require.FileExists(t, filepath.Join(partial, "image.part"))
}

func TestListSkipsUnusableRecords(t *testing.T) {
	for _, change := range []string{missingMetadataCase, invalidMetadataCase, missingArchiveCase, "wrong size", "wrong identity", "unsafe archive name"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			c := New(t.TempDir())
			rel := releaseFor("test image")
			archive, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
			require.NoError(t, err)
			metaPath := filepath.Join(filepath.Dir(archive), metadataFile)
			switch change {
			case missingMetadataCase:
				require.NoError(t, os.Remove(metaPath))
			case invalidMetadataCase:
				require.NoError(t, os.WriteFile(metaPath, []byte("{"), 0o600))
			case missingArchiveCase:
				require.NoError(t, os.Remove(archive))
			case "wrong size":
				require.NoError(t, os.Truncate(archive, 3))
			default:
				data, err := os.ReadFile(metaPath)
				require.NoError(t, err)
				var entry Entry
				require.NoError(t, json.Unmarshal(data, &entry))
				if change == "wrong identity" {
					entry.Version = "another-version"
				} else {
					entry.FileName = "../../outside"
				}
				data, err = json.Marshal(entry)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(metaPath, data, 0o600))
			}
			listing, err := c.List(ctx)
			require.NoError(t, err)
			require.Empty(t, listing.Images)
			require.Positive(t, listing.TotalSize, "unusable entries still occupy cache space")
		})
	}
}

func TestListIsInventoryRatherThanChecksumVerification(t *testing.T) {
	ctx := context.Background()
	c := New(t.TempDir())
	rel := releaseFor("test image")
	archive, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archive, []byte("corrupted!"), 0o600))
	listing, err := c.List(ctx)
	require.NoError(t, err)
	require.Len(t, listing.Images, 1, "listing inspects metadata and sizes without reading multi-GB archives")
	_, err = c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss, "reuse must still detect same-size corruption")
}

func TestMissingCacheIsEmpty(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "images"))
	listing, err := c.List(context.Background())
	require.NoError(t, err)
	require.NotNil(t, listing.Images, "JSON should contain an empty array, not null")
	require.Empty(t, listing.Images)
	require.Zero(t, listing.TotalSize)
	require.NoError(t, c.Clear(context.Background()))
	require.NoDirExists(t, c.Dir())
}

func TestListAndClearRejectFileAsCacheDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "images")
	require.NoError(t, os.WriteFile(file, []byte("keep"), 0o600))
	c := New(file)
	_, err := c.List(context.Background())
	require.Error(t, err)
	require.Error(t, c.Clear(context.Background()))
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package interactive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

func wizardRelease() registry.Release {
	sum := sha256.Sum256([]byte("test image"))
	return registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Sha256: hex.EncodeToString(sum[:]), FileName: "image.tar.zst",
		Url: "https://images.example/image.tar.zst", Latest: true,
	}
}

func wizardEntry(rel registry.Release) cache.Entry {
	return cache.Entry{
		Board: rel.Board, OS: rel.OS, Version: rel.Version,
		SHA256: rel.Sha256, FileName: rel.FileName,
	}
}

func TestImageOptionsCacheLabels(t *testing.T) {
	newest := wizardRelease()
	older := newest
	older.Version, older.Latest = "20250914-172", false
	images := registry.Releases{newest, older}
	for _, cached := range []string{"none", "newest", "older"} {
		t.Run(cached, func(t *testing.T) {
			var inventory cache.Inventory
			labels := []string{newest.Version + " (latest)", older.Version}
			switch cached {
			case "newest":
				inventory.Images = []cache.Entry{wizardEntry(newest)}
				labels[0] += " (cached)"
			case "older":
				inventory.Images = []cache.Entry{wizardEntry(older)}
				labels[1] += " (cached)"
			}
			options := imageOptions(images, inventory)
			require.Len(t, options, len(images))
			for i, option := range options {
				require.Equal(t, labels[i], option.Key)
				require.Equal(t, images[i], option.Value, "labels must not change the release used for flashing")
			}
		})
	}
}

func TestImageOptionsCacheStatusAcrossBoardsAndOS(t *testing.T) {
	debian := wizardRelease()
	ubuntu := debian
	ubuntu.OS = "ubuntu"
	otherBoard := debian
	otherBoard.Board = registry.VentunoQ.ID
	images := registry.Releases{debian, ubuntu, otherBoard}
	inventory := cache.Inventory{Images: []cache.Entry{wizardEntry(debian)}}

	// The generic image selector offers more than one OS. A cached archive of
	// one distribution must not mark the same version of another distribution.
	options := imageOptions(images.Filter("", registry.UnoQ.ID), inventory)
	require.Len(t, options, 2)
	require.Equal(t, debian.Version+" (latest) — debian (cached)", options[0].Key)
	require.Equal(t, ubuntu.Version+" — ubuntu", options[1].Key)

	// Options are rebuilt when switching boards using the same inventory.
	options = imageOptions(images.Filter("", registry.VentunoQ.ID), inventory)
	require.Len(t, options, 1)
	require.Equal(t, otherBoard.Version+" (latest)", options[0].Key)
	require.Equal(t, otherBoard, options[0].Value)
}

func TestImageOptionsRejectsStaleMetadata(t *testing.T) {
	for _, change := range []string{"checksum", "archive name"} {
		t.Run(change, func(t *testing.T) {
			rel := wizardRelease()
			entry := wizardEntry(rel)
			if change == "checksum" {
				entry.SHA256 = strings.Repeat("a", 64)
			} else {
				entry.FileName = "previous-name.tar.zst"
			}
			options := imageOptions(registry.Releases{rel}, cache.Inventory{Images: []cache.Entry{entry}})
			require.Len(t, options, 1)
			require.NotContains(t, options[0].Key, "(cached)")
		})
	}
}

func setupWizardCache(t *testing.T) *cache.Cache {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", t.TempDir())
	case "darwin":
		t.Setenv("HOME", t.TempDir())
	default:
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
	}
	c, err := cache.Default()
	require.NoError(t, err)
	return c
}

func TestWizardCacheSnapshotDoesNotVerifyArchiveContents(t *testing.T) {
	c := setupWizardCache(t)
	rel := wizardRelease()
	ctx := context.Background()
	archive, err := c.Store(ctx, rel, func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), []byte("test image"), 0o600)
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archive, []byte("corrupted!"), 0o600))
	inventory := readImageCache(ctx)
	options := imageOptions(registry.Releases{rel}, inventory)
	require.Len(t, options, 1)
	require.Contains(t, options[0].Key, "(cached)")
	_, err = c.Lookup(ctx, rel)
	require.ErrorIs(t, err, cache.ErrMiss, "flashing must still detect same-size corruption")

	// Rebuilding options uses the snapshot; it does not access the cache again.
	require.NoError(t, c.Clear(ctx))
	require.Equal(t, options, imageOptions(registry.Releases{rel}, inventory))
	require.Empty(t, readImageCache(ctx).Images)
}

func TestWizardUnavailableCacheDoesNotHideVersions(t *testing.T) {
	for _, state := range []string{"missing", "unreadable"} {
		t.Run(state, func(t *testing.T) {
			c := setupWizardCache(t)
			if state == "unreadable" {
				require.NoError(t, os.MkdirAll(filepath.Dir(c.Dir()), 0o750))
				require.NoError(t, os.WriteFile(c.Dir(), []byte("not a directory"), 0o600))
			}
			inventory := readImageCache(context.Background())
			require.Empty(t, inventory.Images)
			rel := wizardRelease()
			options := imageOptions(registry.Releases{rel}, inventory)
			require.Len(t, options, 1)
			require.Equal(t, rel, options[0].Value)
			require.NotContains(t, options[0].Key, "(cached)")
			if state == "missing" {
				require.NoDirExists(t, c.Dir())
			}
		})
	}
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

const (
	missingMetadataCase = "missing metadata"
	invalidMetadataCase = "invalid metadata"
	missingArchiveCase  = "missing archive"
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
	rel := releaseFor("downloaded image")
	_, err := c.Lookup(ctx, rel)
	require.ErrorIs(t, err, ErrMiss)
	require.NoDirExists(t, c.Dir(), "looking up an image does not create the cache")

	calls := 0
	download := func(ctx context.Context, dir string) error {
		calls++
		_, err := c.Lookup(ctx, rel)
		require.ErrorIs(t, err, ErrMiss, "an in-progress download must not be visible")
		return downloadContents(rel, "downloaded image")(ctx, dir)
	}
	archive, err := c.Store(ctx, rel, download)
	require.NoError(t, err)
	require.FileExists(t, archive)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(archive), metadataFile))
	require.NoError(t, err)
	var meta Entry
	require.NoError(t, json.Unmarshal(data, &meta))
	require.Equal(t, rel.Board, meta.Board)
	require.Equal(t, rel.OS, meta.OS)
	require.Equal(t, rel.Version, meta.Version)
	require.Equal(t, rel.Sha256, meta.SHA256)
	require.Equal(t, rel.FileName, meta.FileName)
	require.Equal(t, int64(len("downloaded image")), meta.Size)
	require.False(t, meta.DownloadedAt.IsZero())

	// The manifest's resolved release has the same identity whether the user
	// requested "latest" or its explicit version; Latest is only a UI marker.
	rel.Latest = true
	reused, err := c.Store(ctx, rel, download)
	require.NoError(t, err)
	require.Equal(t, archive, reused)
	require.Equal(t, 1, calls, "a cache hit does not invoke the downloader")
	entries, err := os.ReadDir(c.Dir())
	require.NoError(t, err)
	require.Len(t, entries, 1, "successful downloads leave no staging directories")
}

func TestStoreReusesEntryPublishedDuringDownload(t *testing.T) {
	ctx := context.Background()
	c := New(t.TempDir())
	other := New(c.Dir()) // Another CLI or daemon instance shares the directory.
	rel := releaseFor("test image")
	var published string
	var originalInfo os.FileInfo
	var originalMetadata []byte
	archive, err := c.Store(ctx, rel, func(ctx context.Context, staging string) error {
		// Both requests saw a miss, but the other download finishes first.
		var err error
		published, err = other.Store(ctx, rel, downloadContents(rel, "test image"))
		if err != nil {
			return err
		}
		reader, err := os.Open(published)
		if err != nil {
			return err
		}
		t.Cleanup(func() { _ = reader.Close() })
		originalInfo, err = reader.Stat()
		if err != nil {
			return err
		}
		originalMetadata, err = os.ReadFile(filepath.Join(filepath.Dir(published), metadataFile))
		if err != nil {
			return err
		}
		// Keep the first archive open as an extractor would. Windows cannot
		// replace it, and other platforms must also preserve the valid entry.
		return downloadContents(rel, "test image")(ctx, staging)
	})
	require.NoError(t, err)
	require.Equal(t, published, archive)
	info, err := os.Stat(archive)
	require.NoError(t, err)
	require.True(t, os.SameFile(originalInfo, info), "a concurrent download must not replace the published archive")
	metadata, err := os.ReadFile(filepath.Join(filepath.Dir(archive), metadataFile))
	require.NoError(t, err)
	require.Equal(t, originalMetadata, metadata, "reuse preserves the original download metadata")
	_, err = c.Lookup(ctx, rel)
	require.NoError(t, err)
	entries, err := os.ReadDir(c.Dir())
	require.NoError(t, err)
	require.Len(t, entries, 1, "the redundant download must leave no staging directory")
}

func TestLookupRejectsIncompleteAndCorruptEntries(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, string)
	}{
		{missingMetadataCase, func(t *testing.T, archive string) {
			require.NoError(t, os.Remove(filepath.Join(filepath.Dir(archive), metadataFile)))
		}},
		{invalidMetadataCase, func(t *testing.T, archive string) {
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(archive), metadataFile), []byte("{"), 0o600))
		}},
		{"wrong identity in metadata", func(t *testing.T, archive string) {
			path := filepath.Join(filepath.Dir(archive), metadataFile)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var meta Entry
			require.NoError(t, json.Unmarshal(data, &meta))
			meta.Version = "another-version"
			data, err = json.Marshal(meta)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))
		}},
		{"wrong size in metadata", func(t *testing.T, archive string) {
			path := filepath.Join(filepath.Dir(archive), metadataFile)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var meta Entry
			require.NoError(t, json.Unmarshal(data, &meta))
			meta.Size++
			data, err = json.Marshal(meta)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))
		}},
		{missingArchiveCase, func(t *testing.T, archive string) {
			require.NoError(t, os.Remove(archive))
		}},
		{"truncated archive", func(t *testing.T, archive string) {
			require.NoError(t, os.Truncate(archive, 3))
		}},
		{"same-size corruption", func(t *testing.T, archive string) {
			require.NoError(t, os.WriteFile(archive, []byte("corrupted!"), 0o600))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			c := New(t.TempDir())
			rel := releaseFor("test image")
			archive, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
			require.NoError(t, err)
			tt.change(t, archive)
			_, err = c.Lookup(ctx, rel)
			require.ErrorIs(t, err, ErrMiss)

			// A miss can be repaired by a fresh verified download.
			repaired, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
			require.NoError(t, err)
			require.Equal(t, archive, repaired)
			_, err = c.Lookup(ctx, rel)
			require.NoError(t, err)
		})
	}
}

func TestFailedDownloadIsNotCached(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		name := "checksum mismatch"
		if interrupted {
			name = "download interrupted"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			c := New(t.TempDir())
			rel := releaseFor("complete image")
			downloadErr := errors.New("connection lost")
			_, err := c.Store(ctx, rel, func(ctx context.Context, dir string) error {
				if err := downloadContents(rel, "partial")(ctx, dir); err != nil {
					return err
				}
				if interrupted {
					return downloadErr
				}
				return nil
			})
			if interrupted {
				require.ErrorIs(t, err, downloadErr)
			} else {
				require.ErrorIs(t, err, errChecksumMismatch)
			}
			_, err = c.Lookup(ctx, rel)
			require.ErrorIs(t, err, ErrMiss)
			entries, err := os.ReadDir(c.Dir())
			require.NoError(t, err)
			require.Empty(t, entries, "failed downloads leave no partial cache entries")
		})
	}
}

func TestReleaseIdentity(t *testing.T) {
	ctx := context.Background()
	c := New(t.TempDir())
	rel := releaseFor("test image")
	archive, err := c.Store(ctx, rel, downloadContents(rel, "test image"))
	require.NoError(t, err)

	for _, field := range []string{"board", "os", "version", "checksum"} {
		t.Run(field, func(t *testing.T) {
			other := rel
			contents := "test image"
			switch field {
			case "board":
				other.Board = registry.VentunoQ.ID
			case "os":
				other.OS = "ubuntu"
			case "version":
				other.Version = "20250915-174"
			case "checksum":
				contents = "republished image"
				other.Sha256 = releaseFor(contents).Sha256
			}
			_, err := c.Lookup(ctx, other)
			require.ErrorIs(t, err, ErrMiss, "distinct images must not share an entry")
			otherArchive, err := c.Store(ctx, other, downloadContents(other, contents))
			require.NoError(t, err)
			require.NotEqual(t, archive, otherArchive)
			_, err = c.Lookup(ctx, rel)
			require.NoError(t, err, "storing another image must not overwrite the original")
		})
	}
	rel.OS = strings.ToUpper(rel.OS)
	rel.Sha256 = strings.ToUpper(rel.Sha256)
	reused, err := c.Lookup(ctx, rel)
	require.NoError(t, err)
	require.Equal(t, archive, reused, "OS and hexadecimal checksum spelling do not change identity")
}

func TestRejectUnsafeArchiveNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../image.tar.zst", `..\image.tar.zst`, "/image.tar.zst", `C:\image.tar.zst`, "meta.json", "META.JSON", publicationLockFile, strings.ToUpper(publicationLockFile)} {
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

func TestCancellationDoesNotPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := New(t.TempDir())
	rel := releaseFor("test image")
	_, err := c.Store(ctx, rel, func(ctx context.Context, dir string) error {
		if err := downloadContents(rel, "test image")(ctx, dir); err != nil {
			return err
		}
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	_, err = c.Lookup(context.Background(), rel)
	require.ErrorIs(t, err, ErrMiss)
	entries, err := os.ReadDir(c.Dir())
	require.NoError(t, err)
	require.Empty(t, entries)
}

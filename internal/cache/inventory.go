// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// Inventory contains complete image records and the total size of regular files
// in the cache, including metadata and incomplete downloads. Sizes are in bytes.
type Inventory struct {
	Images    []Entry `json:"images"`
	TotalSize uint64  `json:"total_size"`
}

// List inspects local metadata and file sizes without network access or hashing
// multi-gigabyte archives. Lookup still verifies the checksum before reuse.
// A missing cache is empty; listing it does not create any directories.
func (c *Cache) List(ctx context.Context) (Inventory, error) {
	result := Inventory{Images: []Entry{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	root, err := c.openRoot()
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer root.Close()
	rootFS := root.FS()
	dirs, err := fs.ReadDir(rootFS, ".")
	if err != nil {
		return result, fmt.Errorf("could not list image cache: %w", err)
	}
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !dir.IsDir() || strings.HasPrefix(dir.Name(), ".") {
			continue
		}
		entry, err := c.readEntry(root, dir.Name())
		if errors.Is(err, ErrMiss) {
			continue
		}
		if err != nil {
			return result, err
		}
		result.Images = append(result.Images, entry)
	}
	// WalkDir does not follow symbolic links, and Root keeps access within the
	// opened cache directory even if another process renames its children.
	err = fs.WalkDir(rootFS, ".", func(_ string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		size := info.Size()
		if info.Mode().IsRegular() && size >= 0 {
			result.TotalSize += uint64(size)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("could not measure image cache: %w", err)
	}
	slices.SortFunc(result.Images, func(a, b Entry) int {
		return cmp.Or(
			strings.Compare(a.Board, b.Board),
			strings.Compare(a.OS, b.OS),
			b.DownloadedAt.Compare(a.DownloadedAt),
			strings.Compare(b.Version, a.Version),
			strings.Compare(a.SHA256, b.SHA256),
		)
	})
	return result, nil
}

func (c *Cache) readEntry(root *os.Root, name string) (Entry, error) {
	metaPath := path.Join(name, metadataFile)
	info, err := root.Lstat(metaPath)
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, ErrMiss
	}
	if err != nil {
		return Entry{}, fmt.Errorf("could not inspect cached image metadata: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Entry{}, ErrMiss
	}
	data, err := fs.ReadFile(root.FS(), metaPath)
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, ErrMiss
	}
	if err != nil {
		return Entry{}, fmt.Errorf("could not read cached image metadata: %w", err)
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return Entry{}, ErrMiss
	}
	dir, err := c.entryDir(registry.Release{
		Board: entry.Board, OS: entry.OS, Version: entry.Version,
		Sha256: entry.SHA256, FileName: entry.FileName,
	})
	if err != nil || filepath.Base(dir) != name || entry.DownloadedAt.IsZero() || entry.Size < 0 {
		return Entry{}, ErrMiss
	}
	info, err = root.Lstat(path.Join(name, entry.FileName))
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, ErrMiss
	}
	if err != nil {
		return Entry{}, fmt.Errorf("could not inspect cached image archive: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != entry.Size {
		return Entry{}, ErrMiss
	}
	entry.Path = filepath.Join(c.dir, name, entry.FileName)
	return entry, nil
}

// openRoot rejects a symlink in place of the cache directory. Children are
// accessed through a bounded directory handle rather than metadata-supplied paths.
func (c *Cache) openRoot() (*os.Root, error) {
	info, err := os.Lstat(c.dir)
	if err != nil {
		return nil, fmt.Errorf("could not inspect image cache: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("image cache %s must be a directory, not a file or symbolic link", c.dir)
	}
	root, err := os.OpenRoot(c.dir)
	if err != nil {
		return nil, fmt.Errorf("could not open image cache: %w", err)
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, fmt.Errorf("image cache directory changed while opening %s", c.dir)
	}
	return root, nil
}

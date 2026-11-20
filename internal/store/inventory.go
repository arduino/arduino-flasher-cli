// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// Inventory contains the kept archives, the bytes in the partial directory, and the
// bytes of everything else in the store, such as directories that a crash left.
type Inventory struct {
	Images      []Entry
	PartialSize int64
	OtherSize   int64
}

// Entry describes a kept archive.
type Entry struct {
	FileName     string
	Size         int64
	DownloadedAt time.Time
}

// Contains reports whether the archive of rel is kept. Lookup still checks its hash.
func (inventory Inventory) Contains(rel registry.Release) bool {
	return slices.ContainsFunc(inventory.Images, func(entry Entry) bool {
		return entry.FileName == rel.FileName
	})
}

// List reads the kept archives without network access or hashing.
// A missing store is empty; listing it does not create any directories.
func (s *Store) List() (Inventory, error) {
	result := Inventory{Images: []Entry{}}
	files, err := os.ReadDir(s.Dir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("could not list the image store: %w", err)
	}
	for _, file := range files {
		if !isKept(file) {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return result, err
		}
		result.Images = append(result.Images, Entry{
			FileName:     file.Name(),
			Size:         info.Size(),
			DownloadedAt: info.ModTime().UTC(),
		})
	}
	others, err := s.others()
	if err != nil {
		return result, err
	}
	for _, other := range others {
		size := &result.OtherSize
		if other == s.partialDir() {
			size = &result.PartialSize
		}
		err := filepath.WalkDir(other, func(_ string, entry fs.DirEntry, err error) error {
			if err != nil || !entry.Type().IsRegular() {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			*size += info.Size()
			return nil
		})
		if err != nil {
			return result, fmt.Errorf("could not measure the image store: %w", err)
		}
	}
	slices.SortFunc(result.Images, func(a, b Entry) int {
		return cmp.Or(b.DownloadedAt.Compare(a.DownloadedAt), strings.Compare(a.FileName, b.FileName))
	})
	return result, nil
}

// others returns the paths in the store that are not kept archives.
func (s *Store) others() ([]string, error) {
	var others []string
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not list the image store: %w", err)
	}
	for _, entry := range entries {
		if path := filepath.Join(s.root, entry.Name()); path != s.Dir() {
			others = append(others, path)
		}
	}
	images, err := os.ReadDir(s.Dir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("could not list the image store: %w", err)
	}
	for _, image := range images {
		if !isKept(image) {
			others = append(others, filepath.Join(s.Dir(), image.Name()))
		}
	}
	return others, nil
}

func isKept(entry fs.DirEntry) bool {
	return entry.Type().IsRegular() && !strings.HasPrefix(entry.Name(), ".")
}

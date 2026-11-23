// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Stored returns the file names of the stored archives. A missing store has none.
func (s *Store) Stored() ([]string, error) {
	entries, err := os.ReadDir(s.Dir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("could not list the image store: %w", err)
	}
	var stored []string
	for _, entry := range entries {
		if isStored(entry) {
			stored = append(stored, entry.Name())
		}
	}
	return stored, nil
}

// Remove removes the stored archive with the given file name.
func (s *Store) Remove(name string) error {
	path := filepath.Join(s.Dir(), name)
	info, err := os.Lstat(path)
	if err != nil || filepath.Base(name) != name || !isStored(fs.FileInfoToDirEntry(info)) {
		return fmt.Errorf("%s is not a stored image", name)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("could not remove the stored image %s: %w", name, err)
	}
	return nil
}

// Prune removes everything in the store except the stored archives, and returns the freed bytes.
func (s *Store) Prune() (int64, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("could not list the image store: %w", err)
	}
	var others []string
	for _, entry := range entries {
		if path := filepath.Join(s.root, entry.Name()); path != s.Dir() {
			others = append(others, path)
		}
	}
	images, err := os.ReadDir(s.Dir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("could not list the image store: %w", err)
	}
	for _, image := range images {
		if !isStored(image) {
			others = append(others, filepath.Join(s.Dir(), image.Name()))
		}
	}
	var freed int64
	for _, other := range others {
		_ = filepath.WalkDir(other, func(_ string, entry fs.DirEntry, err error) error {
			if err == nil && entry.Type().IsRegular() {
				if info, err := entry.Info(); err == nil {
					freed += info.Size()
				}
			}
			return nil
		})
		if err := os.RemoveAll(other); err != nil {
			return freed, fmt.Errorf("could not remove %s: %w", other, err)
		}
	}
	return freed, nil
}

func isStored(entry fs.DirEntry) bool {
	return entry.Type().IsRegular() && !strings.HasPrefix(entry.Name(), ".")
}

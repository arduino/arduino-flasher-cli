// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// Inventory contains the cached archives.
type Inventory struct {
	Images []Entry `json:"images"`
}

// Contains reports whether the archive of rel is cached. Lookup still checks its hash.
func (inventory Inventory) Contains(rel registry.Release) bool {
	return slices.ContainsFunc(inventory.Images, func(entry Entry) bool {
		return entry.FileName == rel.FileName
	})
}

// List reads the cached archives without network access or hashing.
// A missing cache is empty; listing it does not create any directories.
func (c *Cache) List(ctx context.Context) (Inventory, error) {
	result := Inventory{Images: []Entry{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	files, err := os.ReadDir(c.dir)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("could not list image cache: %w", err)
	}
	for _, file := range files {
		if !file.Type().IsRegular() || strings.HasPrefix(file.Name(), ".") {
			continue
		}
		info, err := file.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return result, err
		}
		result.Images = append(result.Images, Entry{
			FileName:     file.Name(),
			Size:         info.Size(),
			DownloadedAt: info.ModTime().UTC(),
			Path:         filepath.Join(c.dir, file.Name()),
		})
	}
	slices.SortFunc(result.Images, func(a, b Entry) int {
		return cmp.Or(b.DownloadedAt.Compare(a.DownloadedAt), strings.Compare(a.FileName, b.FileName))
	})
	return result, nil
}

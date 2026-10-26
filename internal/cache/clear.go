// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Clear removes images and incomplete downloads, preserving the directory,
// its siblings and the persistent publication locks used by other processes.
// Symbolic links are removed without deleting their targets. A missing cache
// is already clear.
func (c *Cache) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Clean(c.dir)
	if !filepath.IsAbs(dir) || filepath.Dir(dir) == dir {
		return errors.New("clearing the image cache requires an absolute path below the filesystem root")
	}
	root, err := c.openRoot()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return fmt.Errorf("could not list image cache for clearing: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name() == publicationLocksDir {
			continue
		}
		if err := root.RemoveAll(entry.Name()); err != nil {
			return fmt.Errorf("could not clear cached item %s: %w", entry.Name(), err)
		}
	}
	return nil
}

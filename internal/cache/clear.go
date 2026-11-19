// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Clear removes images and incomplete downloads, but not the directory and its
// siblings. A missing cache is already clear.
func (c *Cache) Clear(ctx context.Context) error {
	entries, err := os.ReadDir(c.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not list image cache for clearing: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(c.dir, entry.Name())); err != nil {
			return fmt.Errorf("could not clear cached item %s: %w", entry.Name(), err)
		}
	}
	return nil
}

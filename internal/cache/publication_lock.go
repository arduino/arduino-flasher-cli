// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	publicationLockFile = ".publish.lock"
	publicationLocksDir = ".locks"
)

// lockPublication serializes publishers of one cache entry across processes.
// Closing the returned file or terminating its process releases the OS lock.
// Empty files stay in a separate directory, preserved by Clear, so waiters
// always lock the same file even when an image directory is replaced.
func lockPublication(ctx context.Context, dir, key string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("could not open cache publication directory: %w", err)
	}
	defer root.Close()
	if err := root.MkdirAll(publicationLocksDir, 0o750); err != nil {
		return nil, fmt.Errorf("could not create cache publication locks directory: %w", err)
	}
	file, err := root.OpenFile(filepath.Join(publicationLocksDir, key+publicationLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("could not open cache publication lock: %w", err)
	}
	locked := false
	defer func() {
		if !locked {
			_ = file.Close()
		}
	}()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		locked, err = tryPublicationLock(file)
		if err != nil {
			return nil, fmt.Errorf("could not lock cache publication: %w", err)
		}
		if locked {
			return file, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"fmt"
	"os"
	"time"
)

const publicationLockFile = ".publish.lock"

// lockPublication serializes publishers of one cache entry across processes.
// Closing the returned file or terminating its process releases the OS lock.
// The empty file stays in place so waiters always lock the same file.
func lockPublication(ctx context.Context, dir string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("could not open cache publication directory: %w", err)
	}
	defer root.Close()
	file, err := root.OpenFile(publicationLockFile, os.O_CREATE|os.O_RDWR, 0o600)
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

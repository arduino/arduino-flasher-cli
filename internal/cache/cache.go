// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package cache stores downloaded image archives for reuse by the CLI and daemon.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// ErrMiss means that no complete, valid archive is cached for the release.
var ErrMiss = errors.New("image is not cached")

var errChecksumMismatch = errors.New("archive checksum does not match the manifest")

// Cache holds image archives in a directory separate from extraction scratch space.
type Cache struct {
	dir string
}

// New uses dir as the images directory. It creates nothing until Store is called.
func New(dir string) *Cache {
	return &Cache{dir: dir}
}

// Default uses the operating system's cache directory for the current user.
func Default() (*Cache, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not find the user cache directory: %w", err)
	}
	return New(filepath.Join(dir, "arduino-flasher-cli", "images")), nil
}

// Dir returns the images directory.
func (c *Cache) Dir() string {
	return c.dir
}

// Entry describes a cached archive. The release fields are empty: the online index has them.
type Entry struct {
	Board        string    `json:"board,omitempty"`
	OS           string    `json:"os,omitempty"`
	Version      string    `json:"version,omitempty"`
	SHA256       string    `json:"sha256,omitempty"`
	FileName     string    `json:"file_name"`
	Size         int64     `json:"size"`
	DownloadedAt time.Time `json:"downloaded_at"`
	Path         string    `json:"path,omitempty"`
}

// Lookup returns the cached archive of rel after it checks the archive hash.
// A missing archive or a hash mismatch is a cache miss.
func (c *Cache) Lookup(ctx context.Context, rel registry.Release) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	archive, err := c.archivePath(rel)
	if err != nil {
		return "", err
	}
	_, err = verifyArchive(ctx, archive, rel.Sha256)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, errChecksumMismatch) {
		return "", ErrMiss
	}
	if err != nil {
		return "", err
	}
	return archive, nil
}

// Store runs download, which must write and verify rel.FileName in the partial
// directory. Call it after a Lookup miss. A failed download stays there for resume.
func (c *Cache) Store(ctx context.Context, rel registry.Release, download func(context.Context, string) error) (string, error) {
	if download == nil {
		return "", errors.New("no image download function provided")
	}
	archive, err := c.archivePath(rel)
	if err != nil {
		return "", err
	}
	partial := filepath.Join(c.dir, ".partial")
	if err := os.MkdirAll(partial, 0o750); err != nil {
		return "", fmt.Errorf("could not create image cache: %w", err)
	}
	if err := download(ctx, partial); err != nil {
		return "", err
	}
	// Rename fails on Windows if the archive exists.
	if err := os.Remove(archive); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("could not remove damaged cached image: %w", err)
	}
	if err := os.Rename(filepath.Join(partial, rel.FileName), archive); err != nil {
		return "", fmt.Errorf("could not move downloaded image into the cache: %w", err)
	}
	return archive, nil
}

func (c *Cache) archivePath(rel registry.Release) (string, error) {
	if !filepath.IsLocal(rel.FileName) || rel.FileName == "." || strings.ContainsAny(rel.FileName, `/\:`) {
		return "", fmt.Errorf("invalid image archive name %q", rel.FileName)
	}
	checksum, err := hex.DecodeString(rel.Sha256)
	if err != nil || len(checksum) != sha256.Size {
		return "", errors.New("a cached image requires a valid SHA-256 checksum")
	}
	return filepath.Join(c.dir, rel.FileName), nil
}

func verifyArchive(ctx context.Context, path, expected string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("could not open image archive: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errChecksumMismatch
	}
	hash := sha256.New()
	size, err := io.Copy(hash, contextReader{ctx: ctx, reader: f})
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
		return 0, errChecksumMismatch
	}
	return size, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

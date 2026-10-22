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
	"encoding/json"
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
var ErrMiss = errors.New("image is not cached or failed integrity verification")

var errChecksumMismatch = errors.New("archive checksum does not match the manifest")

const metadataFile = "meta.json"

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

// Entry describes a stored image. Path is populated when listing the cache;
// it is not needed in the metadata saved alongside an archive.
type Entry struct {
	Board        string    `json:"board"`
	OS           string    `json:"os"`
	Version      string    `json:"version"`
	SHA256       string    `json:"sha256"`
	FileName     string    `json:"file_name"`
	Size         int64     `json:"size"`
	DownloadedAt time.Time `json:"downloaded_at"`
	Path         string    `json:"path,omitempty"`
}

// Lookup returns a cached archive only after verifying it against rel, which the
// caller must resolve from the online manifest. Missing metadata, incomplete
// downloads and corrupt archives are cache misses; filesystem errors are returned.
func (c *Cache) Lookup(ctx context.Context, rel registry.Release) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir, err := c.entryDir(rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, metadataFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrMiss
	}
	if err != nil {
		return "", fmt.Errorf("could not read cached image metadata: %w", err)
	}
	var meta Entry
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", ErrMiss
	}
	if meta.Board != rel.Board || !strings.EqualFold(meta.OS, rel.OS) || meta.Version != rel.Version ||
		!strings.EqualFold(meta.SHA256, rel.Sha256) || meta.FileName != rel.FileName {
		return "", ErrMiss
	}
	archive := filepath.Join(dir, rel.FileName)
	_, err = verifyArchive(ctx, archive, rel.Sha256)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, errChecksumMismatch) {
		return "", ErrMiss
	}
	if err != nil {
		return "", err
	}
	return archive, nil
}

// Store reuses a valid entry or invokes download in a private staging directory.
// download must write rel.FileName into that directory and close it before
// returning. Only a verified archive is published, with metadata written last.
// A failed or interrupted download can never become a cache hit.
func (c *Cache) Store(ctx context.Context, rel registry.Release, download func(context.Context, string) error) (string, error) {
	if archive, err := c.Lookup(ctx, rel); err == nil {
		return archive, nil
	} else if !errors.Is(err, ErrMiss) {
		return "", err
	}
	if download == nil {
		return "", errors.New("no image download function provided")
	}
	dir, err := c.entryDir(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		return "", fmt.Errorf("could not create image cache: %w", err)
	}
	staging, err := os.MkdirTemp(c.dir, ".download-")
	if err != nil {
		return "", fmt.Errorf("could not create image download directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := download(ctx, staging); err != nil {
		return "", fmt.Errorf("could not download image for the cache: %w", err)
	}
	archive := filepath.Join(staging, rel.FileName)
	size, err := verifyArchive(ctx, archive, rel.Sha256)
	if err != nil {
		return "", fmt.Errorf("could not verify downloaded image: %w", err)
	}
	meta := Entry{
		Board:        rel.Board,
		OS:           strings.ToLower(rel.OS),
		Version:      rel.Version,
		SHA256:       strings.ToLower(rel.Sha256),
		FileName:     rel.FileName,
		Size:         size,
		DownloadedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", fmt.Errorf("could not encode cached image metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staging, metadataFile), data, 0o600); err != nil {
		return "", fmt.Errorf("could not write cached image metadata: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("could not create cached image directory: %w", err)
	}
	publicationLock, err := lockPublication(ctx, dir)
	if err != nil {
		return "", err
	}
	defer publicationLock.Close()
	// Another request may have published this release while we downloaded or
	// waited for the lock. Preserve its archive, even if it is open for flashing.
	if existing, err := c.Lookup(ctx, rel); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrMiss) {
		return "", err
	}
	destination := filepath.Join(dir, rel.FileName)
	if err := os.Rename(archive, destination); err != nil {
		return "", fmt.Errorf("could not publish cached image: %w", err)
	}
	if err := os.Rename(filepath.Join(staging, metadataFile), filepath.Join(dir, metadataFile)); err != nil {
		return "", fmt.Errorf("could not publish cached image metadata: %w", err)
	}
	return destination, nil
}

func (c *Cache) entryDir(rel registry.Release) (string, error) {
	if rel.Board == "" || rel.Version == "" {
		return "", errors.New("a cached image requires a board and a resolved version")
	}
	if !filepath.IsLocal(rel.FileName) || rel.FileName == "." || strings.ContainsAny(rel.FileName, `/\:`) || strings.EqualFold(rel.FileName, metadataFile) || strings.EqualFold(rel.FileName, publicationLockFile) {
		return "", fmt.Errorf("invalid image archive name %q", rel.FileName)
	}
	checksum, err := hex.DecodeString(rel.Sha256)
	if err != nil || len(checksum) != sha256.Size {
		return "", errors.New("a cached image requires a valid SHA-256 checksum")
	}
	// Hash the identity rather than using manifest fields as directory names.
	// This avoids path traversal and works with versions on every platform.
	// The checksum separates republished images of the same version, while the
	// resolved version makes "latest" and an explicit version share one entry.
	identity, err := json.Marshal([]string{
		rel.Board, strings.ToLower(rel.OS), rel.Version,
		strings.ToLower(rel.Sha256), rel.FileName,
	})
	if err != nil {
		return "", err
	}
	key := sha256.Sum256(identity)
	return filepath.Join(c.dir, hex.EncodeToString(key[:])), nil
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

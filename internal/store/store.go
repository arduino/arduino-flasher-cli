// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package store keeps downloaded image archives for reuse by the CLI and daemon.
package store

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

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// errMiss means that no complete, valid archive is stored for the release.
var errMiss = errors.New("image is not stored")

// Store holds the image archives in its images directory and the temporary
// extraction directories next to it.
type Store struct {
	root string
}

// New uses root as the store directory. It creates nothing until it writes a file.
func New(root string) *Store {
	return &Store{root: root}
}

// Default uses the operating system's cache directory for the current user.
func Default() (*Store, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("could not find the user cache directory: %w", err)
	}
	return New(filepath.Join(dir, "arduino-flasher-cli")), nil
}

// Dir returns the images directory.
func (s *Store) Dir() string {
	return filepath.Join(s.root, "images")
}

// TempDir creates a temporary directory in parent, or in the default store when
// parent is empty. The caller removes it; Prune removes one that a crash left.
func TempDir(prefix, parent string) (*paths.Path, error) {
	if parent == "" {
		s, err := Default()
		if err != nil {
			return nil, err
		}
		parent = s.root
		if err := os.MkdirAll(parent, 0o750); err != nil {
			return nil, fmt.Errorf("could not create the image store: %w", err)
		}
	}
	temp, err := paths.MkTempDir(parent, prefix)
	if err != nil {
		return nil, fmt.Errorf("could not create a temporary directory: %w", err)
	}
	return temp, nil
}

// Get returns the archive of rel and whether it was already on disk. download writes
// it in the partial directory, where it resumes; keep moves it to the images directory.
func (s *Store) Get(ctx context.Context, rel registry.Release, keep bool, download func(context.Context, string) error) (string, bool, error) {
	archive, err := s.lookup(ctx, rel)
	if !errors.Is(err, errMiss) {
		return archive, err == nil, err
	}
	// A stored archive that fails the checksum is damaged.
	_ = os.Remove(filepath.Join(s.Dir(), rel.FileName))
	partial := filepath.Join(s.partialDir(), rel.FileName)
	err = verifyArchive(ctx, partial, rel.Sha256)
	found := err == nil
	if errors.Is(err, errMiss) {
		if err := os.MkdirAll(s.partialDir(), 0o750); err != nil {
			return "", false, fmt.Errorf("could not create the image store: %w", err)
		}
		err = download(ctx, s.partialDir())
	}
	if err != nil {
		return "", false, err
	}
	if !keep {
		return partial, found, nil
	}
	archive = filepath.Join(s.Dir(), rel.FileName)
	if err := os.Rename(partial, archive); err != nil {
		return "", false, fmt.Errorf("could not move downloaded image into the image store: %w", err)
	}
	return archive, found, nil
}

// Discard removes an archive that Get did not keep, if it can. List reports one it cannot.
func (s *Store) Discard(archive string) {
	if filepath.Dir(archive) == s.partialDir() {
		_ = os.Remove(archive)
	}
}

// lookup returns the stored archive of rel after it checks the archive hash.
// A missing archive or a hash mismatch is errMiss.
func (s *Store) lookup(ctx context.Context, rel registry.Release) (string, error) {
	archive := filepath.Join(s.Dir(), rel.FileName)
	if err := verifyArchive(ctx, archive, rel.Sha256); err != nil {
		return "", err
	}
	return archive, nil
}

func (s *Store) partialDir() string {
	return filepath.Join(s.Dir(), ".partial")
}

func verifyArchive(ctx context.Context, path, expected string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return errMiss
	}
	if err != nil {
		return fmt.Errorf("could not open image archive: %w", err)
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx: ctx, reader: f}); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
		return errMiss
	}
	return nil
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

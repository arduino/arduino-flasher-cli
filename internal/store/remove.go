// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Prune removes everything in the store except the kept archives.
func (s *Store) Prune() error {
	others, err := s.others()
	if err != nil {
		return err
	}
	for _, other := range others {
		if err := os.RemoveAll(other); err != nil {
			return fmt.Errorf("could not remove %s: %w", other, err)
		}
	}
	return nil
}

// Remove removes the kept archive with the file name that List reports.
func (s *Store) Remove(name string) error {
	inventory, err := s.List()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(inventory.Images, func(image Entry) bool { return image.FileName == name }) {
		return fmt.Errorf("%s is not a kept image", name)
	}
	if err := os.Remove(filepath.Join(s.Dir(), name)); err != nil {
		return fmt.Errorf("could not remove the kept image %s: %w", name, err)
	}
	return nil
}

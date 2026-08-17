// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package selector resolves the image argument of the flash and download
// commands.
package selector

import (
	"fmt"
	"strings"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

const latest = "latest"

// Parse resolves the argument into where the image comes from: a path to one
// already on disk, or a release to fetch. A nil path means the release is set.
func Parse(arg string) (*paths.Path, registry.ImageRef, error) {
	if path, ok := localImagePath(arg); ok {
		return path, registry.ImageRef{}, nil
	}
	ref, err := ParseRef(arg)
	return nil, ref, err
}

// ParseRef resolves the argument into the image it names. It never looks
// at the filesystem, which is why download can use it directly.
func ParseRef(arg string) (registry.ImageRef, error) {
	// The connected board cannot be identified, so latest cannot pick an index.
	if arg == latest {
		return registry.ImageRef{}, fmt.Errorf("%q does not say which board to use, name one of: %s",
			latest, strings.Join(registry.Aliases(), ", "))
	}

	board, os, version := registry.SplitSelector(arg)
	switch {
	case board == "":
		// A bare version is UNO Q Debian until the indexes say otherwise.
		board, os, version = registry.UnoQ, registry.Debian, arg
	case version == "":
		version = latest
	}

	// Checked here so that a board with no images fails before the user is asked
	// to confirm a flash.
	if _, err := registry.IndexFor(board, os); err != nil {
		return registry.ImageRef{}, err
	}
	return registry.ImageRef{Version: version, Board: board, Os: os}, nil
}

// localImagePath reports whether the argument names an image already on disk.
// Selectors are matched first, so a file named after one cannot shadow it.
func localImagePath(arg string) (*paths.Path, bool) {
	if board, _, _ := registry.SplitSelector(arg); board != "" || arg == latest {
		return nil, false
	}
	p := paths.New(arg)
	if !p.Exist() {
		return nil, false
	}
	abs, err := p.Abs()
	if err != nil {
		return nil, false
	}
	return abs, true
}

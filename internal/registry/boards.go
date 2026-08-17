// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"fmt"
	"strings"
)

// ImageIndex is a published set of images.
type ImageIndex struct {
	Os Os
	// Path of the index on the download server.
	Path string
}

// ImageRef identifies an image to fetch: which board, which OS, which version.
type ImageRef struct {
	Version string
	Board   Board
	// Os is empty when the board publishes only one, which is then used.
	Os Os
}

// Index returns where the referenced image is published.
func (r ImageRef) Index() (ImageIndex, error) {
	return IndexFor(r.Board, r.Os)
}

// Variant is one storage configuration of a board.
type Variant struct {
	Label string
	// Capacity is the advertised eMMC size; the GPT reports slightly less.
	Capacity uint64
	// DefaultRootSize is the root size to use when none was requested. Zero
	// keeps the size the image ships with.
	DefaultRootSize uint64
}

type BoardDef struct {
	Board Board
	// Alias is what the board is called on the command line.
	Alias string
	// Images are the indexes publishing for this board. Empty means none do.
	Images   []ImageIndex
	Variants []Variant
}

// Supported is every board the flasher knows about.
var Supported = []BoardDef{
	{
		Board:  UnoQ,
		Alias:  "unoq",
		Images: []ImageIndex{{Os: Debian, Path: "debian-im/Stable"}},
		Variants: []Variant{
			{Label: "UNO Q (2GB RAM, 16GB storage)", Capacity: 16_000_000_000},
			{Label: "UNO Q (4GB RAM, 32GB storage)", Capacity: 32_000_000_000, DefaultRootSize: 20 * 1024 * 1024 * 1024},
		},
	},
	{
		Board: VentunoQ,
		Alias: "ventunoq",
	},
}

// BoardsWithImages returns the boards that can be flashed without a local image.
func BoardsWithImages() []BoardDef {
	var defs []BoardDef
	for _, def := range Supported {
		if len(def.Images) > 0 {
			defs = append(defs, def)
		}
	}
	return defs
}

// Aliases returns the command line names of the boards that can be flashed
// without a local image.
func Aliases() []string {
	defs := BoardsWithImages()
	aliases := make([]string, 0, len(defs))
	for _, def := range defs {
		aliases = append(aliases, def.Alias)
	}
	return aliases
}

func defFor(board Board) (BoardDef, error) {
	for _, def := range Supported {
		if def.Board == board {
			return def, nil
		}
	}
	return BoardDef{}, fmt.Errorf("unknown board %q", board)
}

// SplitSelector takes a board alias, optionally followed by an OS, off the front
// of a selector. What remains is the version, empty meaning the latest.
func SplitSelector(selector string) (Board, Os, string) {
	for _, def := range Supported {
		if selector == def.Alias {
			return def.Board, "", ""
		}
		rest, ok := strings.CutPrefix(selector, def.Alias+"-")
		if !ok {
			continue
		}
		for _, img := range def.Images {
			if rest == string(img.Os) {
				return def.Board, img.Os, ""
			}
			if version, ok := strings.CutPrefix(rest, string(img.Os)+"-"); ok {
				return def.Board, img.Os, version
			}
		}
		return def.Board, "", rest
	}
	return "", "", selector
}

// IndexFor returns the index publishing os for board. An empty os is allowed
// when the board has exactly one index.
func IndexFor(board Board, os Os) (ImageIndex, error) {
	def, err := defFor(board)
	if err != nil {
		return ImageIndex{}, err
	}
	if len(def.Images) == 0 {
		return ImageIndex{}, fmt.Errorf("no images are published for the %s yet: pass the path of a local image instead", board)
	}
	if os == "" {
		if len(def.Images) > 1 {
			return ImageIndex{}, fmt.Errorf("more than one OS is published for the %s: select one, for example %s-%s", board, def.Alias, def.Images[0].Os)
		}
		return def.Images[0], nil
	}
	for _, img := range def.Images {
		if img.Os == os {
			return img, nil
		}
	}
	return ImageIndex{}, fmt.Errorf("no %s images are published for the %s", os, board)
}

// VariantFor returns the smallest variant that can hold the reported capacity.
func VariantFor(board Board, reported uint64) (Variant, error) {
	def, err := defFor(board)
	if err != nil {
		return Variant{}, err
	}
	var best Variant
	for _, v := range def.Variants {
		if v.Capacity < reported {
			continue
		}
		if best.Capacity == 0 || v.Capacity < best.Capacity {
			best = v
		}
	}
	if best.Capacity == 0 {
		return Variant{}, fmt.Errorf("no known %s variant can hold %d bytes", board, reported)
	}
	return best, nil
}

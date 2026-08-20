// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import "fmt"

// Variant is one storage configuration of a board.
type Variant struct {
	Label string
	// Capacity is the advertised eMMC size; the GPT reports slightly less.
	Capacity uint64
	// DefaultRootSize is the root size to use when none was requested. Zero
	// keeps the size the image ships with.
	DefaultRootSize uint64
}

// Board is a board the flasher can target.
type Board struct {
	ID string
	// Label is how the board is presented to the user.
	Label string
	// DefaultOs is the index to look in when none is asked for. What is actually
	// published for a board is not listed here: every release carries its board,
	// so it is discoverable by reading the indexes.
	DefaultOs string
	Variants  []Variant
	// PreserveUser tells whether a flash can keep the user partition. It works by
	// reading the partition table of the board before writing, which is also what
	// sizing the root partition needs, so a board without it is flashed with the
	// layout the image ships with.
	PreserveUser bool
}

// The boards the flasher knows about. Code that targets one names it here; a
// board coming from the user is looked up with BoardByID instead.
var (
	UnoQ = Board{
		ID:           "unoq",
		Label:        "UNO Q",
		DefaultOs:    "debian",
		PreserveUser: true,
		Variants: []Variant{
			{Label: "UNO Q (2GB RAM, 16GB storage)", Capacity: 16_000_000_000},
			{Label: "UNO Q (4GB RAM, 32GB storage)", Capacity: 32_000_000_000, DefaultRootSize: 20 * 1024 * 1024 * 1024},
		},
	}

	VentunoQ = Board{
		ID:        "ventunoq",
		Label:     "VENTUNO Q",
		DefaultOs: "ubuntu",
	}
)

// Supported is every board the flasher knows about.
var Supported = []Board{UnoQ, VentunoQ}

// ResolveOs returns the index holding the images of this board: the one asked
// for, or the default of the board.
func (b Board) ResolveOs(requested string) string {
	if requested == "" {
		return b.DefaultOs
	}
	return requested
}

// VariantByCapacity returns the smallest variant that can hold the reported
// capacity. It is how the variant is told apart, since the capacity is read from
// the partition table of the board itself.
func (b Board) VariantByCapacity(reported uint64) (Variant, error) {
	var best Variant
	for _, v := range b.Variants {
		if v.Capacity < reported {
			continue
		}
		if best.Capacity == 0 || v.Capacity < best.Capacity {
			best = v
		}
	}
	if best.Capacity == 0 {
		return Variant{}, fmt.Errorf("no known %s variant can hold %d bytes", b.Label, reported)
	}
	return best, nil
}

// BoardByID returns the board with the given id.
func BoardByID(id string) (Board, bool) {
	for _, board := range Supported {
		if board.ID == id {
			return board, true
		}
	}
	return Board{}, false
}

// BoardIDs returns the id of every supported board.
func BoardIDs() []string {
	ids := make([]string, 0, len(Supported))
	for _, board := range Supported {
		ids = append(ids, board.ID)
	}
	return ids
}

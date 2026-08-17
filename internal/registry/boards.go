// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import "fmt"

// Board is a board the flasher can target. The name matches the one each release
// carries in the index.
type Board string

const (
	UnoQ     Board = "UNO Q"
	VentunoQ Board = "VENTUNO Q"
)

// Os is the distribution family an image belongs to, and picks the index to look
// in. It is not the manifest's own os field, which spells out the exact release
// ("Debian GNU/Linux 13 (trixie)").
type Os string

const (
	Debian Os = "debian"
	Ubuntu Os = "ubuntu"
)

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
	// DefaultOs is the index to look in when none is asked for. What is actually
	// published for a board is not listed here: every release carries its board,
	// so it is discoverable by reading the indexes.
	DefaultOs Os
	Variants  []Variant
	// PreserveUser tells whether a flash can keep the user partition. It works by
	// reading the board's partition table before writing, which is also what
	// sizing the root partition needs, so a board without it is flashed with the
	// layout the image ships with.
	PreserveUser bool
}

// Supported is every board the flasher knows about.
var Supported = []BoardDef{
	{
		Board:        UnoQ,
		Alias:        "unoq",
		DefaultOs:    Debian,
		PreserveUser: true,
		Variants: []Variant{
			{Label: "UNO Q (2GB RAM, 16GB storage)", Capacity: 16_000_000_000},
			{Label: "UNO Q (4GB RAM, 32GB storage)", Capacity: 32_000_000_000, DefaultRootSize: 20 * 1024 * 1024 * 1024},
		},
	},
	{
		Board:     VentunoQ,
		Alias:     "ventunoq",
		DefaultOs: Ubuntu,
	},
}

// Index returns the index holding this board's images: the one asked for, or the
// board's default.
func (d BoardDef) Index(requested Os) Os {
	if requested == "" {
		return d.DefaultOs
	}
	return requested
}

// DefForAlias returns the definition of the board named on the command line.
func DefForAlias(alias string) (BoardDef, bool) {
	for _, def := range Supported {
		if def.Alias == alias {
			return def, true
		}
	}
	return BoardDef{}, false
}

// Aliases returns the command line names of every supported board.
func Aliases() []string {
	aliases := make([]string, 0, len(Supported))
	for _, def := range Supported {
		aliases = append(aliases, def.Alias)
	}
	return aliases
}

// DefFor returns the definition of a board.
func DefFor(board Board) (BoardDef, error) {
	for _, def := range Supported {
		if def.Board == board {
			return def, nil
		}
	}
	return BoardDef{}, fmt.Errorf("unknown board %q", board)
}

// VariantFor returns the smallest variant that can hold the reported capacity.
func VariantFor(board Board, reported uint64) (Variant, error) {
	def, err := DefFor(board)
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

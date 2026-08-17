// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefForAlias(t *testing.T) {
	def, ok := DefForAlias("unoq")
	require.True(t, ok)
	assert.Equal(t, UnoQ, def.Board)

	def, ok = DefForAlias("ventunoq")
	require.True(t, ok)
	assert.Equal(t, VentunoQ, def.Board)

	// A path is not a board, which is what keeps a file named after one from
	// shadowing it.
	_, ok = DefForAlias("./image.tar.zst")
	assert.False(t, ok)
}

func TestIndex(t *testing.T) {
	unoQ, err := DefFor(UnoQ)
	require.NoError(t, err)
	assert.Equal(t, Debian, unoQ.Index(""), "falls back to the board's default")
	assert.Equal(t, Ubuntu, unoQ.Index(Ubuntu), "an explicit OS wins over the default")

	ventunoQ, err := DefFor(VentunoQ)
	require.NoError(t, err)
	assert.Equal(t, Ubuntu, ventunoQ.Index(""))

	_, err = DefFor("SOMETHING ELSE")
	require.ErrorContains(t, err, "unknown board")
}

// A board reports slightly less than its advertised size, so matching has to be
// by range. The rootfs in the image is ~7.3 GiB, hence these are plausible
// values rather than the round marketing numbers.
func TestVariantFor(t *testing.T) {
	v, err := VariantFor(UnoQ, 15_634_268_160)
	require.NoError(t, err)
	assert.Equal(t, uint64(16_000_000_000), v.Capacity)
	assert.Zero(t, v.DefaultRootSize, "the 16 GB variant keeps the size the image ships with")

	v, err = VariantFor(UnoQ, 31_268_536_320)
	require.NoError(t, err)
	assert.Equal(t, uint64(32_000_000_000), v.Capacity)
	assert.Equal(t, uint64(20*1024*1024*1024), v.DefaultRootSize)

	_, err = VariantFor(UnoQ, 64_000_000_000)
	require.ErrorContains(t, err, "no known UNO Q variant can hold")

	_, err = VariantFor(VentunoQ, 15_634_268_160)
	require.ErrorContains(t, err, "no known VENTUNO Q variant can hold")
}

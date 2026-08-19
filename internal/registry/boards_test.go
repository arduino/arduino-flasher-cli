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

func TestBoardByID(t *testing.T) {
	board, ok := BoardByID("unoq")
	require.True(t, ok)
	assert.Equal(t, UnoQ, board)

	board, ok = BoardByID("ventunoq")
	require.True(t, ok)
	assert.Equal(t, VentunoQ, board)

	// A path is not a board, which is what keeps a file named after one from
	// shadowing it.
	_, ok = BoardByID("./image.tar.zst")
	assert.False(t, ok)
}

func TestResolveOs(t *testing.T) {
	assert.Equal(t, Debian, UnoQ.ResolveOs(""), "falls back to the default of the board")
	assert.Equal(t, Ubuntu, UnoQ.ResolveOs(Ubuntu), "an explicit OS wins over the default")
	assert.Equal(t, Ubuntu, VentunoQ.ResolveOs(""))
}

// A board reports slightly less than its advertised size, so matching has to be
// by range. The rootfs in the image is ~7.3 GiB, hence these are plausible
// values rather than the round marketing numbers.
func TestVariantByCapacity(t *testing.T) {
	v, err := UnoQ.VariantByCapacity(15_634_268_160)
	require.NoError(t, err)
	assert.Equal(t, uint64(16_000_000_000), v.Capacity)
	assert.Zero(t, v.DefaultRootSize, "the 16 GB variant keeps the size the image ships with")

	v, err = UnoQ.VariantByCapacity(31_268_536_320)
	require.NoError(t, err)
	assert.Equal(t, uint64(32_000_000_000), v.Capacity)
	assert.Equal(t, uint64(20*1024*1024*1024), v.DefaultRootSize)

	_, err = UnoQ.VariantByCapacity(64_000_000_000)
	require.ErrorContains(t, err, "no known UNO Q variant can hold")

	_, err = VentunoQ.VariantByCapacity(15_634_268_160)
	require.ErrorContains(t, err, "no known VENTUNO Q variant can hold")
}

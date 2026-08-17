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

func TestSplitSelector(t *testing.T) {
	tests := []struct {
		selector    string
		wantBoard   Board
		wantOs      Os
		wantVersion string
	}{
		{"unoq", UnoQ, "", ""},
		{"unoq-debian", UnoQ, Debian, ""},
		{"unoq-debian-20260611-999", UnoQ, Debian, "20260611-999"},
		{"unoq-20260611-999", UnoQ, "", "20260611-999"},
		{"ventunoq", VentunoQ, "", ""},
		// Not published for the VENTUNO Q, so ubuntu is not taken as an OS.
		{"ventunoq-ubuntu", VentunoQ, "", "ubuntu"},
		// No board alias matches, so the whole thing is a version.
		{"20260611-999", "", "", "20260611-999"},
		{"latest", "", "", "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			board, os, version := SplitSelector(tt.selector)
			assert.Equal(t, tt.wantBoard, board)
			assert.Equal(t, tt.wantOs, os)
			assert.Equal(t, tt.wantVersion, version)
		})
	}
}

func TestIndexFor(t *testing.T) {
	index, err := IndexFor(UnoQ, "")
	require.NoError(t, err)
	assert.Equal(t, ImageIndex{Os: Debian, Path: "debian-im/Stable"}, index)

	index, err = IndexFor(UnoQ, Debian)
	require.NoError(t, err)
	assert.Equal(t, Debian, index.Os)

	_, err = IndexFor(UnoQ, Ubuntu)
	require.ErrorContains(t, err, "no ubuntu images are published for the UNO Q")

	_, err = IndexFor(VentunoQ, "")
	require.ErrorContains(t, err, "no images are published for the VENTUNO Q yet")

	_, err = IndexFor("SOMETHING ELSE", "")
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

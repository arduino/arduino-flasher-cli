// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		arg  string
		want registry.ImageRef
	}{
		{"unoq", registry.ImageRef{Version: latest, Board: registry.UnoQ}},
		{"unoq-debian", registry.ImageRef{Version: latest, Board: registry.UnoQ, Os: registry.Debian}},
		{"unoq-debian-20260611-999", registry.ImageRef{Version: "20260611-999", Board: registry.UnoQ, Os: registry.Debian}},
		{"unoq-20260611-999", registry.ImageRef{Version: "20260611-999", Board: registry.UnoQ}},
		// A bare version is UNO Q Debian for now.
		{"20260611-999", registry.ImageRef{Version: "20260611-999", Board: registry.UnoQ, Os: registry.Debian}},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got, err := ParseRef(tt.arg)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A board with no published images has to fail at parse time, before the flash
// is confirmed.
func TestParseRefRejectsBoardWithoutImages(t *testing.T) {
	_, err := ParseRef("ventunoq")
	require.ErrorContains(t, err, "no images are published for the VENTUNO Q yet")
}

// latest cannot pick an index while nothing identifies the board.
func TestParseRefRejectsLatest(t *testing.T) {
	_, err := ParseRef(latest)
	require.ErrorContains(t, err, "does not say which board to use")
	require.ErrorContains(t, err, "unoq", "the error should name the boards that can be used")
}

func TestParse(t *testing.T) {
	archive := paths.New(t.TempDir(), "image.tar.zst")
	require.NoError(t, archive.WriteFile([]byte("not really an archive")))

	path, release, err := Parse(archive.String())
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.Equal(t, archive.String(), path.String())
	assert.Empty(t, release)

	path, release, err = Parse("unoq-debian")
	require.NoError(t, err)
	assert.Nil(t, path)
	assert.Equal(t, registry.ImageRef{Version: latest, Board: registry.UnoQ, Os: registry.Debian}, release)
}

// A file named after a selector must not shadow it.
func TestParseSelectorsWinOverFilesystem(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{latest, "unoq", "unoq-debian"} {
		require.NoError(t, paths.New(dir, name).WriteFile([]byte("decoy")))
	}
	t.Chdir(dir)

	for _, name := range []string{"unoq", "unoq-debian"} {
		path, _, err := Parse(name)
		require.NoError(t, err)
		assert.Nil(t, path, "%q was shadowed by the file of the same name", name)
	}

	// latest is rejected rather than taken for the file of the same name.
	path, _, err := Parse(latest)
	require.ErrorContains(t, err, "does not say which board to use")
	assert.Nil(t, path)
}

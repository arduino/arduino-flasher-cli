// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGptTable(t *testing.T) {
	gptTable, err := ParseGptTable(paths.New("testdata/gpt_main0.bin"))
	require.NoError(t, err)

	assert.Equal(t, PartitionEntry{
		PosFistLBA: 9632,
		FirstLBA:   2033984,
		PosLastLBA: 9640,
		LastLBA:    22954551,
	}, gptTable.RootPartition)

	assert.Equal(t, PartitionEntry{
		PosFistLBA: 9760,
		FirstLBA:   22954552,
		PosLastLBA: 9768,
		LastLBA:    22954551,
	}, gptTable.UserPartition)
}

func TestParseGptTableResizeRoot(t *testing.T) {
	gptTable, err := ParseGptTable(paths.New("testdata/gpt_main0.bin"))
	require.NoError(t, err)

	newGptFile := paths.New(t.TempDir()).Join("gpt_main0.bin")

	_, err = gptTable.ResizeRoot(newGptFile, 3*GiB)
	require.NoError(t, err)

	gptTable, err = ParseGptTable(newGptFile)
	require.NoError(t, err)

	assert.Equal(t, PartitionEntry{
		PosFistLBA: 9632,
		FirstLBA:   2033984,
		PosLastLBA: 9640,
		LastLBA:    (3*GiB)/512 + 2033984 - 1,
	}, gptTable.RootPartition)

	assert.Equal(t, PartitionEntry{
		PosFistLBA: 9760,
		FirstLBA:   (3*GiB)/512 + 2033984,
		PosLastLBA: 9768,
		LastLBA:    (3*GiB)/512 + 2033984 - 1,
	}, gptTable.UserPartition)
}

func TestMoveUserdata(t *testing.T) {
	tmpDir := paths.New(t.TempDir())
	rawProgramFile := tmpDir.Join("rawprogram0.xml")
	err := paths.New("testdata/rawprogram0.xml").CopyTo(rawProgramFile)
	require.NoError(t, err)

	resizedFileName, _, err := MoveUserdata(rawProgramFile, 3*GiB)
	require.NoError(t, err)

	resizedFile := tmpDir.Join(resizedFileName)
	require.FileExists(t, resizedFile.String())
	got, err := resizedFile.ReadFile()
	require.NoError(t, err)

	want, err := paths.New("testdata/rawprogram0_resized.xml").ReadFile()
	require.NoError(t, err)

	require.Equal(t, string(want), string(got))
}

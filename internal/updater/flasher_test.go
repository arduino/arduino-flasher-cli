// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendBoardSerial(t *testing.T) {
	base := []string{"qdl", "--storage", "emmc"}

	t.Run("Empty serial leaves args untouched", func(t *testing.T) {
		args, err := appendBoardSerial(base, "")
		require.NoError(t, err)
		require.Equal(t, base, args)
	})

	t.Run("Decimal serial is appended as hex", func(t *testing.T) {
		args, err := appendBoardSerial(base, "305419896")
		require.NoError(t, err)
		require.Equal(t, append(slices.Clone(base), "--serial", "12345678"), args)
	})

	t.Run("Invalid serial returns an error", func(t *testing.T) {
		args, err := appendBoardSerial(base, "not-a-serial")
		require.Error(t, err)
		require.Nil(t, args)
	})
}

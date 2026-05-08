// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseQdlLogLine(t *testing.T) {
	tests := []struct {
		line string
		want QDLLogLine
	}{
		{
			line: "Waiting for EDL device...",
			want: QDLLogLine{
				Op:  Waiting,
				Log: "Waiting for EDL device...",
			},
		},
		{
			line: "waiting for programmer...",
			want: QDLLogLine{
				Op:  Waiting,
				Log: "waiting for programmer...",
			},
		},
		{
			line: `flashed "xbl_a" successfully`,
			want: QDLLogLine{
				Op:  Flashed,
				Log: `flashed "xbl_a" successfully`,
			},
		},
		{
			line: `flashed "rootfs" successfully at 65058kB/s`,
			want: QDLLogLine{
				Op:  Flashed,
				Log: `flashed "rootfs" successfully at 65058kB/s`,
			},
		},
		{

			line: "13 patches applied",
			want: QDLLogLine{
				Op:  Unknown,
				Log: "13 patches applied",
			},
		},
		{
			line: "partition 0 is now bootable",
			want: QDLLogLine{
				Op:  Unknown,
				Log: "partition 0 is now bootable",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := parseQdlLogLine(tt.line)
			require.Equal(t, tt.want, result)
		})
	}
}

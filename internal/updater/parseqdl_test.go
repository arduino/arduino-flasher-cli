// This file is part of arduino-flasher-cli.
//
// Copyright 2025 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-flasher-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to
// modify or otherwise use the software for commercial activities involving the
// Arduino software without disclosing the source code of your own applications.
// To purchase a commercial license, send an email to license@arduino.cc.

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

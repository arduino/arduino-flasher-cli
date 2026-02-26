// This file is part of arduino-flasher-cli.
//
// Copyright (C) Arduino s.r.l. and/or its affiliated companies
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

package serial

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSerialFromNum(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Serial
		wantErr bool
	}{
		{
			name:    "Valid decimal serial",
			input:   "123456789",
			want:    Serial{num: 123456789},
			wantErr: false,
		},
		{
			name:    "Invalid serial with letters",
			input:   "12345ABC",
			want:    Serial{},
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			want:    Serial{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := FromNum(tt.input)
			if tt.wantErr {
				require.Error(t, err)

			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, s)
			}

		})
	}
}

func TestSerialFromHex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Serial
		wantErr bool
	}{
		{
			name:    "Valid hexadecimal serial",
			input:   "1A2B3C4D5E6F7081",
			want:    Serial{num: 0x1A2B3C4D5E6F7081},
			wantErr: false,
		},
		{
			name:    "Invalid serial with letters",
			input:   "12345GHI",
			want:    Serial{},
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			want:    Serial{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := FromHex(tt.input)
			if tt.wantErr {
				require.Error(t, err)

			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, s)
			}

		})
	}
}

func TestSerialHex(t *testing.T) {
	tests := []struct {
		name  string
		input Serial
		want  string
	}{
		{
			name:  "Convert to hexadecimal",
			input: Serial{num: 123456789},
			want:  "075BCD15",
		},
		{
			name:  "Convert to hexadecimal (no padding)",
			input: Serial{num: 987654321},
			want:  "3ADE68B1",
		},
		{
			name:  "Convert zero",
			input: Serial{num: 0},
			want:  "00000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Hex()
			require.Equal(t, tt.want, got)
		})
	}
}

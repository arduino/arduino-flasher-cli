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

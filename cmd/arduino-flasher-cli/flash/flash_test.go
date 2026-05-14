// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package flash

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAskFlashQuestions(t *testing.T) {
	tests := []struct {
		name                    string
		input                   string
		preserveUserFlagChanged bool
		preserveUser            bool
		wantProceed             bool
		wantPreserveUser        bool
	}{
		{
			name:         "user declines flash",
			input:        "no\n",
			wantProceed:  false,
			wantPreserveUser: false,
		},
		{
			name:             "user confirms flash, declines preserve",
			input:            "yes\nno\n",
			wantProceed:      true,
			wantPreserveUser: false,
		},
		{
			name:             "user confirms flash, confirms preserve",
			input:            "yes\nyes\n",
			wantProceed:      true,
			wantPreserveUser: true,
		},
		{
			name:             "user uses 'y' shorthand for both",
			input:            "y\ny\n",
			wantProceed:      true,
			wantPreserveUser: true,
		},
		{
			name:             "user uses 'y' to confirm, 'n' to decline preserve",
			input:            "y\nn\n",
			wantProceed:      true,
			wantPreserveUser: false,
		},
		{
			name:                    "--preserve-user=true was explicitly set, skip preserve question",
			input:                   "yes\n",
			preserveUserFlagChanged: true,
			preserveUser:            true,
			wantProceed:             true,
			wantPreserveUser:        true,
		},
		{
			name:                    "--preserve-user=false was explicitly set, skip preserve question",
			input:                   "yes\n",
			preserveUserFlagChanged: true,
			preserveUser:            false,
			wantProceed:             true,
			wantPreserveUser:        false,
		},
		{
			name:             "case-insensitive: YES and YES",
			input:            "YES\nYES\n",
			wantProceed:      true,
			wantPreserveUser: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			proceed, preserveUser, err := askFlashQuestions(r, "latest", tt.preserveUserFlagChanged, tt.preserveUser)
			require.NoError(t, err)
			require.Equal(t, tt.wantProceed, proceed)
			require.Equal(t, tt.wantPreserveUser, preserveUser)
		})
	}
}

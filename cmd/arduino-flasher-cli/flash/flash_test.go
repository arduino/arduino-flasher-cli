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

func TestAskProceedFlash(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantProceed bool
	}{
		{name: "user declines flash", input: "no\n", wantProceed: false},
		{name: "user confirms flash with yes", input: "yes\n", wantProceed: true},
		{name: "user confirms flash with y", input: "y\n", wantProceed: true},
		{name: "user declines flash with n", input: "n\n", wantProceed: false},
		{name: "case-insensitive YES", input: "YES\n", wantProceed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			proceed, err := askProceedFlash(r, "latest")
			require.NoError(t, err)
			require.Equal(t, tt.wantProceed, proceed)
		})
	}
}

func TestAskPreservePartition(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantPreserveUser bool
	}{
		{name: "user declines preserve", input: "no\n", wantPreserveUser: false},
		{name: "user confirms preserve", input: "yes\n", wantPreserveUser: true},
		{name: "user confirms preserve with y", input: "y\n", wantPreserveUser: true},
		{name: "user declines preserve with n", input: "n\n", wantPreserveUser: false},
		{name: "case-insensitive YES", input: "YES\n", wantPreserveUser: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			preserveUser, err := askPreservePartition(r)
			require.NoError(t, err)
			require.Equal(t, tt.wantPreserveUser, preserveUser)
		})
	}
}

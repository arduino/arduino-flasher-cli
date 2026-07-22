// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetManifestBoardType(t *testing.T) {
	c := NewClient()
	_, err := c.GetInfoManifest(context.Background(), VentunoQ, Ubuntu)
	require.Error(t, err)

	manifest, err := c.GetInfoManifest(context.Background(), UnoQ, Debian)
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Releases)
}

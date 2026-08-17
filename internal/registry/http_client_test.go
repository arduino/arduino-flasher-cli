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

func TestGetInfoManifest(t *testing.T) {
	index, err := IndexFor(UnoQ, "")
	require.NoError(t, err)

	manifest, err := NewClient().GetInfoManifest(context.Background(), index)
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Releases)
}

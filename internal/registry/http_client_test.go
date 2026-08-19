// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInfoManifest(t *testing.T) {
	manifest, err := NewClient().GetInfoManifest(context.Background(), "debian")
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Releases)
}

// The Debian index carries both boards, and its latest release is not
// necessarily the latest for the board that was asked for.
const twoBoardManifest = `{
  "latest": {"version": "300", "board": "VENTUNO Q", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/v300.tar.zst"},
  "releases": [
    {"version": "100", "board": "UNO Q",     "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u100.tar.zst"},
    {"version": "200", "board": "UNO Q",     "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u200.tar.zst"},
    {"version": "300", "board": "VENTUNO Q", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/v300.tar.zst"}
  ]
}`

// serveManifest points the registry at a stub index for the duration of a test.
func serveManifest(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	stub, err := url.Parse(srv.URL)
	require.NoError(t, err)
	original := baseURL
	baseURL = stub
	t.Cleanup(func() { baseURL = original })
}

func TestGetReleaseByVersion(t *testing.T) {
	serveManifest(t, twoBoardManifest)
	ctx := context.Background()
	client := NewClient()

	rel, err := client.GetReleaseByVersion(ctx, "", "debian", VentunoQ.ID)
	require.NoError(t, err)
	assert.Equal(t, "300", rel.Version, "the index's latest is already this board's")

	rel, err = client.GetReleaseByVersion(ctx, "", "debian", UnoQ.ID)
	require.NoError(t, err)
	assert.Equal(t, "200", rel.Version, "falls back to the most recent release of that board")

	rel, err = client.GetReleaseByVersion(ctx, "", "debian", "")
	require.NoError(t, err)
	assert.Equal(t, "300", rel.Version, "no board means the index's latest")

	rel, err = client.GetReleaseByVersion(ctx, "100", "debian", UnoQ.ID)
	require.NoError(t, err)
	assert.Equal(t, "100", rel.Version)

	_, err = client.GetReleaseByVersion(ctx, "100", "debian", VentunoQ.ID)
	require.ErrorContains(t, err, "could not find debian image 100 for the VENTUNO Q")

	_, err = client.GetReleaseByVersion(ctx, "", "debian", "MISSING BOARD")
	require.ErrorContains(t, err, "no debian image is published for the MISSING BOARD")
}

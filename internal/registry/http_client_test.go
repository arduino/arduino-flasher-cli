// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseManifest reads an index the way [Client.load] does, so the wire format
// and what it is turned into are both covered without a server in the way.
func parseManifest(t *testing.T, body string) Releases {
	t.Helper()
	var idx index
	require.NoError(t, json.Unmarshal([]byte(body), &idx))
	return idx.parse()
}

// An index carries both boards, and its most recent release is not necessarily
// the most recent one of the board that was asked for.
const twoBoardManifest = `{
  "latest": {"version": "300", "board": "VENTUNO Q", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/v300.tar.zst"},
  "releases": [
    {"version": "100", "board": "UNO Q",     "os": "Debian GNU/Linux 13 (trixie)", "kernel": "linux-image-6.16.0", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u100.tar.zst"},
    {"version": "200", "board": "UNO Q",     "os": "Debian GNU/Linux 13 (trixie)", "kernel": "linux-image-6.16.7", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u200.tar.zst"},
    {"version": "300", "board": "VENTUNO Q", "os": "Debian GNU/Linux 13 (trixie)", "kernel": "linux-image-7.0.0",  "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/v300.tar.zst"},
    {"version": "400", "board": "SOME FUTURE BOARD", "os": "Debian GNU/Linux 13 (trixie)", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/f400.tar.zst"}
  ]
}`

// One index can hold releases of several distributions, whichever ones the
// releases name. Only a release naming none has no distribution.
const mixedOsManifest = `{
  "releases": [
    {"version": "100", "board": "UNO Q",     "os": "Debian GNU/Linux 13 (trixie)", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u100.tar.zst"},
    {"version": "200", "board": "UNO Q",                                           "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u200.tar.zst"},
    {"version": "300", "board": "UNO Q",     "os": "Ubuntu 24.04.1 LTS",           "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u300.tar.zst"},
    {"version": "400", "board": "VENTUNO Q", "os": "ubuntu 26.04",                 "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/v400.tar.zst"},
    {"version": "500", "board": "UNO Q",     "os": "Fedora 42",                    "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u500.tar.zst"}
  ]
}`

const Debian = "debian"
const Ubuntu = "ubuntu"

func TestParse(t *testing.T) {
	releases := parseManifest(t, twoBoardManifest)

	require.Len(t, releases, 3, "a board the flasher cannot flash is skipped")
	assert.Equal(t, "300", releases[0].Version, "newest first")
	assert.Equal(t, VentunoQ.ID, releases[0].Board, "the label is resolved to a board")
	assert.Equal(t, Debian, releases[0].OS, "the distribution the release names")
	assert.Equal(t, "v300.tar.zst", releases[0].FileName, "the archive is named after the URL")
}

func TestParseSeveralDistributions(t *testing.T) {
	releases := parseManifest(t, mixedOsManifest)

	require.Len(t, releases, 5, "every release is read")
	assert.Equal(t, []string{"fedora", Ubuntu, Debian}, releases.OSes(),
		"one index, several distributions, none of them a set this flasher carries")

	byVersion := func(v string) Release {
		rel, ok := releases.byVersion(v)
		require.True(t, ok, "release %s should have been read", v)
		return rel
	}
	assert.Equal(t, Debian, byVersion("100").OS, "the distribution the release names")
	assert.Equal(t, Ubuntu, byVersion("300").OS, "the same index, another distribution")
	assert.Equal(t, Ubuntu, byVersion("400").OS, "matched however it is spelled")

	assert.Empty(t, byVersion("200").OS, "a release naming no distribution has none")
	assert.Equal(t, "fedora", byVersion("500").OS, "one naming any other is taken at its word")

	// The most recent release of a board is the most recent one of the
	// distribution asked for, which an index holding several makes a real
	// distinction rather than a theoretical one.
	latest, ok := releases.Filter(Debian, UnoQ.ID).newest()
	require.True(t, ok)
	assert.Equal(t, "100", latest.Version, "the newest Debian one, not the newest one")
	latest, ok = releases.Filter(Ubuntu, UnoQ.ID).newest()
	require.True(t, ok)
	assert.Equal(t, "300", latest.Version)
}

// A release the publisher got wrong is no reason to lose the ones it got right.
func TestParseSkipsUnusableReleases(t *testing.T) {
	releases := parseManifest(t, `{"releases": [
	  {"version": "100", "board": "UNO Q", "os": "Debian GNU/Linux 13 (trixie)", "sha256": "abcd", "url": "https://x/u100.tar.zst"},
	  {"version": "200", "board": "UNO Q", "os": "Debian GNU/Linux 13 (trixie)", "sha256": "not hex at all 00000000000000000000000000000000000000000000000000", "url": "https://x/u200.tar.zst"},
	  {"version": "300", "board": "UNO Q", "os": "Debian GNU/Linux 13 (trixie)", "sha256": "0000000000000000000000000000000000000000000000000000000000000000", "url": "https://x/u300.tar.zst"}
	]}`)

	require.Len(t, releases, 1, "a checksum that cannot be right takes only its own release")
	assert.Equal(t, "300", releases[0].Version)
}

func TestResolve(t *testing.T) {
	releases := parseManifest(t, twoBoardManifest)

	rel, err := releases.Resolve(Debian, VentunoQ.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "300", rel.Version, "the index's latest is already this board's")

	rel, err = releases.Resolve(Debian, UnoQ.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "200", rel.Version, "falls back to the most recent release of that board")

	rel, err = releases.Resolve(Debian, "", "")
	require.NoError(t, err)
	assert.Equal(t, "300", rel.Version, "no board means the index's latest")

	rel, err = releases.Resolve(Debian, UnoQ.ID, "100")
	require.NoError(t, err)
	assert.Equal(t, "100", rel.Version)

	_, err = releases.Resolve(Debian, VentunoQ.ID, "100")
	require.ErrorContains(t, err, "could not find debian image 100 for the VENTUNO Q")

	_, err = releases.Resolve(Debian, "MISSING BOARD", "")
	require.ErrorContains(t, err, "no debian image is published for the MISSING BOARD")

	_, err = releases.Resolve(Ubuntu, UnoQ.ID, "")
	require.ErrorContains(t, err, "no ubuntu image is published for the UNO Q", "the OS filters too")
}

// A build number is not padded, so it cannot be compared as text as it comes.
func TestCompareVersions(t *testing.T) {
	assert.Negative(t, compareVersions("20260407-99", "20260407-523"), "99 is an earlier build than 523")
	assert.Positive(t, compareVersions("20260407-523", "20260407-99"))
	assert.Zero(t, compareVersions("20260407-523", "20260407-523"))
	assert.Negative(t, compareVersions("20260407-999", "20260408-1"), "a later date wins whatever the build")

	// Nothing is assumed about how many parts there are or what they mean.
	assert.Negative(t, compareVersions("20260407", "20260407-1"), "fewer parts is less specific")
	assert.Negative(t, compareVersions("1-2-9", "1-2-10"), "any number of parts")
	assert.Negative(t, compareVersions("1.9.0", "1.10.0"), "and parts that are not numbers at all")

	releases := Releases{{Version: "20260407-99"}, {Version: "20260407-523"}}
	releases.sortNewestFirst()
	assert.Equal(t, "20260407-523", releases[0].Version, "newest first")

	latest, ok := releases.newest()
	require.True(t, ok)
	assert.Equal(t, "20260407-523", latest.Version)
}

func TestFilter(t *testing.T) {
	releases := Releases{
		{Version: "300", Board: VentunoQ.ID, OS: Ubuntu},
		{Version: "200", Board: UnoQ.ID, OS: Debian},
		{Version: "100", Board: UnoQ.ID, OS: Ubuntu},
	}

	assert.Len(t, releases.Filter("", ""), 3, "a zero OS and board match any")
	assert.Len(t, releases.Filter(Ubuntu, ""), 2)
	assert.Len(t, releases.Filter("", UnoQ.ID), 2)
	assert.Len(t, releases.Filter(Ubuntu, UnoQ.ID), 1)
	assert.Empty(t, releases.Filter(Debian, VentunoQ.ID))

	assert.Equal(t, []string{Ubuntu, Debian}, releases.OSes())

	latest, ok := releases.Filter("", UnoQ.ID).newest()
	require.True(t, ok)
	assert.Equal(t, "200", latest.Version, "the latest of the board, not of the snapshot")

	_, ok = Releases{}.newest()
	assert.False(t, ok, "an empty snapshot has no latest")

	unordered := Releases{{Version: "100"}, {Version: "300"}, {Version: "200"}}
	latest, ok = unordered.newest()
	require.True(t, ok)
	assert.Equal(t, "300", latest.Version, "the most recent, not the first")
}

// serveIndexes points the registry at one stub index per body for the duration
// of a test, so a snapshot spanning several of them can be exercised. An empty
// body stands for an index that is not published yet.
func serveIndexes(t *testing.T, bodies ...string) {
	t.Helper()
	// Whoever is testing an unpublished index has this set, and a snapshot of
	// theirs as well as the stubs is not what any of these tests mean.
	t.Setenv(additionalURLsEnv, "")
	original := indexes
	indexes = nil
	for _, body := range bodies {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)

		stub, err := url.Parse(srv.URL)
		require.NoError(t, err)
		indexes = append(indexes, stub)
	}
	t.Cleanup(func() { indexes = original })
}

func TestFetchSpansIndexes(t *testing.T) {
	serveIndexes(t, twoBoardManifest, mixedOsManifest)

	releases, err := NewClient().Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 8, "a snapshot spans every index")
	assert.True(t, slices.IsSortedFunc(releases, func(a, b Release) int {
		return compareVersions(b.Version, a.Version)
	}), "a snapshot is newest first across indexes, not per index")
}

// An index that is not published yet is an ordinary state of affairs, so it is
// passed over without a word.
func TestFetchPassesOverUnpublishedIndex(t *testing.T) {
	serveIndexes(t, twoBoardManifest, "")

	releases, err := NewClient().Fetch(context.Background())
	require.NoError(t, err, "not being published yet is not a failure")
	require.Len(t, releases, 3, "the published one is still read")

	rel, err := releases.Resolve(Debian, UnoQ.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "200", rel.Version)
}

// An index that is there but cannot be read is a different matter, and is
// reported without costing the ones that could.
func TestFetchReportsUnreadableIndex(t *testing.T) {
	serveIndexes(t, twoBoardManifest, "<html>not an index at all</html>")

	releases, err := NewClient().Fetch(context.Background())
	require.ErrorContains(t, err, "could not read the image index at", "the one that failed is named")
	require.Len(t, releases, 3, "the ones that could be read are still returned")

	rel, resolveErr := releases.Resolve(Debian, UnoQ.ID, "")
	require.NoError(t, resolveErr, "one bad index does not hide the rest")
	assert.Equal(t, "200", rel.Version)
}

func TestFetchLiveIndexes(t *testing.T) {
	// Against the real indexes, of which not all are published: whatever could
	// be read is still returned, and the rest is only reported.
	releases, err := NewClient().Fetch(context.Background())
	require.NotEmpty(t, releases, "the published indexes should still be read: %v", err)
	for _, r := range releases {
		assert.NotEmpty(t, r.FileName, "the archive of %s should be named", r.Version)
		assert.NotEmpty(t, r.Board, "the board of %s should be resolved", r.Version)
		assert.NotEmpty(t, r.OS, "the distribution of %s should be resolved", r.Version)
	}
}

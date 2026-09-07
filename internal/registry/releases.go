// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"fmt"
	"slices"
	"strings"
)

// Release is one published image, with what an index spells its own way already
// resolved.
type Release struct {
	Version string
	Url     string
	Sha256  string
	// Board the release is built for: an index holds several.
	Board string
	// OS is the distribution the release is built for, as the release named it:
	// an index may hold several. Empty only when the index named none.
	OS string
	// FileName is the name the archive is downloaded under.
	FileName string
	// Latest is whether this is the most recent release of its board and
	// distribution. Set once, when the snapshot is built.
	Latest bool
}

// Releases is a snapshot of every index, newest release first. Nothing is
// cached, so a snapshot is a round trip to every index: fetch it once with
// [Client.Fetch] and filter it as many times as needed.
type Releases []Release

// Filter keeps the releases of one distribution and board. An empty os or board
// matches any, and a distribution matches regardless of case: a release carries
// the index's spelling, the one asked for was typed by someone.
func (rs Releases) Filter(os string, board string) Releases {
	var out Releases
	for _, r := range rs {
		if (os == "" || strings.EqualFold(r.OS, os)) && (board == "" || r.Board == board) {
			out = append(out, r)
		}
	}
	return out
}

// Resolve is the single release a command asked for: the one carrying version,
// or the most recent one when no version was given.
func (rs Releases) Resolve(os string, board string, version string) (Release, error) {
	// Named the way the user knows it, which for a board of ours is its label.
	name := board
	if b, ok := BoardByID(board); ok {
		name = b.Label
	}
	image := "image"
	if os != "" {
		image = os + " image"
	}

	matching := rs.Filter(os, board)
	if version == "" {
		rel, ok := matching.newest()
		if !ok {
			return Release{}, fmt.Errorf("no %s is published for the %s", image, name)
		}
		return rel, nil
	}
	rel, ok := matching.byVersion(version)
	if !ok {
		return Release{}, fmt.Errorf("could not find %s %s for the %s", image, version, name)
	}
	return rel, nil
}

// OSes is every distribution something was published for in rs, which is what to
// offer a choice between rather than a list this flasher carries.
func (rs Releases) OSes() []string {
	var out []string
	for _, r := range rs {
		if r.OS != "" && !slices.Contains(out, r.OS) {
			out = append(out, r.OS)
		}
	}
	return out
}

// Everything below is how the above is kept in order, and is not for use
// outside this package.

// sortNewestFirst is the order every snapshot is in.
func (rs Releases) sortNewestFirst() {
	slices.SortStableFunc(rs, func(a, b Release) int {
		return compareVersions(b.Version, a.Version)
	})
}

// compareVersions orders two versions oldest first. A version is parts separated
// by "-", of no fixed number and no assumed meaning, so each is widened to a
// fixed size before comparing as text: that is what puts build 99 before 523
// instead of after it. A part already longer than that is left alone.
func compareVersions(a, b string) int {
	const versionPartWidth = 10

	padVersion := func(v string) string {
		parts := strings.Split(v, "-")
		for i, part := range parts {
			if len(part) < versionPartWidth {
				parts[i] = strings.Repeat("0", versionPartWidth-len(part)) + part
			}
		}
		return strings.Join(parts, "-")
	}

	return strings.Compare(padVersion(a), padVersion(b))
}

// markLatest flags the most recent release of each board and distribution. It
// needs the whole snapshot, since one line can span several indexes, and it
// survives [Releases.Filter], which never discriminates on version.
func (rs Releases) markLatest() {
	type line struct {
		board string
		os    string
	}
	// Newest first, so the first release of a line is the most recent one.
	seen := make(map[line]bool, len(rs))
	for i, r := range rs {
		key := line{r.Board, r.OS}
		rs[i].Latest = !seen[key]
		seen[key] = true
	}
}

// newest is the most recent release in rs, whatever it is grouped by. Distinct
// from the [Release.Latest] field, which is per board and distribution.
func (rs Releases) newest() (Release, bool) {
	if len(rs) == 0 {
		return Release{}, false
	}
	return slices.MaxFunc(rs, func(a, b Release) int {
		return compareVersions(a.Version, b.Version)
	}), true
}

// byVersion is the release carrying an exact version.
func (rs Releases) byVersion(version string) (Release, bool) {
	for _, r := range rs {
		if r.Version == version {
			return r, true
		}
	}
	return Release{}, false
}

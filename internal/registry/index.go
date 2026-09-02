// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path"
	"strings"
)

// index is the JSON an index publishes, mirrored field for field so that what an
// index holds is readable in one place, including what the flasher ignores.
type index struct {
	// Latest is the most recent release of the index as a whole, which is not the
	// most recent one of any board or distribution in particular. Nothing uses
	// it: callers want [Releases.Latest] after a [Releases.Filter].
	Latest indexRelease `json:"latest"`
	// Releases is every release the index holds, oldest first.
	Releases []indexRelease `json:"releases"`
}

// indexRelease is one entry of an index.
type indexRelease struct {
	// Version is a build date and number, "20250807-136".
	Version string `json:"version"`
	// Url points at the image archive, named after the release.
	Url string `json:"url"`
	// Sha256 is the checksum of the archive, hex encoded.
	Sha256 string `json:"sha256"`
	// Board is the label of the board the image is built for, which is how an
	// index names a board: "UNO Q".
	Board string `json:"board,omitempty"`
	// Os spells out the exact distribution: "Debian GNU/Linux 13 (trixie)".
	Os string `json:"os,omitempty"`
	// Kernel is the kernel package the image ships. Unused, published for the
	// record.
	Kernel string `json:"kernel,omitempty"`
}

// parse turns an index into the releases the flasher works with. An entry it
// cannot use is passed over: a newly published board must not break reading the
// others, and one entry the publisher got wrong is no reason to lose the rest.
func (idx index) parse() Releases {
	var releases Releases
	for _, r := range idx.Releases {
		board, ok := boardByLabel(r.Board)
		if !ok {
			continue
		}

		os := osByDistro(r.Os)

		// The download checks the archive against it, so a checksum that cannot
		// be right makes this release unusable.
		if sha256Byte, err := hex.DecodeString(r.Sha256); err != nil || len(sha256Byte) != sha256.Size {
			continue
		}

		relURL, err := url.Parse(r.Url)
		if err != nil {
			continue
		}

		releases = append(releases, Release{
			Version:  r.Version,
			Url:      r.Url,
			Sha256:   r.Sha256,
			Board:    board.ID,
			OS:       os,
			FileName: path.Base(relURL.Path),
		})
	}
	releases.sortNewestFirst()
	return releases
}

// boardByLabel returns the board an index named, which it does by label.
func boardByLabel(label string) (Board, bool) {
	for _, board := range supported {
		if board.Label == label {
			return board, true
		}
	}
	return Board{}, false
}

// osByDistro returns the distribution a release named, which it spells out in
// full: "Debian GNU/Linux 13 (trixie)" is debian.
func osByDistro(distro string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(distro), " ")
	return strings.ToLower(name)
}

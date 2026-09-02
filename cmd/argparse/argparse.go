// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package argparse holds the argument parsing the commands have in common.
package argparse

import (
	"regexp"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// legacyVersion matches how an image is published, e.g. 20250915-173.
var legacyVersion = regexp.MustCompile(`^\d{8}-\d+$`)

// LegacyArgs keeps working the arguments the commands took before the board was
// named first: "latest" or a version, which both meant the most recent image of
// the only board there was, and an image on disk, which meant an UNO Q one. It
// returns the board, the image and the version that mean the same thing today.
// Deleting this function and its callers ends the deprecation.
func LegacyArgs(board, image, version string) (string, string, string) {
	// Nothing to translate when the board was not named.
	if board == "" {
		return board, image, version
	}
	// A board id always wins, so a file named after one does not shadow it.
	if _, ok := registry.BoardByID(board); ok {
		return board, image, version
	}
	unoQ := registry.UnoQ.ID

	switch {
	case board == "latest":
		feedback.Warning(i18n.Tr("%q is deprecated and will be removed, use: %s", board, unoQ))
		return unoQ, image, version

	case legacyVersion.MatchString(board):
		if version != "" {
			feedback.Fatal(i18n.Tr("the version was passed both as %s and as --version %s, keep only the flag",
				board, version), feedback.ErrBadArgument)
		}
		feedback.Warning(i18n.Tr("%q is deprecated and will be removed, use: %s --version %s", board, unoQ, board))
		return unoQ, image, board

	// An image where the board is expected only ever meant an UNO Q one.
	case image == "" && paths.New(board).Exist():
		feedback.Warning(i18n.Tr("flashing an image without naming the board is deprecated and will be removed, use: %s %s",
			unoQ, board))
		return unoQ, board, version

	default:
		return board, image, version
	}
}

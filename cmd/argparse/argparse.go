// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package argparse holds the argument parsing the commands have in common.
package argparse

import (
	"regexp"

	"github.com/arduino/go-paths-helper"
	"github.com/fatih/color"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// legacyVersion matches how an image is published, e.g. 20250915-173.
var legacyVersion = regexp.MustCompile(`^\d{8}-\d+$`)

// LegacyArg keeps working the argument the commands took before the boards were
// named: "latest", or the version to use, which both meant the most recent image
// of the only board there was. It returns the board and the version that mean
// the same thing today, warning that the spelling is on its way out. Deleting
// this function and its callers ends the deprecation.
func LegacyArg(arg, version string) (string, string) {
	unoQ := string(registry.UnoQ.ID)

	switch {
	case arg == "latest":
		warn(i18n.Tr("%q is deprecated and will be removed, use: %s", arg, unoQ))
		return unoQ, version

	// An image on disk is not a version, however it is named.
	case legacyVersion.MatchString(arg) && !paths.New(arg).Exist():
		if version != "" {
			feedback.Fatal(i18n.Tr("the version was passed both as %s and as --version %s, keep only the flag",
				arg, version), feedback.ErrBadArgument)
		}
		warn(i18n.Tr("%q is deprecated and will be removed, use: %s --version %s", arg, unoQ, arg))
		return unoQ, arg

	default:
		return arg, version
	}
}

func warn(message string) {
	feedback.Print(color.YellowString("WARNING: " + message))
}

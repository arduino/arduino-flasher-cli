// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

// boardToString converts a Board enum into the internal registry string
// representation. It returns an empty string for unspecified/unknown values.
func boardToString(b flasher.Board) string {
	switch b {
	case flasher.Board_BOARD_UNO_Q:
		return registry.UnoQ
	case flasher.Board_BOARD_VENTUNO_Q:
		return registry.VentunoQ
	default:
		return ""
	}
}

// osToString converts an OS enum into the internal registry string
// representation. It returns an empty string for unspecified/unknown values.
func osToString(o flasher.OS) string {
	switch o {
	case flasher.OS_OS_DEBIAN:
		return registry.Debian
	case flasher.OS_OS_UBUNTU:
		return registry.Ubuntu
	default:
		return ""
	}
}

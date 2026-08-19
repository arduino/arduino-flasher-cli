// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

// boardFromRPC converts a Board enum into the board the registry knows. An
// unspecified board yields an empty one, which means "any".
func boardFromRPC(b flasher.Board) string {
	switch b {
	case flasher.Board_BOARD_UNO_Q:
		return registry.UnoQ.ID
	case flasher.Board_BOARD_VENTUNO_Q:
		return registry.VentunoQ.ID
	default:
		return ""
	}
}

// boardToRPC converts a registry board into its Board enum.
func boardToRPC(b string) flasher.Board {
	switch b {
	case registry.UnoQ.ID:
		return flasher.Board_BOARD_UNO_Q
	case registry.VentunoQ.ID:
		return flasher.Board_BOARD_VENTUNO_Q
	default:
		return flasher.Board_BOARD_UNSPECIFIED
	}
}

// osFromRPC converts an OS enum into the distribution the registry knows. An
// unspecified OS yields an empty one, which means "the default".
func osFromRPC(o flasher.OS) string {
	switch o {
	case flasher.OS_OS_DEBIAN:
		return "debian"
	case flasher.OS_OS_UBUNTU:
		return "ubuntu"
	default:
		return ""
	}
}

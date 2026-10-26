// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/internal/updater"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

type flasherServerImpl struct {
	flasher.UnsafeFlasherServer // Force compile error for unimplemented methods
	flashBoard                  func(context.Context, string, *paths.Path, string, string, bool, uint64, updater.FlashCallback) error
}

func NewFlasherServer() flasher.FlasherServer {
	return &flasherServerImpl{flashBoard: updater.FlashBoard}
}

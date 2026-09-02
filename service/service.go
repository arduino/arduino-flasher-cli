// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"

type flasherServerImpl struct {
	flasher.UnsafeFlasherServiceServer // Force compile error for unimplemented methods
}

func NewFlasherServer() flasher.FlasherServiceServer {
	return &flasherServerImpl{}
}

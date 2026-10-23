// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package updater

import (
	"context"
	"errors"

	"github.com/arduino/go-paths-helper"
)

func StreamImage(ctx context.Context, archive, dest *paths.Path) (func(), error) {
	return nil, errors.New("streaming the image is only supported on Linux and macOS")
}

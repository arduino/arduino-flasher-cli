// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"

	"github.com/arduino/go-paths-helper"
	"github.com/shirou/gopsutil/v4/disk"

	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

func (s *flasherServerImpl) GetAvailableFreeSpace(ctx context.Context, req *flasher.GetAvailableFreeSpaceRequest) (*flasher.GetAvailableFreeSpaceResponse, error) {
	dk, err := disk.Usage(paths.New(req.Path).Parent().String())
	if err != nil {
		return nil, err
	}

	return &flasher.GetAvailableFreeSpaceResponse{
		FreeSpace: dk.Free,
	}, nil
}

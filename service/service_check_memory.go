// This file is part of arduino-flasher-cli.
//
// Copyright 2025 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-flasher-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to
// modify or otherwise use the software for commercial activities involving the
// Arduino software without disclosing the source code of your own applications.
// To purchase a commercial license, send an email to license@arduino.cc.

package service

import (
	"context"

	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
	"github.com/arduino/go-paths-helper"
	"github.com/shirou/gopsutil/v4/disk"
)

const GiB = 1024 * 1024 * 1024

func (s *flasherServerImpl) CheckMemory(ctx context.Context, req *flasher.CheckMemoryRequest) (*flasher.CheckMemoryResponse, error) {

	dk, err := disk.Usage(paths.New(req.Path).Parent().String())
	if err != nil {
		return nil, err
	}

	// TODO Return a rounded value? e.g. 1.234?
	freeGB := float32(dk.Free) / GiB

	return &flasher.CheckMemoryResponse{
		FreeGigaBytes: freeGB,
	}, nil
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater/artifacts"
)

func DetectBoardType(ctx context.Context, qdlPath, flashDir *paths.Path) (string, error) {
	detectBinPath := flashDir.Join("detect.bin")
	detectXMLPath := qdlPath.Parent().Join("detect.xml")
	if err := detectXMLPath.WriteFile(artifacts.DetectXML); err != nil {
		return "", err
	}
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--storage", "emmc", "prog_firehose_ddr.elf", detectXMLPath.String())
	if err != nil {
		return "", err
	}
	cmd.SetDir(flashDir.String())
	if err := cmd.RunWithinContext(ctx); err != nil {
		// qdl fails with exit code 1 if the programmer retrieved from the image is not compatible with the board.
		return "", fmt.Errorf("%w: the provided image might not be compatible with the attached board", err)
	}
	if !detectBinPath.Exist() {
		return "", fmt.Errorf("it was not possible to access the CDT")
	}
	detect, err := detectBinPath.ReadFile()
	if err != nil {
		return "", err
	}
	strDetect := hex.Dump(detect)
	if strings.HasPrefix(strDetect, "00000000  43 44 54 00 01 00 00 00  00 00 00 00 00 00 12 00  |CDT.............|\n00000010  06 00 03 20 01 00 01 00  00 00 00 00 00 00 00 00  |... ............|") {
		return registry.UnoQ, nil
	}
	return registry.VentunoQ, nil
}

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

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater/artifacts"
)

const latest = "latest"

func SetBoardAndVersion(ctx context.Context, version string) (string, string, error) {
	boardType := registry.UnoQ
	switch version {
	case latest:
		var err error
		feedback.Print(i18n.Tr("Detecting board type. Please connect the board in EDL mode."))
		boardType, err = DetectBoardType(ctx)
		if err != nil {
			return "", "", err
		}
	case "unoq":
		version = latest
		boardType = registry.UnoQ
	case "ventunoq":
		version = latest
		boardType = registry.VentunoQ
	}
	return version, boardType, nil
}

func DetectBoardType(ctx context.Context) (string, error) {
	qdlPath, cleanup, err := installQdl()
	if err != nil {
		return "", err
	}
	defer cleanup()
	detectBinPath := qdlPath.Parent().Join("detect.bin")
	detectXMLPath := qdlPath.Parent().Join("detect.xml")
	err = detectXMLPath.WriteFile(artifacts.DetectXML)
	if err != nil {
		return "", err
	}
	progElfPath := qdlPath.Parent().Join("prog_firehose_ddr.elf")
	err = progElfPath.WriteFile(artifacts.ProgElf)
	if err != nil {
		return "", err
	}
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--storage", "emmc", "prog_firehose_ddr.elf", detectXMLPath.String())
	if err != nil {
		return "", err
	}
	cmd.SetDir(qdlPath.Parent().String())
	if err := cmd.RunWithinContext(ctx); err != nil {
		return "", err
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

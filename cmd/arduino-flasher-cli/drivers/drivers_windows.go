// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package drivers

import (
	"embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
)

//go:embed src
var drivers embed.FS

// InstallDrivers installs the Windows driver using pnputil. This requires
// administrative privileges.
func InstallDrivers(logFile string) error {
	tmpDir, err := paths.MkTempDir("", "arduino-flasher-windriver-")
	if err != nil {
		return err
	}
	defer tmpDir.RemoveAll()

	driverCat, err := drivers.ReadFile("src/qcserlib.cat")
	if err != nil {
		return err
	}
	driverInf, err := drivers.ReadFile("src/qcserlib.inf")
	if err != nil {
		return err
	}
	catPath := tmpDir.Join("qcserlib.cat")
	err = catPath.WriteFile(driverCat)
	if err != nil {
		return err
	}
	infPath := tmpDir.Join("qcserlib.inf")
	err = infPath.WriteFile(driverInf)
	if err != nil {
		return err
	}

	slog.Info("Installing Windows driver")
	pnputilProc := exec.Command("pnputil", "/add-driver", infPath.String(), "/install")
	pnputilProc.Dir = tmpDir.String()
	out, err := pnputilProc.CombinedOutput()
	feedback.Print(string(out))

	if logFile != "" {
		logContent := string(out)
		if err != nil {
			logContent += fmt.Sprintf("\npnputil failed: %v\n", err)
		}
		if writeErr := os.WriteFile(logFile, []byte(logContent), 0o644); writeErr != nil {
			slog.Warn("Could not write driver installation log", "file", logFile, "error", writeErr)
		}
	}

	return err
}

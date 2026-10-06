// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package drivers

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
)

// See https://learn.microsoft.com/en-us/windows-hardware/drivers/devtest/pnputil-return-values
const (
	errorSuccess               = 0    // ERROR_SUCCESS
	errorNoMoreItems           = 259  // ERROR_NO_MORE_ITEMS: driver staged, but no present device matched it
	errorSuccessRebootRequired = 3010 // ERROR_SUCCESS_REBOOT_REQUIRED: success, reboot needed to finalize
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
	out, runErr := pnputilProc.CombinedOutput()
	feedback.Print(string(out))

	// Determine pnputil's actual exit code. CombinedOutput reports any non-zero
	// exit as an *exec.ExitError, but some of those codes are not failures.
	exitCode := errorSuccess
	var exitErr *exec.ExitError
	if runErr != nil {
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// pnputil could not be started at all (e.g. not found).
			writeInstallLog(logFile, string(out), runErr)
			return runErr
		}
	}

	var installErr error
	switch exitCode {
	case errorSuccess:
	case errorNoMoreItems:
		slog.Info("Windows driver staged, but no matching device is currently present; it will be installed automatically when the board is connected")
	case errorSuccessRebootRequired:
		feedback.Print("A system reboot is required to finalize the driver installation.")
		installErr = ErrRebootRequired
	default:
		installErr = fmt.Errorf("pnputil exited with code %d", exitCode)
	}

	writeInstallLog(logFile, string(out), installErr)
	return installErr
}

func writeInstallLog(logFile, out string, err error) {
	if logFile == "" {
		return
	}
	logContent := out
	if err != nil && !errors.Is(err, ErrRebootRequired) {
		logContent += fmt.Sprintf("\ndriver installation failed: %v\n", err)
	}
	if writeErr := os.WriteFile(logFile, []byte(logContent), 0o644); writeErr != nil {
		slog.Warn("Could not write driver installation log", "file", logFile, "error", writeErr)
	}
}

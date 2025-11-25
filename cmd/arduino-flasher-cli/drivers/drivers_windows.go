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

package drivers

import (
	"embed"
	"log/slog"
	"runtime"

	"github.com/arduino/go-paths-helper"
)

//go:embed src
var drivers embed.FS

// installDrivers installs the Windows driver using dpinst.exe. This requires
// administrative privileges.
func installDrivers() error {
	tmpDir, err := paths.MkTempDir("", "arduino-flasher-windriver-")
	if err != nil {
		return err
	}
	defer tmpDir.RemoveAll()

	dpinstArch := "drivers/dpinst-x86.exe"
	if runtime.GOARCH == "amd64" {
		dpinstArch = "drivers/dpinst-amd64.exe"
	}
	dpinst, err := drivers.ReadFile(dpinstArch)
	if err != nil {
		return err
	}
	driverCat, err := drivers.ReadFile("drivers/unoq.cat")
	if err != nil {
		return err
	}
	driverInf, err := drivers.ReadFile("drivers/unoq.inf")
	if err != nil {
		return err
	}
	dpinstPath := tmpDir.Join("dpinst.exe")
	err = dpinstPath.WriteFile(dpinst)
	if err != nil {
		return err
	}
	catPath := tmpDir.Join("unoq.cat")
	err = catPath.WriteFile(driverCat)
	if err != nil {
		return err
	}
	infPath := tmpDir.Join("unoq.inf")
	err = infPath.WriteFile(driverInf)
	if err != nil {
		return err
	}

	slog.Info("Installing Windows driver")
	dpinstProc, err := paths.NewProcessFromPath(nil, dpinstPath, "/SE", "/SW", "/SA")
	if err != nil {
		return err
	}
	dpinstProc.SetDir(tmpDir.String())
	return dpinstProc.Run()
}

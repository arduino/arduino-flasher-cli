package main

import (
	"embed"
	"log/slog"
	"runtime"

	"github.com/arduino/go-paths-helper"
)

//go:embed drivers
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

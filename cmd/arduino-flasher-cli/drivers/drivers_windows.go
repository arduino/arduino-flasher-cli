package drivers

import (
	"embed"
	"log/slog"
	"os/exec"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
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

	driverCat, err := drivers.ReadFile("src/unoq.cat")
	if err != nil {
		return err
	}
	driverInf, err := drivers.ReadFile("src/unoq.inf")
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
	pnputilProc := exec.Command("pnputil", "/add-driver", infPath.String(), "/install")
	pnputilProc.Dir = tmpDir.String()
	out, err := pnputilProc.CombinedOutput()
	feedback.Print(string(out))
	return err
}

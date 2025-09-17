package updater

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"embed"

	"github.com/arduino/go-paths-helper"
	"github.com/bcmi-labs/orchestrator/cmd/feedback"
)

//go:embed assets
var assets embed.FS

func FlashBoard(ctx context.Context, downloadedImagePath string, updatedVersion string) error {
	qdl, err := getQdlBytes()
	if err != nil {
		return err
	}

	flashDir := paths.New(downloadedImagePath, "arduino-unoq-debian-image-"+updatedVersion, "flash_UnoQ")
	qdlPath := flashDir.Join("qdl")
	if runtime.GOOS == "windows" {
		qdlPath = flashDir.Join("qdl.exe")
	}

	err = qdlPath.WriteFile(qdl)
	if err != nil {
		return err
	}
	err = qdlPath.Chmod(0755)
	if err != nil {
		return err
	}

	stdout, _, err := feedback.DirectStreams()
	if err != nil {
		return err
	}
	// TODO: add logic to preserve the user partition
	slog.Info("Flashing with qdl")
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--storage", "emmc", "prog_firehose_ddr.elf", "rawprogram0.xml", "patch0.xml")
	if err != nil {
		return err
	}
	// Setting the directory is needed because rawprogram0.xml contains relative file paths
	cmd.SetDir(flashDir.String())
	cmd.RedirectStderrTo(stdout)
	cmd.RedirectStdoutTo(stdout)
	if err := cmd.RunWithinContext(ctx); err != nil {
		return err
	}

	return nil
}

func getQdlBytes() ([]byte, error) {
	var filename string
	switch runtime.GOOS {
	case "linux":
		filename = "assets/qdl_Linux"
	case "darwin":
		filename = "assets/qdl_Darwin"
	case "windows":
		filename = "assets/qdl_Windows.exe"
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return assets.ReadFile(filename)
}

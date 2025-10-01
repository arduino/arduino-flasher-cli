package updater

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"runtime"
	"strings"

	"embed"

	"github.com/arduino/go-paths-helper"
	"github.com/bcmi-labs/orchestrator/cmd/feedback"
)

//go:embed assets
var assets embed.FS

func Flash(ctx context.Context, imagePath *paths.Path, version string, forceYes bool) error {
	if !imagePath.Exist() {
		updateURL := "https://downloads.arduino.cc"

		parsedURL, err := url.Parse(updateURL)
		if err != nil {
			return fmt.Errorf("invalid UPDATE_URL: %v", err)
		}

		client := NewClient(parsedURL, "debian-im/Stable")

		tempImagePath, err := DownloadAndExtract(client, version, func(target string) (bool, error) {
			feedback.Printf("Found Debian image version: %s", target)
			feedback.Printf("Do you want to download it and flash it on the board? (yes/no)")

			var yesInput string
			_, err := fmt.Scanf("%s\n", &yesInput)
			if err != nil {
				return false, err
			}
			yes := strings.ToLower(yesInput) == "yes" || strings.ToLower(yesInput) == "y"
			return yes, nil
		}, forceYes)

		if err != nil {
			return fmt.Errorf("could not download and extract the image: %v", err)
		}

		defer tempImagePath.Parent().RemoveAll()

		imagePath = tempImagePath
	} else if !imagePath.IsDir() {
		temp, err := paths.MkTempDir("", "debian-image-")
		if err != nil {
			return fmt.Errorf("error creating a temporary directory to extract the archive: %v", err)
		}
		defer temp.RemoveAll()

		err = ExtractImage(imagePath, temp)
		if err != nil {
			return fmt.Errorf("error extracting the archive: %v", err)
		}

		tempContent, err := temp.ReadDir(paths.AndFilter(paths.FilterDirectories(), paths.FilterPrefixes("arduino-unoq-debian-image-")))
		if err != nil {
			return fmt.Errorf("could not find Debian image directory: %v", err)
		}

		imagePath = tempContent[0]
	}

	return FlashBoard(ctx, imagePath.String())
}

func FlashBoard(ctx context.Context, downloadedImagePath string) error {
	qdl, err := getQdlBytes()
	if err != nil {
		return err
	}

	var flashDir *paths.Path
	for _, entry := range []string{"flash", "flash_UnoQ"} {
		if p := paths.New(downloadedImagePath, entry); p.Exist() {
			flashDir = p
			break
		}
	}
	if flashDir == nil {
		return fmt.Errorf("could not find the `flash` directory")
	}

	qdlDir, err := paths.MkTempDir("", "qdl-")
	if err != nil {
		return err
	}
	defer qdlDir.RemoveAll()

	qdlPath := qdlDir.Join("qdl")
	if runtime.GOOS == "windows" {
		qdlPath = qdlDir.Join("qdl.exe")
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
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--allow-missing", "--storage", "emmc", "prog_firehose_ddr.elf", "rawprogram0.xml", "patch0.xml")
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

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

package updater

import (
	"context"
	"fmt"
	"runtime"

	"github.com/arduino/go-paths-helper"
	"github.com/shirou/gopsutil/v4/disk"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/updater/artifacts"
)

const GiB = uint64(1024 * 1024 * 1024)
const DownloadDiskSpace = uint64(12)
const ExtractDiskSpace = uint64(10)

func Flash(ctx context.Context, imagePath *paths.Path, version string, forceYes bool, tempDir string) error {
	if !imagePath.Exist() {
		temp, err := SetTempDir("download-", tempDir)
		if err != nil {
			return fmt.Errorf("error creating a temporary directory to extract the archive: %v", err)
		}
		defer func() { _ = temp.RemoveAll() }()

		// Check if there is enough free disk space before downloading and extracting an image
		d, err := disk.Usage(temp.String())
		if err != nil {
			return err
		}
		if d.Free/GiB < DownloadDiskSpace {
			return fmt.Errorf("download and extraction requires up to %d GiB of free space", DownloadDiskSpace)
		}

		tempImagePath, v, err := DownloadAndExtract(ctx, version, temp)

		if err != nil {
			return fmt.Errorf("could not download and extract the image: %v", err)
		}

		version = v
		imagePath = tempImagePath
	} else if !imagePath.IsDir() {
		temp, err := SetTempDir("extract-", tempDir)
		if err != nil {
			return fmt.Errorf("error creating a temporary directory to extract the archive: %v", err)
		}
		defer func() { _ = temp.RemoveAll() }()

		// Check if there is enough free disk space before extracting an image
		d, err := disk.Usage(temp.String())
		if err != nil {
			return err
		}
		if d.Free/GiB < ExtractDiskSpace {
			return fmt.Errorf("extraction requires up to %d GiB of free space", ExtractDiskSpace)
		}

		err = ExtractImage(ctx, imagePath, temp)
		if err != nil {
			return fmt.Errorf("error extracting the archive: %v", err)
		}

		tempContent, err := temp.ReadDir(paths.AndFilter(paths.FilterDirectories(), paths.FilterPrefixes("arduino-unoq-debian-image-")))
		if err != nil {
			return fmt.Errorf("could not find Debian image directory: %v", err)
		}

		imagePath = tempContent[0]
	}

	return FlashBoard(ctx, imagePath.String(), version)
}

func FlashBoard(ctx context.Context, downloadedImagePath string, version string) error {
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
	defer func() { _ = qdlDir.RemoveAll() }()

	qdlPath := qdlDir.Join("qdl")
	if runtime.GOOS == "windows" {
		qdlPath = qdlDir.Join("qdl.exe")
	}

	err = qdlPath.WriteFile(artifacts.QdlBinary)
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
	feedback.Print(i18n.Tr("Flashing with qdl"))
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

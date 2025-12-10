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
	"encoding/hex"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/shirou/gopsutil/v4/disk"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/updater/artifacts"
)

const GiB = uint64(1024 * 1024 * 1024)
const DownloadDiskSpace = uint64(12)
const ExtractDiskSpace = uint64(10)
const yesPrompt = "yes"

func Flash(ctx context.Context, imagePath *paths.Path, version string, forceYes bool, preserveUser bool, tempDir string) error {
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

	return FlashBoard(ctx, imagePath.String(), version, preserveUser)
}

func FlashBoard(ctx context.Context, downloadedImagePath string, version string, preserveUser bool) error {
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

	rawProgram := "rawprogram0.xml"
	if preserveUser {
		if errT := checkBoardGPTTable(ctx, qdlPath, flashDir); errT == nil && flashDir.Join("rawprogram0.nouser.xml").Exist() {
			rawProgram = "rawprogram0.nouser.xml"
		} else {
			res, err := func(target string) (bool, error) {
				warnStr := "Linux image " + target + " does not support user partition preservation"
				if errT != nil {
					warnStr = errT.Error()
				}
				feedback.Printf("\nWARNING: %s.", warnStr)
				feedback.Printf("Do you want to proceed and flash %s on the board, erasing any existing data you have on it? (yes/no)", target)

				var yesInput string
				_, err := fmt.Scanf("%s\n", &yesInput)
				if err != nil {
					return false, err
				}
				yes := strings.ToLower(yesInput) == yesPrompt || strings.ToLower(yesInput) == "y"
				return yes, nil
			}(version)
			if err != nil {
				return err
			}
			if !res {
				return fmt.Errorf("flashing not confirmed by user, exiting")
			}
		}

	}

	feedback.Print(i18n.Tr("Flashing with qdl"))
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--allow-missing", "--storage", "emmc", "prog_firehose_ddr.elf", rawProgram, "patch0.xml")
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

func checkBoardGPTTable(ctx context.Context, qdlPath, flashDir *paths.Path) error {
	dumpBinPath := qdlPath.Parent().Join("dump.bin")
	readXMLPath := qdlPath.Parent().Join("read.xml")
	err := readXMLPath.WriteFile(artifacts.ReadXML)
	if err != nil {
		return err
	}
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--storage", "emmc", flashDir.Join("prog_firehose_ddr.elf").String(), readXMLPath.String())
	if err != nil {
		return err
	}
	cmd.SetDir(qdlPath.Parent().String())
	if err := cmd.RunWithinContext(ctx); err != nil {
		return err
	}
	if !dumpBinPath.Exist() {
		return fmt.Errorf("it was not possible to access the current Debian image GPT table")
	}
	dump, err := dumpBinPath.ReadFile()
	if err != nil {
		return err
	}
	strDump := hex.Dump(dump)

	strDumpSlice := strings.Split(strDump, "\n")
	// the max number of partitions is stored at entry 0x50
	maxPartitions, err := strconv.ParseInt(strings.Split(strDumpSlice[5], " ")[2], 16, 16)
	if err != nil {
		return err
	}

	numPartitions := 0
	// starting from entry 0x200, there is a new partition every 0x80 bytes
	// TODO: check if the size of each partition is 80h or just assume it?
	for i := 32; numPartitions < int(maxPartitions); i += 8 {
		// partitions are made of non-zero bytes, if all 0s then there are no more entries
		if strings.Contains(strDumpSlice[i], "00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00") {
			break
		}
		numPartitions++
	}

	if numPartitions == 73 && maxPartitions == 76 {
		return fmt.Errorf("the current Debian image (R0) does not support user partition preservation")
	}

	return nil
}

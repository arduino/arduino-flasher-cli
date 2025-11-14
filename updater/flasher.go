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
	"strings"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-flasher-cli/feedback"
	"github.com/arduino/arduino-flasher-cli/i18n"
	"github.com/arduino/arduino-flasher-cli/updater/artifacts"
)

func Flash(ctx context.Context, imagePath *paths.Path, version string, forceYes bool, preserveUser bool) error {
	if !imagePath.Exist() {
		client := NewClient()

		tempImagePath, v, err := DownloadAndExtract(client, version, func(target string) (bool, error) {
			feedback.Printf("Found Debian image version: %s", target)
			feedback.Printf("Do you want to download it? (yes/no)")

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

		// Download not confirmed
		if tempImagePath == nil {
			return nil
		}

		defer func() { _ = tempImagePath.Parent().RemoveAll() }()

		version = v
		imagePath = tempImagePath
	} else if !imagePath.IsDir() {
		temp, err := GetTempDir("extract-")
		if err != nil {
			return fmt.Errorf("error creating a temporary directory to extract the archive: %v", err)
		}
		defer func() { _ = temp.RemoveAll() }()

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

	return FlashBoard(ctx, imagePath.String(), version, func(target string) (bool, error) {
		feedback.Print("\nWARNING: flashing a new Linux image on the board will erase any existing data you have on it.")
		feedback.Printf("Do you want to proceed and flash %s on the board? (yes/no)", target)

		var yesInput string
		_, err := fmt.Scanf("%s\n", &yesInput)
		if err != nil {
			return false, err
		}
		yes := strings.ToLower(yesInput) == "yes" || strings.ToLower(yesInput) == "y"
		return yes, nil
	}, forceYes, preserveUser)
}

func FlashBoard(ctx context.Context, downloadedImagePath string, version string, upgradeConfirmCb DownloadConfirmCB, forceYes bool, preserveUser bool) error {
	if !forceYes && !preserveUser {
		res, err := upgradeConfirmCb(version)
		if err != nil {
			return err
		}
		if !res {
			feedback.Print(i18n.Tr("Flashing not confirmed by user, exiting"))
			return nil
		}
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
		if ok, errT := checkBoardGPTTable(ctx, qdlPath, flashDir); ok && errT == nil {
			rawProgram = "rawprogram0.nouser.xml"
		} else {
			res, err := func(target string) (bool, error) {
				feedback.Printf("\nWARNING: %v.\nFlashing a new Linux image on the board will erase any existing data you have on it.", errT)
				feedback.Printf("Do you want to proceed and flash %s on the board? (yes/no)", target)

				var yesInput string
				_, err := fmt.Scanf("%s\n", &yesInput)
				if err != nil {
					return false, err
				}
				yes := strings.ToLower(yesInput) == "yes" || strings.ToLower(yesInput) == "y"
				return yes, nil
			}(version)
			if err != nil {
				return err
			}
			if !res {
				feedback.Print(i18n.Tr("Flashing not confirmed by user, exiting"))
				return nil
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

	feedback.Print("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper.")

	return nil
}

func checkBoardGPTTable(ctx context.Context, qdlPath, flashDir *paths.Path) (bool, error) {
	dumpBinPath := qdlPath.Parent().Join("dump.bin")
	readXMLPath := qdlPath.Parent().Join("read.xml")
	err := readXMLPath.WriteFile(artifacts.ReadXML)
	if err != nil {
		return false, err
	}
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--storage", "emmc", flashDir.Join("prog_firehose_ddr.elf").String(), readXMLPath.String())
	if err != nil {
		return false, err
	}
	cmd.SetDir(qdlPath.Parent().String())
	if err := cmd.RunWithinContext(ctx); err != nil {
		return false, err
	}
	if !dumpBinPath.Exist() {
		return false, fmt.Errorf("it was not possible to access the current Debian image GPT table")
	}
	dump, err := dumpBinPath.ReadFile()
	if err != nil {
		return false, err
	}
	strDump := hex.Dump(dump)

	if strings.Contains(strDump, "00000250  4c 00 00 00") {
		fmt.Println("R0")
		return false, fmt.Errorf("the current Debian image (R0) does not support user partition preservation")
	}

	return true, nil
}

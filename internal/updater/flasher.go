// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/fatih/color"
	"github.com/shirou/gopsutil/v4/disk"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/helper"
	"github.com/arduino/arduino-flasher-cli/internal/types/serial"
	"github.com/arduino/arduino-flasher-cli/internal/updater/artifacts"
)

const GiB = uint64(1024 * 1024 * 1024)
const DownloadDiskSpace = uint64(12)
const ExtractDiskSpace = uint64(10)
const yesPrompt = "yes"
const boardSize16GB = uint64(16)
const boardSize32GB = uint64(32)

func Flash(ctx context.Context, imagePath *paths.Path, version string, forceYes bool, preserveUser bool, tempDir string, rootSize uint64, callback FlashCallback) error {
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

		v, err := DownloadAndExtract(ctx, version, temp)

		if err != nil {
			return fmt.Errorf("could not download and extract the image: %v", err)
		}

		version = v
		imagePath = temp
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

		imagePath = temp
	}

	return FlashBoard(ctx, "", imagePath, version, preserveUser, rootSize, nil)
}

type FlashEvent struct {
	Log      string
	Progress int
	Total    int
}

type FlashCallback func(FlashEvent)

func FlashBoard(ctx context.Context, serialStr string, downloadedImagePath *paths.Path, version string, preserveUser bool, rootSize uint64, callback FlashCallback) error {
	qdlPath, cleanup, err := installQdl()
	if err != nil {
		return err
	}
	defer cleanup()

	flashDir, err := searchForFlashDir(downloadedImagePath)
	if err != nil {
		return err
	}

	feedback.Print(i18n.Tr("Checking UNO Q board size and OS version. Please connect the board in EDL mode."))
	gpt, err := readGPTTable(ctx, qdlPath, flashDir)
	if err != nil {
		return err
	}

	_, err = checkBoardSize(gpt)
	if err != nil {
		return err
	}

	rawProgram := "rawprogram0.xml"
	if preserveUser {
		if errT := checkUserPartitionPreservation(gpt); errT == nil && flashDir.Join("rawprogram0.nouser.xml").Exist() {
			rawProgram = "rawprogram0.nouser.xml"
		} else if callback != nil {
			return fmt.Errorf("it will not be possible to preserve the data: %w", errT)
		} else {
			res, err := func(target string) (bool, error) {
				warnStr := "Linux image " + target + " does not support user partition preservation"
				if errT != nil {
					warnStr = errT.Error()
				}
				feedback.Print(color.RedString("\nWARNING: %s. It will not be possible to preserve your data.\n", warnStr))
				feedback.Printf("Do you want to proceed and flash %s on the board? (yes/no)", target)

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

	} else if rootSize > 0 {
		fmt.Printf("Resizing root partition to %d bytes...\n", rootSize)

		gptTableFile := flashDir.Join("gpt_main0.bin")

		table, err := ParseGptTable(gptTableFile)
		if err != nil {
			return fmt.Errorf("could not parse GPT table: %w", err)
		}

		if err := table.ResizeRoot(gptTableFile, rootSize); err != nil {
			return fmt.Errorf("could not resize root partition in GPT table: %w", err)
		}

		rawProgramFile := flashDir.Join(rawProgram)
		if err = MoveUserdata(rawProgramFile, rootSize); err != nil {
			return fmt.Errorf("could not move userdata partition in rawprogram0.xml: %w", err)
		}
	}

	totalPartitions, err := getTotalPartition(flashDir.Join(rawProgram))
	if err != nil {
		return err
	}

	args := []string{qdlPath.String(), "--allow-missing", "--storage", "emmc", "prog_firehose_ddr.elf", rawProgram, "patch0.xml"}

	if serialStr != "" {
		serial, err := serial.FromNum(serialStr)
		if err != nil {
			return err
		}
		args = append(args, "--serial", serial.Hex())
	}

	feedback.Print(i18n.Tr("Flashing with qdl"))
	cmd, err := paths.NewProcess(nil, args...)
	if err != nil {
		return err
	}
	// Setting the directory is needed because rawprogram0.xml contains relative file paths
	cmd.SetDir(flashDir.String())

	if callback != nil {
		progress := 0
		w := helper.NewCallbackWriter(func(line string) {
			parsedLine := parseQdlLogLine(line)
			if parsedLine.Op == Flashed {
				progress++
			}
			callback(FlashEvent{
				Log:      line,
				Progress: progress,
				Total:    totalPartitions,
			})
		})
		cmd.RedirectStderrTo(w)
		cmd.RedirectStdoutTo(w)
	} else {
		stdout, _, err := feedback.DirectStreams()
		if err != nil {
			return err
		}
		cmd.RedirectStderrTo(stdout)
		cmd.RedirectStdoutTo(stdout)
	}

	if err := cmd.RunWithinContext(ctx); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 && runtime.GOOS == "linux" {
				return fmt.Errorf("exit status %d\nPossible qcserial issue?\nSee https://docs.arduino.cc/tutorials/uno-q/update-image/#fixing-qcserial-issue-linux-only for details", exitErr.ExitCode())
			}
		}
		return err
	}

	return nil
}

func searchForFlashDir(extractPath *paths.Path) (*paths.Path, error) {
	pathList, err := extractPath.ReadDirRecursiveFiltered(func(p *paths.Path) bool {
		return p.IsDir()
	}, func(p *paths.Path) bool {
		return p.IsDir() && (p.Base() == "flash" || p.Base() == "flash_UnoQ")
	})
	if err != nil {
		return nil, fmt.Errorf("could not find the `flash` directory: %w", err)
	}
	switch len(pathList) {
	case 1:
		return pathList[0], nil
	case 0:
		return nil, fmt.Errorf("could not find the `flash` directory in: %s", extractPath.String())
	default:
		return nil, fmt.Errorf("multiple `flash` directories found in: %s", extractPath.String())
	}
}

func installQdl() (*paths.Path, func(), error) {
	qdlDir, err := paths.MkTempDir("", "qdl-")
	if err != nil {
		return nil, nil, err
	}

	qdlPath := qdlDir.Join("qdl")
	if runtime.GOOS == "windows" {
		qdlPath = qdlDir.Join("qdl.exe")
	}

	if err = qdlPath.WriteFile(artifacts.QdlBinary); err != nil {
		return nil, nil, err
	}
	if err = qdlPath.Chmod(0755); err != nil {
		return nil, nil, err
	}

	return qdlPath, func() { _ = qdlDir.RemoveAll() }, nil
}

// readGPTTable reads the GPT table of the board performing a qdl read
func readGPTTable(ctx context.Context, qdlPath, flashDir *paths.Path) (string, error) {
	dumpBinPath := flashDir.Join("dump.bin")
	readXMLPath := qdlPath.Parent().Join("read.xml")
	err := readXMLPath.WriteFile(artifacts.ReadXML)
	if err != nil {
		return "", err
	}
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--storage", "emmc", "prog_firehose_ddr.elf", readXMLPath.String())
	if err != nil {
		return "", err
	}
	cmd.SetDir(flashDir.String())
	if err := cmd.RunWithinContext(ctx); err != nil {
		return "", err
	}
	if !dumpBinPath.Exist() {
		return "", fmt.Errorf("it was not possible to access the current Debian image GPT table")
	}
	defer func() {
		_ = dumpBinPath.Remove()
	}()
	dump, err := dumpBinPath.ReadFile()
	if err != nil {
		return "", err
	}
	return hex.Dump(dump), nil
}

// checkUserPartitionPreservation checks the board GPT table and counts the number of partitions, to tell if the board supports preserving the user's data.
func checkUserPartitionPreservation(gpt string) error {
	gptSlice := strings.Split(gpt, "\n")
	// the max number of partitions is stored at entry 0x50
	maxPartitions, err := strconv.ParseInt(strings.Split(gptSlice[5], " ")[2], 16, 16)
	if err != nil {
		return err
	}

	numPartitions := 0
	// starting from entry 0x200, there is a new partition every 0x80 bytes
	// TODO: check if the size of each partition is 80h or just assume it?
	for i := 32; numPartitions < int(maxPartitions); i += 8 {
		// partitions are made of non-zero bytes, if all 0s then there are no more entries
		if strings.Contains(gptSlice[i], "00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00") {
			break
		}
		numPartitions++
	}

	if numPartitions == 73 && maxPartitions == 76 {
		return fmt.Errorf("the current Debian image (R0) does not support user partition preservation")
	}

	return nil
}

// checkBoardSize checks the board size by reading the GPT table and looking for the value of the last usable LBA for partitions.
func checkBoardSize(gpt string) (uint64, error) {
	if strings.Contains(gpt, "00000030  de 27 d3 01") {
		return boardSize16GB, nil
	} else if strings.Contains(gpt, "00000030  de df a3 03") {
		return boardSize32GB, nil
	}
	return 0, fmt.Errorf("unknown board size")
}

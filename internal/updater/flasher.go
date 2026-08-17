// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/shirou/gopsutil/v4/disk"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/helper"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/types/serial"
	"github.com/arduino/arduino-flasher-cli/internal/updater/artifacts"
)

const (
	GiB               = uint64(1024 * 1024 * 1024)
	DownloadDiskSpace = uint64(12)
	ExtractDiskSpace  = uint64(10)
	yesPrompt         = "yes"
)

type FlashOptions struct {
	Serial       string
	PreserveUser bool
	TempDir      string
	RootSize     uint64
}

// DownloadAndFlash fetches the release and flashes it. It is the composition the
// CLI needs; the daemon reports progress between the steps and uses them
// directly instead.
func DownloadAndFlash(ctx context.Context, board registry.Board, index registry.Os, imageVersion string, opts FlashOptions) error {
	temp, err := SetTempDir("download-", opts.TempDir)
	if err != nil {
		return fmt.Errorf("error creating a temporary directory to extract the archive: %v", err)
	}
	defer func() { _ = temp.RemoveAll() }()

	if err := checkFreeSpace(temp, DownloadDiskSpace, "download and extraction"); err != nil {
		return err
	}

	version, err := DownloadAndExtract(ctx, index, imageVersion, temp)
	if err != nil {
		return fmt.Errorf("could not download and extract the image: %v", err)
	}

	return FlashBoard(ctx, opts.Serial, temp, version, board, opts.PreserveUser, opts.RootSize, nil)
}

// FlashImage flashes an image already on disk, extracting it first when it is an
// archive.
func FlashImage(ctx context.Context, imagePath *paths.Path, opts FlashOptions) error {
	if !imagePath.IsDir() {
		temp, err := SetTempDir("extract-", opts.TempDir)
		if err != nil {
			return fmt.Errorf("error creating a temporary directory to extract the archive: %v", err)
		}
		defer func() { _ = temp.RemoveAll() }()

		if err := checkFreeSpace(temp, ExtractDiskSpace, "extraction"); err != nil {
			return err
		}

		if err := ExtractImage(ctx, imagePath, temp); err != nil {
			return fmt.Errorf("error extracting the archive: %v", err)
		}
		imagePath = temp
	}

	return FlashBoard(ctx, opts.Serial, imagePath, imagePath.Base(), registry.UnoQ, opts.PreserveUser, opts.RootSize, nil)
}

func checkFreeSpace(dir *paths.Path, requiredGiB uint64, what string) error {
	d, err := disk.Usage(dir.String())
	if err != nil {
		return err
	}
	if d.Free/GiB < requiredGiB {
		return fmt.Errorf("%s requires up to %d GiB of free space, %s has %s",
			what, requiredGiB, dir, humanize.IBytes(d.Free))
	}
	return nil
}

type FlashEvent struct {
	Log      string
	Progress int
	Total    int
}

type FlashCallback func(FlashEvent)

const (
	// MinRootSize is a floor for a requested root partition. The rootfs shipped
	// in the image is around 7.3 GiB, so anything near that cannot hold it.
	MinRootSize          = 9 * GiB
	MinUserPartitionSize = 2 * GiB
	// SystemReservedBytes is the approximate space taken by Qualcomm system
	// partitions (xbl, abl, boot, tz, ...) before rootfs starts. Used only
	// for the home-partition preview in the wizard; actual sizes are decided
	// by the GPT shipped with the image.
	SystemReservedBytes uint64 = 1 * GiB
)

func FlashBoard(ctx context.Context, serialStr string, downloadedImagePath *paths.Path, version string, boardType registry.Board, preserveUser bool, rootSize uint64, callback FlashCallback) error {
	qdlPath, cleanup, err := installQdl()
	if err != nil {
		return err
	}
	defer cleanup()

	flashDir, err := searchForFlashDir(downloadedImagePath)
	if err != nil {
		return err
	}

	rawProgram := "rawprogram0.xml"
	if boardType == registry.UnoQ {
		feedback.Print(i18n.Tr("Checking board size and image version. Please connect the board in EDL mode."))
		boardGPT, err := readBoardGPTTable(ctx, qdlPath, flashDir)
		if err != nil {
			return err
		}

		boardSize := getBoardSize(boardGPT)
		if rootSize > 0 && rootSize > boardSize-MinUserPartitionSize-SystemReservedBytes {
			return fmt.Errorf("root size exceeds available space. Max size: %d GiB, requested root size: %d GiB", (boardSize-MinUserPartitionSize-SystemReservedBytes)/GiB, rootSize/GiB)
		}
		if rootSize == 0 && !preserveUser {
			// An unknown capacity just means there is no default to apply.
			if variant, err := registry.VariantFor(boardType, boardSize); err == nil {
				rootSize = variant.DefaultRootSize
			}
		}

		if preserveUser {
			if errT := checkUserPartitionPreservation(boardGPT); errT == nil && flashDir.Join("rawprogram0.nouser.xml").Exist() {
				rawProgram = "rawprogram0.nouser.xml"

				rootBoardSize := boardGPT.RootPartition.SizeInBytes()
				imageTable, err := ParseGptTable(flashDir.Join("gpt_main0.bin"))
				if err != nil {
					return err
				}
				rootXmlSize := imageTable.RootPartition.SizeInBytes()
				if rootBoardSize != rootXmlSize {
					binCleanup, err := imageTable.ResizeRoot(flashDir.Join("gpt_main0_resized.bin"), rootBoardSize)
					if err != nil {
						return fmt.Errorf("could not resize root partition in GPT table: %w", err)
					}
					defer binCleanup()

					rawProgramFile := flashDir.Join(rawProgram)
					rawProgram, cleanup, err = MoveUserdata(rawProgramFile, rootBoardSize)
					if err != nil {
						return fmt.Errorf("could not move userdata partition in rawprogram0.xml: %w", err)
					}
					defer cleanup()
				}
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
			gptTableFile := flashDir.Join("gpt_main0.bin")

			table, err := ParseGptTable(gptTableFile)
			if err != nil {
				return fmt.Errorf("could not parse GPT table: %w", err)
			}

			binCleanup, err := table.ResizeRoot(flashDir.Join("gpt_main0_resized.bin"), rootSize)
			if err != nil {
				return fmt.Errorf("could not resize root partition in GPT table: %w", err)
			}
			defer binCleanup()

			rawProgramFile := flashDir.Join(rawProgram)
			rawProgram, cleanup, err = MoveUserdata(rawProgramFile, rootSize)
			if err != nil {
				return fmt.Errorf("could not move userdata partition in rawprogram0.xml: %w", err)
			}
			defer cleanup()
		}
		feedback.Print(i18n.Tr("Flashing with qdl [root partition size: %dGiB]", cmp.Or(rootSize, boardGPT.RootPartition.SizeInBytes())/GiB))
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
		return fmt.Errorf("%w%s", err, qdlExitHint(err))
	}

	return nil
}

// qdlExitHint names the known cause of a qdl failure, when there is one.
func qdlExitHint(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && runtime.GOOS == "linux" {
		return "\nPossible qcserial issue? See https://docs.arduino.cc/tutorials/uno-q/update-image/#fixing-qcserial-issue-linux-only for details"
	}
	return ""
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

// readBoardGPTTable reads the GPT table of the board performing a qdl read
func readBoardGPTTable(ctx context.Context, qdlPath, flashDir *paths.Path) (GptTable, error) {
	dumpBinPath := flashDir.Join("dump.bin")
	readXMLPath := qdlPath.Parent().Join("read.xml")
	err := readXMLPath.WriteFile(artifacts.ReadXML)
	if err != nil {
		return GptTable{}, err
	}
	cmd, err := paths.NewProcess(nil, qdlPath.String(), "--storage", "emmc", "prog_firehose_ddr.elf", readXMLPath.String())
	if err != nil {
		return GptTable{}, err
	}
	cmd.SetDir(flashDir.String())
	var out bytes.Buffer
	cmd.RedirectStdoutTo(&out)
	cmd.RedirectStderrTo(&out)
	if err := cmd.RunWithinContext(ctx); err != nil {
		return GptTable{}, fmt.Errorf("could not read the board's partition table: %w%s\n%s", err, qdlExitHint(err), out.String())
	}
	if !dumpBinPath.Exist() {
		return GptTable{}, fmt.Errorf("it was not possible to access the current Debian image GPT table")
	}
	defer func() { _ = dumpBinPath.Remove() }()

	gptTable, err := ParseGptTable(dumpBinPath)
	if err != nil {
		return GptTable{}, fmt.Errorf("could not parse GPT table: %w", err)
	}

	return gptTable, nil
}

// checkUserPartitionPreservation checks the board GPT table and counts the number of partitions, to tell if the board supports preserving the user's data.
func checkUserPartitionPreservation(gpt GptTable) error {
	if gpt.PartitionCount == 73 && gpt.Header.NumPartitions == 76 {
		return fmt.Errorf("the current Debian image (R0) does not support user partition preservation")
	}
	return nil
}

func getBoardSize(gpt GptTable) uint64 {
	return (gpt.Header.LastLBA + 1) * 512

}

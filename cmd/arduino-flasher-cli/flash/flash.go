// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package flash

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/arduino/go-paths-helper"
	runas "github.com/arduino/go-windows-runas"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/interactive"
	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

const minRootSize uint64 = 9 * 1024 * 1024 * 1024 // 9GiB

func NewFlashCmd() *cobra.Command {
	var forceYes, preserveUser bool
	var tempDir string
	var rootSizeStr string
	appCmd := &cobra.Command{
		Use:   "flash",
		Short: "Flash a Debian image on the board",
		Long: `Flash a Debian image on the board.

WARNING: This operation will completely replace the current system on the board.
Make sure to backup any important data before proceeding.

This command accepts either:
  - A local file path to a compressed Debian image file (e.g., .tar.zst, .tar.xz) or a decompressed Debian image folder
  - A version tag to download from the remote repository

When providing a local file path:
  - The path can be relative or absolute
  - The file is extracted in a temp folder (if needed)
  - The image is flashed into the board

When providing a version tag:
  - Use 'latest' to download the most recent stable image
  - Use a specific version tag (e.g., '20250915-173') to download that exact version
  - The image will be automatically downloaded (in a temp folder), the sha is verified, and flashed


NOTE: On Windows, required drivers are automatically installed with elevated privileges.
`,
		Example: " " + os.Args[0] + " flash latest\n" +
			" " + os.Args[0] + " flash 20250915-173\n" +
			" " + os.Args[0] + " flash ./my-image.tar.zst\n" +
			" " + os.Args[0] + " flash /path/to/debian-image.tar.zst\n" +
			" " + os.Args[0] + " flash /path/to/debian-image.tar.xz \n" +
			" " + os.Args[0] + " flash /path/to/arduino-unoq-debian-image-20250915-173 \n" +
			" " + os.Args[0] + " flash latest --temp-dir /path/to/custom/tempDir \n" +
			" " + os.Args[0] + " flash latest --preserve-user \n" +
			" " + os.Args[0] + " flash latest --root-size 12GB \n",

		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			checkDriversInstalled()
			if len(args) == 0 {
				interactive.Run(cmd.Context())
				return
			}

			rootSize := uint64(0)
			if rootSizeStr != "" {
				if preserveUser {
					feedback.Fatal(i18n.Tr("cannot specify root-size when preserve-user is enabled"), feedback.ErrBadArgument)
				}

				var err error
				rootSize, err = humanize.ParseBytes(rootSizeStr)
				if err != nil {
					feedback.Fatal(i18n.Tr("invalid root-size value: %v", err), feedback.ErrBadArgument)
				}

				if rootSize < minRootSize {
					feedback.Fatal(i18n.Tr("root-size must be at least 10GB"), feedback.ErrBadArgument)
				}
			}

			runFlashCommand(cmd.Context(), args, forceYes, preserveUser, cmd.Flags().Changed("preserve-user"), tempDir, rootSize)
		},
	}
	appCmd.Flags().BoolVarP(&forceYes, "yes", "y", false, "Automatically confirm all prompts")
	appCmd.Flags().StringVar(&tempDir, "temp-dir", "", "Path to the directory in which the image will be downloaded and extracted")
	appCmd.Flags().BoolVar(&preserveUser, "preserve-user", false, "Preserve user partition")
	appCmd.Flags().StringVar(&rootSizeStr, "root-size", "", "Size of the root partition (e.g. 10GB). Leave empty for autodetection")

	return appCmd
}

func checkDriversInstalled() {
	if runtime.GOOS != "windows" {
		return
	}
	cmd, _ := os.Executable()
	pwd, _ := os.Getwd()
	if _, err := runas.RunElevated(cmd, pwd, []string{"install-drivers"}, true); err != nil {
		feedback.Fatal(i18n.Tr("error installing drivers: %v", err), feedback.ErrGeneric)
	}
}

// Prompts the user to confirm the flash operation.
func askProceedFlash(r io.Reader, imageRef string) (bool, error) {
	feedback.Print(color.RedString("\nWARNING: flashing a new Linux image will replace the system partition on the board. On UNO Q, the user partition can optionally be preserved.\n"))
	feedback.Printf("Do you want to proceed and flash %s on the board? (yes/no)", imageRef)

	var yesInput string
	if _, err := fmt.Fscanf(r, "%s\n", &yesInput); err != nil {
		return false, err
	}
	return strings.ToLower(yesInput) == "yes" || strings.ToLower(yesInput) == "y", nil
}

// Prompts the user to choose whether to preserve the user partition.
func askPreservePartition(r io.Reader) (bool, error) {
	feedback.Print("Do you want to preserve the user partition? (yes/no)")
	var preserveInput string
	if _, err := fmt.Fscanf(r, "%s\n", &preserveInput); err != nil {
		return false, err
	}
	return strings.ToLower(preserveInput) == "yes" || strings.ToLower(preserveInput) == "y", nil
}

func runFlashCommand(ctx context.Context, args []string, forceYes bool, preserveUser bool, preserveUserFlagChanged bool, tempDir string, rootSize uint64) {
	imagePath, err := paths.New(args[0]).Abs()
	if err != nil {
		feedback.Fatal(i18n.Tr("could not find image absolute path: %v", err), feedback.ErrBadArgument)
	}

	if !forceYes {
		proceed, err := askProceedFlash(os.Stdin, args[0])
		if err != nil {
			feedback.Fatal(err.Error(), feedback.ErrBadArgument)
		}
		if !proceed {
			return
		}
	}

	version, boardType, osName, err := updater.DetectBoardAndSetOs(ctx, args[0])
	if err != nil {
		feedback.Fatal(i18n.Tr("error detecting the board type: %v", err), feedback.ErrBadArgument)
	}

	// Preserving the user partition is only meaningful on UNO Q boards.
	if !forceYes && !preserveUserFlagChanged && boardType == registry.UnoQ {
		preserveUser, err = askPreservePartition(os.Stdin)
		if err != nil {
			feedback.Fatal(err.Error(), feedback.ErrBadArgument)
		}
	}

	err = updater.Flash(ctx, imagePath, version, boardType, osName, forceYes, preserveUser, tempDir, rootSize, nil)
	if err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper.")
}

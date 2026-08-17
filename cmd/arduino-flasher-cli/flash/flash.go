// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package flash

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	runas "github.com/arduino/go-windows-runas"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/interactive"
	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/selector"
	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/types/serial"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

func NewFlashCmd() *cobra.Command {
	var forceYes, preserveUser bool
	var serialStr string
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
  - A selector naming the board, and optionally the OS and the version, to download

When providing a local file path:
  - The path can be relative or absolute
  - The file is extracted in a temp folder (if needed)
  - The image is flashed into the board

When providing a selector:
  - Name the board to get its most recent image, e.g. 'unoq'
  - Add the OS when the board publishes more than one, e.g. 'unoq-debian'
  - Add a version tag to pick an exact image, e.g. 'unoq-debian-20250915-173'
  - The image will be automatically downloaded (in a temp folder), the sha is verified, and flashed

The board has to be named: the connected board cannot be identified on its own.


NOTE: On Windows, required drivers are automatically installed with elevated privileges.
`,
		Example: " " + os.Args[0] + " flash unoq\n" +
			" " + os.Args[0] + " flash unoq-debian-20250915-173\n" +
			" " + os.Args[0] + " flash ./my-image.tar.zst\n" +
			" " + os.Args[0] + " flash /path/to/debian-image.tar.zst\n" +
			" " + os.Args[0] + " flash /path/to/debian-image.tar.xz \n" +
			" " + os.Args[0] + " flash /path/to/arduino-unoq-debian-image-20250915-173 \n" +
			" " + os.Args[0] + " flash unoq --temp-dir /path/to/custom/tempDir \n" +
			" " + os.Args[0] + " flash unoq --preserve-user \n" +
			" " + os.Args[0] + " flash unoq --root-size 12GB \n",

		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			checkDriversInstalled()
			if serialStr != "" {
				s, err := serial.FromHex(serialStr)
				if err != nil {
					feedback.Fatal(i18n.Tr("invalid --serial value %q: must be a hexadecimal string (e.g. 0004F3A1 or 0x0004F3A1)", serialStr), feedback.ErrBadArgument)
				}
				// The updater and daemon RPC consume the serial as a decimal integer
				// (see issue #96). Convert the hex CLI input to decimal here.
				serialStr = s.Decimal()
			}
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

				if rootSize < updater.MinRootSize {
					feedback.Fatal(i18n.Tr("root-size must be at least %s", humanize.IBytes(updater.MinRootSize)), feedback.ErrBadArgument)
				}
			}

			runFlashCommand(cmd.Context(), args, serialStr, forceYes, preserveUser, tempDir, rootSize)
		},
	}
	appCmd.Flags().StringVarP(&serialStr, "serial", "s", "", "Serial port of the board as hexadecimal string (e.g., 0x12345678). If not specified, the first board found will be used")
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

func runFlashCommand(ctx context.Context, args []string, serialStr string, forceYes bool, preserveUser bool, tempDir string, rootSize uint64) {
	imagePath, release, err := selector.Parse(args[0])
	if err != nil {
		feedback.Fatal(err.Error(), feedback.ErrBadArgument)
	}

	if !forceYes && !preserveUser {
		feedback.Print(color.RedString("\nWARNING: flashing a new Linux image will erase any existing data that you have on the board.\n"))
		feedback.Printf("Do you want to proceed and flash %s on the board? (yes/no)", args[0])

		var yesInput string
		_, err := fmt.Scanf("%s\n", &yesInput)
		if err != nil {
			feedback.Fatal(err.Error(), feedback.ErrBadArgument)
		}
		yes := strings.ToLower(yesInput) == "yes" || strings.ToLower(yesInput) == "y"

		if !yes {
			return
		}
	}

	opts := updater.FlashOptions{
		Serial:       serialStr,
		PreserveUser: preserveUser,
		TempDir:      tempDir,
		RootSize:     rootSize,
	}
	if imagePath != nil {
		err = updater.FlashImage(ctx, imagePath, opts)
	} else {
		err = updater.DownloadAndFlash(ctx, release, opts)
	}
	if err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper.")
}

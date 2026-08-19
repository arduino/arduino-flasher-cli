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

	"github.com/arduino/go-paths-helper"
	runas "github.com/arduino/go-windows-runas"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/interactive"
	"github.com/arduino/arduino-flasher-cli/cmd/argparse"
	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/types/serial"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

func NewFlashCmd() *cobra.Command {
	var forceYes, preserveUser bool
	var serialStr string
	var tempDir string
	var rootSizeStr string
	var osStr string
	var version string
	appCmd := &cobra.Command{
		Use:   "flash [board] [image]",
		Short: "Flash a Debian image on the board",
		Long: `Flash a Debian image on the board.

WARNING: This operation will completely replace the current system on the board.
Make sure to backup any important data before proceeding.

The first argument is the board to flash, the second an OS image file already on disk.
Both arguments are optional. If no arguments are specified, the interactive wizard is
started.

If a board is specified without an OS image file:
  - The OS image is downloaded in a temp folder, verified with checksum, and flashed
  - Unless a --version is specified, the most recent OS image will be selected
  - If more than one distribution is available, it may be specified with --os

NOTE: On Windows, required drivers are automatically installed with elevated privileges.
`,
		Example: " " + os.Args[0] + " flash\n" +
			" " + os.Args[0] + " flash unoq\n" +
			" " + os.Args[0] + " flash unoq --version 20250915-173\n" +
			" " + os.Args[0] + " flash unoq ./my-image.tar.zst\n" +
			" " + os.Args[0] + " flash unoq /path/to/debian-image.tar.xz \n" +
			" " + os.Args[0] + " flash unoq /path/to/arduino-unoq-debian-image-20250915-173 \n" +
			" " + os.Args[0] + " flash unoq --temp-dir /path/to/custom/tempDir \n" +
			" " + os.Args[0] + " flash unoq --preserve-user \n" +
			" " + os.Args[0] + " flash unoq --root-size 12GB \n",

		Args: cobra.MaximumNArgs(2),
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
			// Flashing needs a board, and the wizard is what asks for one.
			if len(args) == 0 {
				interactive.Run(cmd.Context())
				return
			}
			imageArg := ""
			if len(args) > 1 {
				imageArg = args[1]
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

			runFlashCommand(cmd.Context(), args[0], imageArg, osStr, version, serialStr, forceYes, preserveUser, tempDir, rootSize)
		},
	}
	appCmd.Flags().StringVarP(&version, "version", "v", "", "Version of the image to download. Leave empty for latest")
	appCmd.Flags().StringVar(&osStr, "os", "", "Distribution to download, if more than one is available")
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

func runFlashCommand(ctx context.Context, boardID, imageArg string, imageOs string, version, serialStr string, forceYes bool, preserveUser bool, tempDir string, rootSize uint64) {
	boardID, imageArg, version = argparse.LegacyArgs(boardID, imageArg, version)

	board, ok := registry.BoardByID(boardID)
	if !ok {
		feedback.Fatal(i18n.Tr("%s is not a board, use one of: %s", boardID, strings.Join(registry.BoardIDs(), ", ")), feedback.ErrBadArgument)
	}
	imagePath, err := resolveImage(imageArg)
	if err != nil {
		feedback.Fatal(err.Error(), feedback.ErrBadArgument)
	}

	// Resolved before the prompt, so a board with no published image fails while
	// the board is still intact, and the prompt can name the version.
	var target string
	if imagePath != nil {
		target = imagePath.Base()
	} else {
		rel, err := registry.NewClient().GetReleaseByVersion(ctx, version, board.ResolveOs(imageOs), board.ID)
		if err != nil {
			feedback.Fatal(i18n.Tr("error looking up the image: %v", err), feedback.ErrBadArgument)
		}
		target = i18n.Tr("%s %s for the %s", board.ResolveOs(imageOs), rel.Version, board.Label)
		version = rel.Version
	}

	if !forceYes && !preserveUser {
		feedback.Print(color.RedString("\nWARNING: flashing a new Linux image will erase any existing data that you have on the board.\n"))
		feedback.Printf("Do you want to proceed and flash %s on the board? (yes/no)", target)

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
		err = updater.FlashImage(ctx, imagePath, board.ID, opts)
	} else {
		err = updater.DownloadAndFlash(ctx, board.ID, board.ResolveOs(imageOs), version, opts)
	}
	if err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper.")
}

// resolveImage returns the path of the image the argument points at, or nil when
// there is no argument, which is what asks for one to be downloaded.
func resolveImage(arg string) (*paths.Path, error) {
	if arg == "" {
		return nil, nil
	}
	imagePath, err := paths.New(arg).Abs()
	if err != nil {
		return nil, fmt.Errorf("could not find image absolute path: %v", err)
	}
	if !imagePath.Exist() {
		return nil, fmt.Errorf("there is no image at %s", arg)
	}
	return imagePath, nil
}

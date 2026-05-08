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
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

func NewFlashCmd() *cobra.Command {
	var forceYes, preserveUser bool
	var tempDir string
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
			" " + os.Args[0] + " flash latest --temp-dir /path/to/custom/tempDir \n",

		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			checkDriversInstalled()
			runFlashCommand(cmd.Context(), args, forceYes, preserveUser, tempDir)
		},
	}
	appCmd.Flags().BoolVarP(&forceYes, "yes", "y", false, "Automatically confirm all prompts")
	appCmd.Flags().StringVar(&tempDir, "temp-dir", "", "Path to the directory in which the image will be downloaded and extracted")
	appCmd.Flags().BoolVar(&preserveUser, "preserve-user", false, "Preserve user partition")

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

func runFlashCommand(ctx context.Context, args []string, forceYes bool, preserveUser bool, tempDir string) {
	imagePath, err := paths.New(args[0]).Abs()
	if err != nil {
		feedback.Fatal(i18n.Tr("could not find image absolute path: %v", err), feedback.ErrBadArgument)
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

	err = updater.Flash(ctx, imagePath, args[0], forceYes, preserveUser, tempDir, nil)
	if err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper.")
}

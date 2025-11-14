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

package main

import (
	"context"
	"os"
	"runtime"

	"github.com/arduino/go-paths-helper"
	runas "github.com/arduino/go-windows-runas"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/feedback"
	"github.com/arduino/arduino-flasher-cli/i18n"
	"github.com/arduino/arduino-flasher-cli/updater"
)

func newFlashCmd() *cobra.Command {
	var forceYes, preserveUser bool
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
			" " + os.Args[0] + " flash /path/to/arduino-unoq-debian-image-20250915-173 \n",

		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			checkDriversInstalled()
			runFlashCommand(cmd.Context(), args, forceYes, preserveUser)
		},
	}
	appCmd.Flags().BoolVarP(&forceYes, "yes", "y", false, "Automatically confirm all prompts")
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

func runFlashCommand(ctx context.Context, args []string, forceYes bool, preserveUser bool) {
	imagePath, err := paths.New(args[0]).Abs()
	if err != nil {
		feedback.Fatal(i18n.Tr("could not find image absolute path: %v", err), feedback.ErrBadArgument)
	}

	err = updater.Flash(ctx, imagePath, args[0], forceYes, preserveUser)
	if err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
}

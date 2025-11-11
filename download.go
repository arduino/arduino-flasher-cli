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
	"os"

	"github.com/arduino/go-paths-helper"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/feedback"
	"github.com/arduino/arduino-flasher-cli/i18n"
	"github.com/arduino/arduino-flasher-cli/updater"
)

func newDownloadCmd() *cobra.Command {
	var destDir string
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download a Linux image to the specified path",
		Args:  cobra.ExactArgs(1),
		Example: " " + os.Args[0] + " download latest\n" +
			" " + os.Args[0] + " download latest --dest-dir /tmp\n",
		Run: func(cmd *cobra.Command, args []string) {
			runDownloadCommand(args, destDir)
		},
	}
	cmd.Flags().StringVar(&destDir, "dest-dir", ".", "Path to the directory in which the image will be downloaded")

	return cmd
}

func runDownloadCommand(args []string, destDir string) {
	targetVersion := args[0]
	downloadPath := paths.New(destDir)
	if !downloadPath.IsDir() {
		feedback.Fatal(i18n.Tr("error: %s is not a directory", destDir), feedback.ErrBadArgument)
	}

	client := updater.NewClient()
	_, _, err := updater.DownloadImage(client, targetVersion, nil, true, downloadPath)
	if err != nil {
		feedback.Fatal(i18n.Tr("error downloading the image: %v", err), feedback.ErrBadArgument)
	}
}

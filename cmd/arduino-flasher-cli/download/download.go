// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package download

import (
	"context"
	"os"

	"github.com/arduino/go-paths-helper"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

func NewDownloadCmd() *cobra.Command {
	var destDir string
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download a Linux image to the specified path",
		Args:  cobra.ExactArgs(1),
		Example: " " + os.Args[0] + " download latest\n" +
			" " + os.Args[0] + " download 20251024-412\n" +
			" " + os.Args[0] + " download latest --dest-dir /tmp\n",
		Run: func(cmd *cobra.Command, args []string) {
			runDownloadCommand(cmd.Context(), args, destDir)
		},
	}
	cmd.Flags().StringVar(&destDir, "dest-dir", ".", "Path to the directory in which the image will be downloaded")

	return cmd
}

func runDownloadCommand(ctx context.Context, args []string, destDir string) {
	downloadPath := paths.New(destDir)
	if !downloadPath.IsDir() {
		feedback.Fatal(i18n.Tr("error: %s is not a directory. Please, select an existing directory.", destDir), feedback.ErrBadArgument)
	}

	version, boardType, os, err := updater.DetectBoardAndSetOs(ctx, args[0])
	if err != nil {
		feedback.Fatal(i18n.Tr("error detecting the board type: %v", err), feedback.ErrBadArgument)
	}

	downloadPath, _, err = updater.DownloadImage(ctx, version, boardType, os, downloadPath)
	if err != nil {
		feedback.Fatal(i18n.Tr("error downloading the image: %v", err), feedback.ErrBadArgument)
	}
	pathAbs, _ := downloadPath.Abs()
	feedback.Print(i18n.Tr("\nDebian image successfully downloaded: %s", pathAbs.String()))
}

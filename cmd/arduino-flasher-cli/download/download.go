// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package download

import (
	"context"
	"os"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/argparse"
	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

func NewDownloadCmd() *cobra.Command {
	var destDir string
	var osStr string
	var version string
	cmd := &cobra.Command{
		Use:   "download [board]",
		Short: "Download a Linux image to the specified path",
		Long: `Download a Linux image to the specified path.

The first argument is the board name. The most recent image is downloaded
unless a --version is specified. The --os flag selects the distribution if more
than one is available.
`,
		Args: cobra.ExactArgs(1),
		Example: " " + os.Args[0] + " download unoq\n" +
			" " + os.Args[0] + " download unoq --version 20251024-412\n" +
			" " + os.Args[0] + " download unoq --dest-dir /tmp\n",
		Run: func(cmd *cobra.Command, args []string) {
			runDownloadCommand(cmd.Context(), args[0], destDir, osStr, version)
		},
	}
	cmd.Flags().StringVarP(&version, "version", "v", "", "Version of the image to download. Leave empty for latest")
	cmd.Flags().StringVar(&osStr, "os", "", "Distribution to download, if more than one is available")
	cmd.Flags().StringVar(&destDir, "dest-dir", ".", "Path to the directory in which the image will be downloaded")

	return cmd
}

func runDownloadCommand(ctx context.Context, boardID, destDir string, imageOs string, version string) {
	boardID, _, version = argparse.LegacyArgs(boardID, "", version)

	downloadPath := paths.New(destDir)
	if !downloadPath.IsDir() {
		feedback.Fatal(i18n.Tr("error: %s is not a directory. Please, select an existing directory.", destDir), feedback.ErrBadArgument)
	}

	board, ok := registry.BoardByID(boardID)
	if !ok {
		feedback.Fatal(i18n.Tr("%s is not a board, use one of: %s", boardID, strings.Join(registry.BoardIDs(), ", ")), feedback.ErrBadArgument)
	}

	downloadPath, _, err := updater.DownloadImage(ctx, board.ResolveOs(imageOs), board.ID, version, downloadPath)
	if err != nil {
		feedback.Fatal(i18n.Tr("error downloading the image: %v", err), feedback.ErrBadArgument)
	}
	pathAbs, _ := downloadPath.Abs()
	feedback.Print(i18n.Tr("\nImage successfully downloaded: %s", pathAbs.String()))
}

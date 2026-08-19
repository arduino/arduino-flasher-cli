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
		Use:   "download",
		Short: "Download a Linux image to the specified path",
		Long: `Download a Linux image to the specified path.

The argument is the board the image is for. The most recent image is downloaded
unless --version names one, and --os selects the distribution for boards that
publish more than one.
`,
		Args: cobra.ExactArgs(1),
		Example: " " + os.Args[0] + " download unoq\n" +
			" " + os.Args[0] + " download unoq --version 20251024-412\n" +
			" " + os.Args[0] + " download unoq --dest-dir /tmp\n",
		Run: func(cmd *cobra.Command, args []string) {
			runDownloadCommand(cmd.Context(), args[0], destDir, registry.Os(osStr), version)
		},
	}
	cmd.Flags().StringVar(&destDir, "dest-dir", ".", "Path to the directory in which the image will be downloaded")
	cmd.Flags().StringVar(&osStr, "os", "", "Distribution to download, for boards that publish more than one")
	cmd.Flags().StringVar(&version, "version", "", "Version of the image to download. Leave empty for the most recent")

	return cmd
}

func runDownloadCommand(ctx context.Context, boardID, destDir string, imageOs registry.Os, version string) {
	downloadPath := paths.New(destDir)
	if !downloadPath.IsDir() {
		feedback.Fatal(i18n.Tr("error: %s is not a directory. Please, select an existing directory.", destDir), feedback.ErrBadArgument)
	}

	board, ok := registry.BoardByID(registry.BoardID(boardID))
	if !ok {
		feedback.Fatal(i18n.Tr("%s is not a board, use one of: %s", boardID, strings.Join(registry.BoardIDs(), ", ")), feedback.ErrBadArgument)
	}

	downloadPath, _, err := updater.DownloadImage(ctx, board.ResolveOs(imageOs), version, downloadPath)
	if err != nil {
		feedback.Fatal(i18n.Tr("error downloading the image: %v", err), feedback.ErrBadArgument)
	}
	pathAbs, _ := downloadPath.Abs()
	feedback.Print(i18n.Tr("\nDebian image successfully downloaded: %s", pathAbs.String()))
}

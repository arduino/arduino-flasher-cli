// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"go.bug.st/cleanup"

	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/daemon"
	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/download"
	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/drivers"
	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/flash"
	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/list"
	"github.com/arduino/arduino-flasher-cli/cmd/arduino-flasher-cli/version"
	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/service"
)

// Version will be set a build time with -ldflags
var Version string = "0.0.0-dev"
var format string

func main() {
	rootCmd := &cobra.Command{
		Use:   "arduino-flasher-cli",
		Short: "A CLI to update your Arduino UNO Q board, by downloading and flashing the latest Arduino Linux image",
		Example: " " + os.Args[0] + " flash latest\n" +
			" " + os.Args[0] + " list\n",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			format, ok := feedback.ParseOutputFormat(format)
			if !ok {
				feedback.Fatal(i18n.Tr("Invalid output format: %s", format), feedback.ErrBadArgument)
			}
			feedback.SetFormat(format)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&format, "format", "text", "Output format (text, json)")

	rootCmd.AddCommand(
		flash.NewFlashCmd(),
		drivers.NewInstallDriversCmd(),
		list.NewListCmd(),
		download.NewDownloadCmd(),
		version.NewVersionCmd(Version),
		daemon.NewDaemonCommand(service.NewFlasherServer()),
	)

	ctx := context.Background()
	ctx, _ = cleanup.InterruptableContext(ctx)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		slog.Error(err.Error())
	}
}

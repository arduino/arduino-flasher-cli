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
	srv := service.NewFlasherServer()

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
		daemon.NewDaemonCommand(srv),
	)

	ctx := context.Background()
	ctx, _ = cleanup.InterruptableContext(ctx)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		slog.Error(err.Error())
	}
}

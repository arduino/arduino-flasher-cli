// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package drivers

import (
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
)

func NewInstallDriversCmd() *cobra.Command {
	var logFile string
	cmd := &cobra.Command{
		Use:    "install-drivers",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			if err := InstallDrivers(logFile); err != nil {
				feedback.Fatal(i18n.Tr("error installing drivers: %v", err), feedback.ErrGeneric)
			}
		},
	}
	cmd.Flags().StringVar(&logFile, "log-file", "", "Path to a file where the installation output is mirrored")
	return cmd
}

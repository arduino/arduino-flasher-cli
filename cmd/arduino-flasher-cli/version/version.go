// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package version

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
)

func NewVersionCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of Arduino Flasher CLI",
		Run: func(cmd *cobra.Command, args []string) {
			feedback.PrintResult(versionResult{
				Name:    "Arduino Flasher CLI",
				Version: version,
			})

			latest, err := checkForUpdates(version)
			if err != nil {
				feedback.Warning(color.YellowString("\n\nFailed to check for updates: "+err.Error()) + "\n")
			}
			if latest != "" {
				msg := fmt.Sprintf("\n\n%s %s → %s\n%s",
					color.YellowString(i18n.Tr("A new release of Arduino Flasher CLI is available:")),
					color.CyanString(version),
					color.CyanString(latest),
					color.YellowString("https://www.arduino.cc/en/software/#flasher-tool"))
				feedback.Warning(msg)
			}
		},
	}
	return cmd
}

type versionResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (r versionResult) String() string {
	resultMessage := fmt.Sprintf("Arduino Flasher CLI version %s", r.Version)
	return resultMessage
}

func (r versionResult) Data() interface{} {
	return r
}

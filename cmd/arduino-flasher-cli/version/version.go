// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package version

import (
	"fmt"

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
				feedback.Warning(i18n.Tr("failed to check for updates: %v", err))
			}
			if latest != "" {
				// Plain: Warning colors the whole line, and a nested color
				// would reset it partway through.
				feedback.Warning(i18n.Tr("a new release of Arduino Flasher CLI is available: %s → %s\n%s",
					version, latest, "https://www.arduino.cc/en/software/#flasher-tool"))
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

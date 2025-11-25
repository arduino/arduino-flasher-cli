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

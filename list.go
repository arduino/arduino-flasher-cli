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
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/feedback"
	"github.com/arduino/arduino-flasher-cli/i18n"
	"github.com/arduino/arduino-flasher-cli/tablestyle"
	"github.com/arduino/arduino-flasher-cli/updater"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the available Linux images",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runListCommand()
		},
	}
	return cmd
}

func runListCommand() {
	client := updater.NewClient()

	manifest, err := client.GetInfoManifest()
	if err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}

	feedback.PrintResult(listResult{Latest: manifest.Latest, Releases: manifest.Releases})
	msg, err := checkForUpdates()
	if err != nil {
		feedback.Warning("\n\nfailed to check for updates: " + err.Error())
	}
	if msg != "" {
		feedback.Print(msg)
	}
}

type listResult struct {
	Latest   updater.Release   `json:"latest"`
	Releases []updater.Release `json:"releases"`
}

// Data implements Result interface
func (lr listResult) Data() interface{} {
	return lr
}

// String implements Result interface
func (lr listResult) String() string {
	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{"VERSION", "LATEST"})

	for i := len(lr.Releases) - 1; i >= 0; i-- {
		row := table.Row{lr.Releases[i].Version}
		if lr.Releases[i].Version == lr.Latest.Version {
			row = append(row, "✓")
		}
		t.AppendRow(row)
	}
	return t.Render()
}

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
	client := updater.NewClient("debian-im/Stable")

	manifest, err := client.GetInfoManifest()
	if err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}

	feedback.PrintResult(listResult{Latest: manifest.Latest, Releases: manifest.Releases})
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

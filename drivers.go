package main

import (
	"github.com/bcmi-labs/orchestrator/cmd/feedback"
	"github.com/bcmi-labs/orchestrator/cmd/i18n"
	"github.com/spf13/cobra"
)

func newInstallDriversCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "install-drivers",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			if err := installDrivers(); err != nil {
				feedback.Fatal(i18n.Tr("error installing drivers: %v", err), feedback.ErrGeneric)
			}
		},
	}
	return cmd
}

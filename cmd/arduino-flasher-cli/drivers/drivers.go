package drivers

import (
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
)

func NewInstallDriversCmd() *cobra.Command {
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

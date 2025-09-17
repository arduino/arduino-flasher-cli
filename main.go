package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bcmi-labs/orchestrator/cmd/feedback"
	"github.com/bcmi-labs/orchestrator/cmd/i18n"
	"github.com/spf13/cobra"
	"go.bug.st/cleanup"
)

// Version will be set a build time with -ldflags
var Version string = "0.0.0-dev"
var format string

func main() {
	rootCmd := &cobra.Command{
		Use:   "arduino-flasher-cli",
		Short: "A CLI to update and flash the Debian image",
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
		newFlashCmd(),
		&cobra.Command{
			Use:   "version",
			Short: "Print the version number of Arduino Flasher CLI",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Println("Arduino Flasher CLI v" + Version)
			},
		})

	ctx := context.Background()
	ctx, _ = cleanup.InterruptableContext(ctx)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		slog.Error(err.Error())
	}
}

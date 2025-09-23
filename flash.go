package main

import (
	"arduino-flasher/updater"
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/arduino/go-paths-helper"
	runas "github.com/arduino/go-windows-runas"
	"github.com/bcmi-labs/orchestrator/cmd/feedback"
	"github.com/bcmi-labs/orchestrator/cmd/i18n"
	"github.com/spf13/cobra"
)

func newFlashCmd() *cobra.Command {
	var forceYes bool
	appCmd := &cobra.Command{
		Use:   "flash",
		Short: "Flash a Debian image on the board",
		Long:  "Dowload the specified Debian image version and flash it on the board",
		Example: " " + os.Args[0] + " flash latest\n" +
			" " + os.Args[0] + " flash 20250915-173\n",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			checkDriversInstalled()
			runFlashCommand(cmd.Context(), args, forceYes)
		},
	}
	appCmd.Flags().BoolVarP(&forceYes, "yes", "y", false, "Automatically confirm all prompts")
	// TODO: add --clean-install flag or something similar to distinguish between keeping and purging the /home directory

	return appCmd
}

func checkDriversInstalled() {
	if runtime.GOOS != "windows" {
		return
	}
	cmd, _ := os.Executable()
	pwd, _ := os.Getwd()
	if _, err := runas.RunElevated(cmd, pwd, []string{"install-drivers"}, true); err != nil {
		feedback.Fatal(i18n.Tr("error installing drivers: %v", err), feedback.ErrGeneric)
	}
}

func runFlashCommand(ctx context.Context, args []string, forceYes bool) {
	version := args[0]

	updateURL := os.Getenv("UPDATE_URL")
	if updateURL == "" {
		// TODO: change to prod
		updateURL = "https://downloads.arduino.cc"
	}

	parsedURL, err := url.Parse(updateURL)
	if err != nil {
		feedback.Fatal(i18n.Tr("invalid UPDATE_URL:", err), feedback.ErrBadArgument)
	}

	headers := map[string]string{}
	clientID := os.Getenv("CF_ACCESS_CLIENT_ID")
	clientSecret := os.Getenv("CF_ACCESS_CLIENT_SECRET")
	if clientID != "" && clientSecret != "" {
		headers["CF-Access-Client-Id"] = clientID
		headers["CF-Access-Client-Secret"] = clientSecret
	}

	var client *updater.Client
	if len(headers) == 2 {
		client = updater.NewClient(parsedURL, "debian-im/Stable", updater.WithHeaders(headers))
	} else {
		client = updater.NewClient(parsedURL, "debian-im/Stable")
	}

	downloadedImagePath, updatedVersion, err := updater.DownloadImage(client, version, func(target string) (bool, error) {
		feedback.Printf("Found Debian image version: %s", target)
		feedback.Printf("Do you want to download it and flash it on the board? (yes/no)")

		var yesInput string
		_, err := fmt.Scanf("%s\n", &yesInput)
		if err != nil {
			return false, err
		}
		yes := strings.ToLower(yesInput) == "yes" || strings.ToLower(yesInput) == "y"
		return yes, nil
	}, forceYes)

	if err != nil {
		feedback.Fatal(i18n.Tr("error downloading the image: %v", err), feedback.ErrBadArgument)
	}

	if downloadedImagePath != "" {
		defer paths.New(downloadedImagePath).RemoveAll()

		if err := updater.FlashBoard(ctx, downloadedImagePath, updatedVersion); err != nil {
			feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
		}
	}
}

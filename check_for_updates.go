package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/fatih/color"
	semver "go.bug.st/relaxed-semver"

	"github.com/arduino/arduino-flasher-cli/feedback"
	"github.com/arduino/arduino-flasher-cli/i18n"
	"github.com/arduino/arduino-flasher-cli/updater"
)

type FlasherRelease struct {
	TagName string `json:"tag_name"`
}

func checkForUpdates() error {
	currentVersion, err := semver.Parse(Version)
	if err != nil {
		return err
	}

	c := updater.NewClient()
	req, err := http.NewRequest("GET", "https://api.github.com/repos/arduino/arduino-flasher-cli/releases/latest", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var release FlasherRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return err
	}

	release.TagName = strings.TrimPrefix(release.TagName, "v")
	latestVersion, err := semver.Parse(release.TagName)
	if err != nil {
		return err
	}

	// Do nothing if the Arduino Flasher CLI is up to date
	if currentVersion.GreaterThanOrEqual(latestVersion) {
		return nil
	}

	msg := fmt.Sprintf("\n\n%s %s → %s\n%s",
		color.YellowString(i18n.Tr("A new release of Arduino Flasher CLI is available:")),
		color.CyanString(currentVersion.String()),
		color.CyanString(latestVersion.String()),
		color.YellowString("https://github.com/arduino/arduino-flasher-cli/releases/latest"))
	feedback.Print(msg)

	return nil
}

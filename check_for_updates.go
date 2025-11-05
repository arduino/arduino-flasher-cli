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
		color.YellowString("https://www.arduino.cc/en/software/#flasher-tool"))
	feedback.Print(msg)

	return nil
}

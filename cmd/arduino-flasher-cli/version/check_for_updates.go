package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	semver "go.bug.st/relaxed-semver"
)

const maxTime time.Duration = 1 * time.Second

func checkForUpdates(version string) (string, error) {
	currentVersion, err := semver.Parse(version)
	if err != nil {
		return "", err
	}

	client := http.Client{Timeout: maxTime}
	resp, err := client.Get("https://api.github.com/repos/arduino/arduino-flasher-cli/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch latest release: status code %d", resp.StatusCode)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return "", err
	}

	release.TagName = strings.TrimPrefix(release.TagName, "v")
	latestVersion, err := semver.Parse(release.TagName)
	if err != nil {
		return "", err
	}

	// Do nothing if the Arduino Flasher CLI is up to date
	if currentVersion.GreaterThanOrEqual(latestVersion) {
		return "", nil
	}

	return latestVersion.String(), nil
}

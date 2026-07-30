// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"context"
	"fmt"
	"os"

	"github.com/arduino/go-paths-helper"
	"github.com/codeclysm/extract/v4"
	"github.com/schollz/progressbar/v3"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

// DownloadConfirmCB is a function that is called when a Debian image is ready to be downloaded.
type DownloadConfirmCB func(target string) (bool, error)

func DownloadAndExtract(ctx context.Context, targetVersion string, boardType string, os string, temp *paths.Path) (string, error) {
	tmpZip, version, err := DownloadImage(ctx, targetVersion, boardType, os, temp)
	if err != nil {
		return "", fmt.Errorf("error downloading the image: %v", err)
	}

	err = ExtractImage(ctx, tmpZip, temp)
	if err != nil {
		return "", fmt.Errorf("error extracting the image: %v", err)
	}

	if targetVersion == "latest" {
		version += "(latest)"
	}
	return version, nil
}

func DownloadImage(ctx context.Context, targetVersion, boardType, os string, downloadPath *paths.Path) (*paths.Path, string, error) {
	client := registry.NewClient()
	rel, err := client.GetReleaseByVersion(ctx, targetVersion, boardType, os)
	if err != nil {
		return nil, "", fmt.Errorf("could not get release info: %w", err)
	}

	var bar *progressbar.ProgressBar
	callback := func(current, total int64) {
		if bar == nil {
			bar = progressbar.DefaultBytes(
				total,
				i18n.Tr("Downloading %s image version %s", os, rel.Version),
			)
		}
		_ = bar.Set64(current)
	}
	if tmpZip, err := client.DownloadFile(ctx, downloadPath, rel, callback); err != nil {
		return nil, "", fmt.Errorf("could not download Debian image: %w", err)
	} else {
		return tmpZip, rel.Version, nil
	}
}

func ExtractImage(ctx context.Context, archive, temp *paths.Path) error {
	// Unzip the Debian image
	feedback.Print(i18n.Tr("Unzipping Debian image"))
	tmpZipFile, err := archive.Open()
	if err != nil {
		return fmt.Errorf("could not open archive: %w", err)
	}
	defer tmpZipFile.Close()

	if err := extract.Archive(ctx, tmpZipFile, temp.String(), func(s string) string {
		feedback.Print(s)
		return s
	}); err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	return nil
}

// SetTempDir returns a temporary directory inside the user's cache directory (default).
// The tempDir parameter is used to change the download/extraction directory.
// The caller is responsible for removing the directory when no longer needed.
func SetTempDir(prefix string, tempDir string) (*paths.Path, error) {
	cacheDir := paths.New(tempDir)

	if cacheDir == nil {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("could not get user's cache directory: %w", err)
		}

		cacheDir = paths.New(userCacheDir, "arduino-flasher-cli")
		_ = cacheDir.MkdirAll()
	}

	temp, err := paths.MkTempDir(cacheDir.String(), prefix)
	if err != nil {
		return nil, fmt.Errorf("could not create .cache directory: %w", err)
	}

	return temp, nil
}

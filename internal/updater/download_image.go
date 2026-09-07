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

func DownloadAndExtract(ctx context.Context, rel registry.Release, temp *paths.Path) error {
	tmpZip, err := DownloadImage(ctx, rel, temp)
	if err != nil {
		return fmt.Errorf("error downloading the image: %v", err)
	}

	if err := ExtractImage(ctx, tmpZip, temp); err != nil {
		return fmt.Errorf("error extracting the image: %v", err)
	}
	return nil
}

// DownloadImage fetches a release that was already found in an index, and
// returns the archive holding it.
func DownloadImage(ctx context.Context, rel registry.Release, downloadPath *paths.Path) (*paths.Path, error) {
	client := registry.NewClient()

	var bar *progressbar.ProgressBar
	callback := func(current, total int64) {
		if bar == nil {
			bar = progressbar.DefaultBytes(
				total,
				i18n.Tr("Downloading image version %s", rel.Version),
			)
		}
		_ = bar.Set64(current)
	}
	tmpZip, err := client.DownloadFile(ctx, downloadPath, rel, callback)
	if err != nil {
		return nil, fmt.Errorf("could not download Debian image: %w", err)
	}
	return tmpZip, nil
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

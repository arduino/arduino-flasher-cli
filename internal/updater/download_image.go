// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"context"
	"fmt"

	"github.com/arduino/go-paths-helper"
	"github.com/codeclysm/extract/v4"
	"github.com/schollz/progressbar/v3"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
)

// DownloadConfirmCB is a function that is called when a Debian image is ready to be downloaded.
type DownloadConfirmCB func(target string) (bool, error)

// DownloadImageToStore downloads or reuses a verified archive in the image store.
// The caller must resolve rel from the online manifest before calling it.
func DownloadImageToStore(ctx context.Context, rel registry.Release) (*paths.Path, error) {
	imageStore, err := store.Default()
	if err != nil {
		return nil, fmt.Errorf("could not open the image store: %w", err)
	}
	archive, _, err := imageStore.Get(ctx, rel, true, downloadInto(rel))
	if err != nil {
		return nil, err
	}
	return paths.New(archive), nil
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
		return nil, fmt.Errorf("could not download the image: %w", err)
	}
	return tmpZip, nil
}

func ExtractImage(ctx context.Context, archive, temp *paths.Path) error {
	// Unzip the image
	feedback.Print(i18n.Tr("Unzipping image"))
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

// downloadAndExtract gets the archive of rel from the image store, or downloads
// it, then extracts it in temp. It returns the archive for Store.Discard.
func downloadAndExtract(ctx context.Context, imageStore *store.Store, rel registry.Release, temp *paths.Path, keep bool) (string, error) {
	archive, found, err := imageStore.Get(ctx, rel, keep, downloadInto(rel))
	if err != nil {
		return "", fmt.Errorf("error getting the image: %w", err)
	}
	if found {
		feedback.Print(i18n.Tr("Using stored image version %s", rel.Version))
	}
	if err := ExtractImage(ctx, paths.New(archive), temp); err != nil {
		return "", fmt.Errorf("error extracting the image: %v", err)
	}
	return archive, nil
}

// downloadInto returns a store download function that downloads rel into its directory.
func downloadInto(rel registry.Release) func(context.Context, string) error {
	return func(ctx context.Context, dir string) error {
		_, err := DownloadImage(ctx, rel, paths.New(dir))
		return err
	}
}

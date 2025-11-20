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

package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/arduino/go-paths-helper"
	"github.com/codeclysm/extract/v4"
	"github.com/schollz/progressbar/v3"

	"github.com/arduino/arduino-flasher-cli/feedback"
	"github.com/arduino/arduino-flasher-cli/i18n"
)

type Manifest struct {
	Latest   Release   `json:"latest"`
	Releases []Release `json:"releases"`
}

type Release struct {
	Version string `json:"version"`
	Url     string `json:"url"`
	Sha256  string `json:"sha256"`
}

// DownloadConfirmCB is a function that is called when a Debian image is ready to be downloaded.
type DownloadConfirmCB func(target string) (bool, error)

func DownloadAndExtract(ctx context.Context, client *Client, targetVersion string, upgradeConfirmCb DownloadConfirmCB, forceYes bool, temp *paths.Path) (*paths.Path, string, error) {
	tmpZip, version, err := DownloadImage(ctx, client, targetVersion, upgradeConfirmCb, forceYes, temp)
	if err != nil {
		return nil, "", fmt.Errorf("error downloading the image: %v", err)
	}

	// Download not confirmed
	if tmpZip == nil {
		return nil, "", nil
	}

	err = ExtractImage(ctx, tmpZip, tmpZip.Parent())
	if err != nil {
		return nil, "", fmt.Errorf("error extracting the image: %v", err)
	}

	imagePath := tmpZip.Parent().Join("arduino-unoq-debian-image-" + version)
	if targetVersion == "latest" {
		version += "(latest)"
	}
	return imagePath, version, nil
}

func DownloadImage(ctx context.Context, client *Client, targetVersion string, upgradeConfirmCb DownloadConfirmCB, forceYes bool, downloadPath *paths.Path) (*paths.Path, string, error) {
	var err error

	feedback.Print(i18n.Tr("Checking for Debian image releases"))
	manifest, err := client.GetInfoManifest(ctx)
	if err != nil {
		return nil, "", err
	}

	var rel *Release
	if targetVersion == "latest" || targetVersion == manifest.Latest.Version {
		rel = &manifest.Latest
	} else {
		for _, r := range manifest.Releases {
			if targetVersion == r.Version {
				rel = &r
				break
			}
		}
	}

	if rel == nil {
		return nil, "", fmt.Errorf("could not find Debian image %s", targetVersion)
	}

	if !forceYes {
		res, err := upgradeConfirmCb(rel.Version)
		if err != nil {
			return nil, "", err
		}
		if !res {
			feedback.Print(i18n.Tr("Download not confirmed by user, exiting"))
			return nil, "", nil
		}
	}

	download, size, err := client.FetchZip(ctx, rel.Url)
	if err != nil {
		return nil, "", fmt.Errorf("could not fetch Debian image: %w", err)
	}
	defer download.Close()

	tmpZip := downloadPath.Join("arduino-unoq-debian-image-" + rel.Version + ".tar.zst")
	tmpZipFile, err := tmpZip.Create()
	if err != nil {
		return nil, "", err
	}
	defer tmpZipFile.Close()

	// Download and keep track of the progress
	bar := progressbar.DefaultBytes(
		size,
		i18n.Tr("Downloading Debian image version %s", rel.Version),
	)
	checksum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(checksum, tmpZipFile, bar), download); err != nil {
		return nil, "", err
	}

	// Check the hash
	if sha256Byte, err := hex.DecodeString(rel.Sha256); err != nil {
		return nil, "", fmt.Errorf("could not convert sha256 from hex to bytes: %w", err)
	} else if s := checksum.Sum(nil); !bytes.Equal(s, sha256Byte) {
		return nil, "", fmt.Errorf("bad hash: %x (expected %x)", s, sha256Byte)
	}

	return tmpZip, rel.Version, nil
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

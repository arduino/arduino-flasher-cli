package updater

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"

	"github.com/arduino/go-paths-helper"
	"github.com/bcmi-labs/orchestrator/cmd/feedback"
	"github.com/codeclysm/extract/v4"
)

// TODO: add more fields to download other image versions
type Manifest struct {
	Latest struct {
		Version string `json:"version"`
		Url     string `json:"url"`
		Md5sum  string `json:"md5sum"`
	} `json:"latest"`
}

// DownloadConfirmCB is a function that is called when a Debian image is ready to be downloaded.
type DownloadConfirmCB func(target string) (bool, error)

func DownloadImage(client *Client, targetVersion string, upgradeConfirmCb DownloadConfirmCB, forceYes bool) (string, string, error) {
	var err error

	slog.Info("Checking for Debian image releases")
	manifest, err := client.GetInfoManifest()
	if err != nil {
		return "", "", err
	}

	if targetVersion == "latest" {
		targetVersion = manifest.Latest.Version
	}

	if !forceYes {
		res, err := upgradeConfirmCb(targetVersion)
		if err != nil {
			return "", "", err
		}
		if !res {
			slog.Info("Download not confirmed by user, exiting")
			return "", "", nil
		}
	}

	// Download the Debian image
	var download io.ReadCloser
	if targetVersion == manifest.Latest.Version {
		slog.Info("Downloading Debian image", "version", manifest.Latest.Version)
		download, err = client.FetchZip(manifest.Latest.Url)
		if err != nil {
			return "", "", fmt.Errorf("could not fetch Debian image: %w", err)
		}
	} else {
		// TODO: check the json for the specific version and download it
		return "", "", nil
	}
	defer download.Close()

	// Download the zip
	temp, err := paths.MkTempDir("", "flasher-updater-")
	if err != nil {
		return "", "", fmt.Errorf("could not create temporary download directory: %w", err)
	}
	tmpZip := temp.Join("update.tar.xz")
	defer func() {
		if err := tmpZip.Remove(); err != nil {
			slog.Warn("Could not remove temp zip", "zip", tmpZip.String(), "error", err)
		}
	}()

	tmpZipFile, err := tmpZip.Create()
	if err != nil {
		return "", "", err
	}
	defer tmpZipFile.Close()

	md5 := md5.New()
	if _, err := io.Copy(io.MultiWriter(md5, tmpZipFile), download); err != nil {
		return "", "", err
	}
	tmpZipFile.Close()

	// Check the hash
	if md5Byte, err := hex.DecodeString(manifest.Latest.Md5sum); err != nil {
		return "", "", fmt.Errorf("could not convert md5 from hex to bytes: %w", err)
	} else if s := md5.Sum(nil); !bytes.Equal(s, md5Byte) {
		return "", "", fmt.Errorf("bad hash: %x (expected %x)", s, md5Byte)
	}

	// Unzip the Debian image
	slog.Info("Unzipping Debian image", "tmpDir", temp)
	tmpZipFile, err = tmpZip.Open()
	if err != nil {
		return "", "", fmt.Errorf("could not open archive for unzip: %w", err)
	}
	defer tmpZipFile.Close()

	if err := extract.Archive(context.Background(), tmpZipFile, temp.String(), func(s string) string {
		feedback.Print(s)
		return s
	}); err != nil {
		return "", "", fmt.Errorf("extracting archive: %w", err)
	}

	slog.Info("Download of Debian image completed", "path", temp)

	return temp.String(), targetVersion, nil
}

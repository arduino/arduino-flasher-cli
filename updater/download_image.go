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

// PassThru wraps an existing io.Reader.
//
// It simply forwards the Read() call, while displaying
// the results from individual calls to it.
type PassThru struct {
	io.Reader
	total      float64 // Total # of bytes transferred
	length     int64   // Expected length
	progress   float64
	progressCB func(f float64)
}

// Read 'overrides' the underlying io.Reader's Read method.
// This is the one that will be called by io.Copy(). We simply
// use it to keep track of the progress and then forward the call.
func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	pt.total += float64(n)
	percentage := pt.total / float64(pt.length) * float64(100)
	if percentage-pt.progress > 1 {
		pt.progressCB(percentage)
		pt.progress = percentage
	}

	return n, err
}

func DownloadAndExtract(client *Client, targetVersion string, upgradeConfirmCb DownloadConfirmCB, forceYes bool) (*paths.Path, error) {
	tmpZip, version, err := DownloadImage(client, targetVersion, upgradeConfirmCb, forceYes)
	if err != nil {
		return nil, fmt.Errorf("error downloading the image: %v", err)
	}

	err = ExtractImage(tmpZip, tmpZip.Parent())
	if err != nil {
		return nil, fmt.Errorf("error extracting the image: %v", err)
	}

	imagePath := tmpZip.Parent().Join("arduino-unoq-debian-image-" + version)
	return imagePath, nil
}

func DownloadImage(client *Client, targetVersion string, upgradeConfirmCb DownloadConfirmCB, forceYes bool) (*paths.Path, string, error) {
	var err error

	slog.Info("Checking for Debian image releases")
	manifest, err := client.GetInfoManifest()
	if err != nil {
		return nil, "", err
	}

	if targetVersion == "latest" {
		targetVersion = manifest.Latest.Version
	}

	if !forceYes {
		res, err := upgradeConfirmCb(targetVersion)
		if err != nil {
			return nil, "", err
		}
		if !res {
			slog.Info("Download not confirmed by user, exiting")
			return nil, "", nil
		}
	}

	// Download the Debian image
	var download io.ReadCloser
	var size int64
	if targetVersion == manifest.Latest.Version {
		slog.Info("Downloading Debian image", "version", manifest.Latest.Version)
		download, size, err = client.FetchZip(manifest.Latest.Url)
		if err != nil {
			return nil, "", fmt.Errorf("could not fetch Debian image: %w", err)
		}
	} else {
		// TODO: check the json for the specific version and download it
		return nil, "", nil
	}
	defer download.Close()

	// Download the zip
	temp, err := paths.MkTempDir("", "flasher-updater-")
	if err != nil {
		return nil, "", fmt.Errorf("could not create temporary download directory: %w", err)
	}

	tmpZip := temp.Join("update.tar.zst")
	tmpZipFile, err := tmpZip.Create()
	if err != nil {
		return nil, "", err
	}
	defer tmpZipFile.Close()

	// Download and keep track of the progress
	src := &PassThru{Reader: download, length: size, progressCB: func(f float64) { feedback.Printf("Download progress: %.2f %%", f) }}
	md5 := md5.New()
	if _, err := io.Copy(io.MultiWriter(md5, tmpZipFile), src); err != nil {
		return nil, "", err
	}

	// Check the hash
	if md5Byte, err := hex.DecodeString(manifest.Latest.Md5sum); err != nil {
		return nil, "", fmt.Errorf("could not convert md5 from hex to bytes: %w", err)
	} else if s := md5.Sum(nil); !bytes.Equal(s, md5Byte) {
		return nil, "", fmt.Errorf("bad hash: %x (expected %x)", s, md5Byte)
	}

	slog.Info("Download of Debian image completed", "path", temp)

	return tmpZip, targetVersion, nil
}

func ExtractImage(archive, temp *paths.Path) error {
	// Unzip the Debian image
	slog.Info("Unzipping Debian image", "tmpDir", temp)
	tmpZipFile, err := archive.Open()
	if err != nil {
		return fmt.Errorf("could not open archive: %w", err)
	}
	defer tmpZipFile.Close()

	if err := extract.Archive(context.Background(), tmpZipFile, temp.String(), func(s string) string {
		feedback.Print(s)
		return s
	}); err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	return nil
}

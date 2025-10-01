package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/arduino/go-paths-helper"
	"github.com/bcmi-labs/orchestrator/cmd/feedback"
	"github.com/bcmi-labs/orchestrator/cmd/i18n"
	"github.com/codeclysm/extract/v4"
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

	// Download not confirmed
	if tmpZip == nil {
		return nil, nil
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

	feedback.Print(i18n.Tr("Checking for Debian image releases"))
	manifest, err := client.GetInfoManifest()
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

	feedback.Print(i18n.Tr("Downloading Debian image version %s", rel.Version))
	download, size, err := client.FetchZip(rel.Url)
	if err != nil {
		return nil, "", fmt.Errorf("could not fetch Debian image: %w", err)
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
	checksum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(checksum, tmpZipFile), src); err != nil {
		return nil, "", err
	}

	// Check the hash
	if sha256Byte, err := hex.DecodeString(rel.Sha256); err != nil {
		return nil, "", fmt.Errorf("could not convert sha256 from hex to bytes: %w", err)
	} else if s := checksum.Sum(nil); !bytes.Equal(s, sha256Byte) {
		return nil, "", fmt.Errorf("bad hash: %x (expected %x)", s, sha256Byte)
	}

	feedback.Print(i18n.Tr("Download of Debian image completed"))

	return tmpZip, rel.Version, nil
}

func ExtractImage(archive, temp *paths.Path) error {
	// Unzip the Debian image
	feedback.Print(i18n.Tr("Unzipping Debian image"))
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

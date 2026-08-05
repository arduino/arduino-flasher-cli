// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/arduino/go-paths-helper"
	"github.com/shirou/gopsutil/v4/disk"
	"go.bug.st/downloader/v3"
	"go.bug.st/f"

	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
)

var baseURL = f.Must(url.Parse("https://downloads.arduino.cc"))

const (
	pathDebianRelease = "debian-im/Stable"
	pathUbuntuRelease = "ubuntu-im/custom-image/Stable"
	UnoQ              = "UNO Q"
	VentunoQ          = "VENTUNO Q"
	Debian            = "debian"
	Ubuntu            = "ubuntu"
)

type Manifest struct {
	Latest   Release   `json:"latest"`
	Releases []Release `json:"releases"`
}

type Release struct {
	Version  string `json:"version"`
	Url      string `json:"url"`
	Sha256   string `json:"sha256"`
	FileName string `json:"-"`
	Board    string `json:"board,omitempty"`
	OS       string `json:"os,omitempty"`
}

// Client holds the base URL, command name, allows custom HTTP client, and optional headers.
type Client struct {
	HTTPClient *http.Client
	Headers    map[string]string // Optional headers to add to each request
}

// Option is a functional option for configuring Client.
type Option func(*Client)

// WithHeaders sets custom headers for the Client.
func WithHeaders(headers map[string]string) Option {
	return func(c *Client) {
		c.Headers = headers
	}
}

// WithHTTPClient sets a custom HTTP client for the Client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.HTTPClient = client
	}
}

// NewClient creates a new Client with optional configuration.
func NewClient(opts ...Option) *Client {
	c := &Client{
		HTTPClient: http.DefaultClient,
		Headers:    nil,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// addHeaders adds custom headers to the request if present.
func (c *Client) addHeaders(req *http.Request) {
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
}

// GetInfoManifest fetches and decodes the images info.json for the given OS.
func (c *Client) GetInfoManifest(ctx context.Context, os string) (Manifest, error) {
	var releasePath string
	switch os {
	case Debian:
		releasePath = pathDebianRelease
	case Ubuntu:
		releasePath = pathUbuntuRelease
	default:
		return Manifest{}, fmt.Errorf("unsupported OS: %s", os)
	}
	manifestURL := baseURL.JoinPath(releasePath, "info.json").String()
	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("failed to create request: %w", err)
	}
	c.addHeaders(req)
	// #nosec G107 -- manifestURL is constructed from trusted config and parameters
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Manifest{}, fmt.Errorf("failed to GET manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("bad http status from %s: %v", manifestURL, resp.Status)
	}

	var res Manifest
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	if sha256Byte, err := hex.DecodeString(res.Latest.Sha256); err != nil {
		return Manifest{}, fmt.Errorf("could not convert sha256 from hex to bytes: %w", err)
	} else if len(sha256Byte) != sha256.Size {
		return Manifest{}, fmt.Errorf("bad sha256sum in manifest: got %d bytes", len(sha256Byte))
	}

	getFileName := func(rel Release) (string, error) {
		url, err := url.Parse(rel.Url)
		if err != nil {
			return "", fmt.Errorf("invalid URL in manifest for release %s: %w", rel.Version, err)
		}
		return path.Base(url.Path), nil
	}
	if res.Latest.FileName, err = getFileName(res.Latest); err != nil {
		return Manifest{}, err
	}
	for i := range res.Releases {
		if name, err := getFileName(res.Releases[i]); err != nil {
			return Manifest{}, err
		} else {
			res.Releases[i].FileName = name
		}
	}

	return res, nil
}

func (c *Client) GetReleaseByVersion(ctx context.Context, version string, boardType string, os string) (Release, error) {
	manifest, err := c.GetInfoManifest(ctx, os)
	if err != nil {
		return Release{}, err
	}

	if version == "latest" || version == manifest.Latest.Version {
		if manifest.Latest.Board == boardType {
			return manifest.Latest, nil
		}
	} else {
		for _, r := range manifest.Releases {
			if version == r.Version {
				if r.Board == boardType {
					return r, nil
				}
			}
		}
	}

	return Release{}, fmt.Errorf("could not find %s image %s available for board type %s", os, version, boardType)
}

type downloadCallback func(current, total int64)

// DownloadFile downloads a file from a URL into the specified path. An optional config and options may be passed (or nil to use the defaults).
// A DownloadProgressCB callback function must be passed to monitor download progress.
// If a not empty queryParameter is passed, it is appended to the URL for analysis purposes.
func (c *Client) DownloadFile(ctx context.Context, basePath *paths.Path, rel Release, cb downloadCallback) (*paths.Path, error) {
	f.Assert(basePath.IsDir(), "path must be a directory")

	// Check if there is enough free disk space before downloading and extracting an image
	dk, err := disk.Usage(basePath.String())
	if err != nil {
		return nil, err
	}

	filePath := basePath.Join(rel.FileName)
	d, err := downloader.DownloadWithConfigAndContext(ctx, filePath.String(), rel.Url, downloader.Config{
		HttpClient:   *c.HTTPClient,
		ExtraHeaders: maps.Clone(c.Headers),
		AcceptFunc: func(head *http.Response) error {
			if head.ContentLength > int64(dk.Free) { // nolint: gosec
				return fmt.Errorf("not enough disk space: need %d bytes, have %d bytes", head.ContentLength, dk.Free)
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}

	err = d.RunAndPoll(func(downloaded int64) {
		cb(downloaded, d.Size())
	}, 250*time.Millisecond)
	if err != nil {
		return nil, err
	}

	// The URL is not reachable for some reason
	if d.Resp.StatusCode >= 400 && d.Resp.StatusCode <= 599 {
		msg := i18n.Tr("Server responded with: %s", d.Resp.Status)
		return nil, fmt.Errorf("%s", msg)
	}

	// Check the hash
	checksum := sha256.New()
	tmpZipFile, err := filePath.Open()
	if err != nil {
		return nil, fmt.Errorf("could not open archive: %w", err)
	}
	defer tmpZipFile.Close()

	_, err = io.Copy(checksum, tmpZipFile)
	if err != nil {
		return nil, err
	}
	if sha256Byte, err := hex.DecodeString(rel.Sha256); err != nil {
		return nil, fmt.Errorf("could not convert sha256 from hex to bytes: %w", err)
	} else if s := checksum.Sum(nil); !bytes.Equal(s, sha256Byte) {
		return nil, fmt.Errorf("bad hash: %x (expected %x)", s, sha256Byte)
	}

	return filePath, nil
}

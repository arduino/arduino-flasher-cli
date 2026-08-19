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

// indexPaths is where each OS publishes its images. Each OS has one index,
// holding the releases of every board it is built for.
var indexPaths = []struct {
	Os   string
	Path string
}{
	{"debian", "debian-im/Stable"},
	{"ubuntu", "ubuntu-im/custom-image/Stable"},
}

// Indexes returns every OS an index is known for.
func Indexes() []string {
	oses := make([]string, 0, len(indexPaths))
	for _, i := range indexPaths {
		oses = append(oses, i.Os)
	}
	return oses
}

func indexPath(os string) (string, error) {
	for _, i := range indexPaths {
		if i.Os == os {
			return i.Path, nil
		}
	}
	return "", fmt.Errorf("no image index is published for %s yet", os)
}

type Manifest struct {
	Latest   Release   `json:"latest"`
	Releases []Release `json:"releases"`
}

type Release struct {
	Version string `json:"version"`
	Url     string `json:"url"`
	Sha256  string `json:"sha256"`
	// Board the release is built for: an index holds several. It is the label of
	// the board, which is how an index names it, not its id.
	Board string `json:"board,omitempty"`
	// Distro is the full distribution name ("Debian GNU/Linux 13 (trixie)"), for
	// display. Use the OS name to pick an index.
	Distro   string `json:"os,omitempty"`
	FileName string `json:"-"`
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

// GetInfoManifest fetches and decodes the info.json of the given OS's index.
func (c *Client) GetInfoManifest(ctx context.Context, os string) (Manifest, error) {
	indexDir, err := indexPath(os)
	if err != nil {
		return Manifest{}, err
	}
	manifestURL := baseURL.JoinPath(indexDir, "info.json").String()
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
	if resp.StatusCode == http.StatusNotFound {
		// Nothing published there yet, which is not the server being unreachable.
		return Manifest{}, fmt.Errorf("no %s index is published yet (%s)", os, manifestURL)
	}
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

// GetReleaseByVersion finds a release in the given OS's index. An empty board
// matches any, no version means the most recent one for that board.
func (c *Client) GetReleaseByVersion(ctx context.Context, version, os, boardID string) (Release, error) {
	manifest, err := c.GetInfoManifest(ctx, os)
	if err != nil {
		return Release{}, err
	}

	// An index names a board by its label. An id that is not one of ours is left
	// as it is: it matches nothing, which is what an unknown board should do.
	board := boardID
	if b, ok := BoardByID(boardID); ok {
		board = b.Label
	}
	matches := func(r Release) bool {
		return board == "" || r.Board == board
	}

	if version == "" {
		if matches(manifest.Latest) {
			return manifest.Latest, nil
		}
		// The overall latest is another board's. Oldest first, so the last
		// match is this board's most recent.
		for i := len(manifest.Releases) - 1; i >= 0; i-- {
			if matches(manifest.Releases[i]) {
				return manifest.Releases[i], nil
			}
		}
		return Release{}, fmt.Errorf("no %s image is published for the %s", os, board)
	}

	if version == manifest.Latest.Version && matches(manifest.Latest) {
		return manifest.Latest, nil
	}
	for _, r := range manifest.Releases {
		if version == r.Version && matches(r) {
			return r, nil
		}
	}

	return Release{}, fmt.Errorf("could not find %s image %s for the %s", os, version, board)
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

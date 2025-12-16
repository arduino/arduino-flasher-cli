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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
	"github.com/arduino/go-paths-helper"
	"go.bug.st/downloader/v2"
	"go.bug.st/f"
)

var baseURL = f.Must(url.Parse("https://downloads.arduino.cc"))

const pathRelease = "debian-im/Stable"

// Client holds the base URL, command name, allows custom HTTP client, and optional headers.
type Client struct {
	HTTPClient HTTPDoer
	Headers    map[string]string // Optional headers to add to each request
}

// HTTPDoer is an interface for http.Client or mocks.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
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
func WithHTTPClient(client HTTPDoer) Option {
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

// GetInfoManifest fetches and decodes the Debian images info.json.
func (c *Client) GetInfoManifest(ctx context.Context) (Manifest, error) {
	manifestURL := baseURL.JoinPath(pathRelease, "info.json").String()
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
	return res, nil
}

// FetchZip fetches the Debian image archive.
func (c *Client) FetchZip(ctx context.Context, zipURL string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", zipURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	c.addHeaders(req)
	// #nosec G107 -- zipURL is constructed from trusted config and parameters
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to GET zip: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("bad http status from %s: %v", zipURL, resp.Status)
	}
	return resp.Body, resp.ContentLength, nil
}

// DownloadFile downloads a file from a URL into the specified path. An optional config and options may be passed (or nil to use the defaults).
// A DownloadProgressCB callback function must be passed to monitor download progress.
// If a not empty queryParameter is passed, it is appended to the URL for analysis purposes.
func DownloadFile(ctx context.Context, path *paths.Path, URL string, label string, downloadCB flasher.DownloadProgressCB, config downloader.Config, options ...downloader.DownloadOptions) (returnedError error) {
	downloadCB.Start(URL, label)
	defer func() {
		if returnedError == nil {
			downloadCB.End(true, "")
		} else {
			downloadCB.End(false, returnedError.Error())
		}
	}()

	d, err := downloader.DownloadWithConfigAndContext(ctx, path.String(), URL, config, options...)
	if err != nil {
		return err
	}

	err = d.RunAndPoll(func(downloaded int64) {
		downloadCB.Update(downloaded, d.Size())
	}, 250*time.Millisecond)
	if err != nil {
		return err
	}

	// The URL is not reachable for some reason
	if d.Resp.StatusCode >= 400 && d.Resp.StatusCode <= 599 {
		msg := i18n.Tr("Server responded with: %s", d.Resp.Status)
		return fmt.Errorf("%s", msg)
	}

	return nil
}

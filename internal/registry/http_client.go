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
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/arduino/go-paths-helper"
	"github.com/shirou/gopsutil/v4/disk"
	"go.bug.st/downloader/v3"
	"go.bug.st/f"

	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
)

// Client reads the indexes and downloads what they publish.
type Client struct {
	httpClient *http.Client
	headers    map[string]string
}

// NewClient returns a client for the published indexes, and for any the
// environment adds.
func NewClient() *Client {
	return &Client{
		httpClient: http.DefaultClient,
		headers:    additionalHeaders(),
	}
}

// Fetch reads every index, newest release first. It is the only call that hits
// the network.
//
// One index failing does not hide the rest: its error is returned alongside the
// releases that could be read, so a caller should look at the releases first and
// treat the error as a warning. No releases and no error means nothing is
// published yet.
func (c *Client) Fetch(ctx context.Context) (Releases, error) {
	var res Releases
	var errs []error
	for _, index := range indexURLs() {
		releases, err := c.load(ctx, index)
		if errors.Is(err, errNotPublished) {
			continue
		}
		if err != nil {
			// Added here so load has only to say what went wrong.
			errs = append(errs, fmt.Errorf("could not read the image index at %s: %w", index, err))
			continue
		}
		res = append(res, releases...)
	}

	// Each index is sorted already, but a snapshot spans all of them, and so
	// does a line of releases: which is the latest is only known once merged.
	res.sortNewestFirst()
	res.markLatest()

	return res, errors.Join(errs...)
}

// DownloadFile downloads the archive of a release into basePath, under the name
// the release carries, and checks it against the release's checksum. The
// callback is told how much has arrived, and of how much.
func (c *Client) DownloadFile(ctx context.Context, basePath *paths.Path, rel Release, cb downloadCallback) (*paths.Path, error) {
	f.Assert(basePath.IsDir(), "path must be a directory")

	// Check if there is enough free disk space before downloading and extracting an image
	dk, err := disk.Usage(basePath.String())
	if err != nil {
		return nil, err
	}

	filePath := basePath.Join(rel.FileName)
	d, err := downloader.DownloadWithConfigAndContext(ctx, filePath.String(), rel.Url, downloader.Config{
		HttpClient:   *c.httpClient,
		ExtraHeaders: maps.Clone(c.headers),
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

// indexes is where images are published. An index holds the releases of any
// number of boards and distributions, and which of them a release is for is the
// release's own business: where it was fetched from says nothing. Nothing
// outside this file picks one either, they are fetched as a set.
var indexes = []*url.URL{
	f.Must(url.Parse("https://downloads.arduino.cc/debian-im/Stable/info.json")),
	f.Must(url.Parse("https://downloads.arduino.cc/ubuntu-im/custom-image/Stable/info.json")),
}

// Indexes beyond the published ones, separated by commas or newlines, for
// testing one before it is published. Alongside and not instead: what is
// published stays visible.
const additionalURLsEnv = "ARDUINO_FLASHER_ADDITIONAL_URLS"

// indexURLs is every index to read.
func indexURLs() []*url.URL {
	out := slices.Clone(indexes)
	for _, field := range strings.FieldsFunc(os.Getenv(additionalURLsEnv), func(r rune) bool {
		return r == ',' || r == '\n'
	}) {
		if field = strings.TrimSpace(field); field == "" {
			continue
		}
		// Not checked beyond parsing: one that cannot be read is reported by
		// [Client.Fetch] like any other, which is louder than dropping it here.
		if u, err := url.Parse(field); err == nil {
			out = append(out, u)
		}
	}
	return out
}

// Headers added to every index request and to the downloads they point at, one
// "Name: value" per line. Newlines rather than commas: a header value can hold
// one. Whatever a host asks to be let in with is its own business, so this
// flasher carries no credential of its own.
const additionalHeadersEnv = "ARDUINO_FLASHER_ADDITIONAL_HEADERS"

// additionalHeaders are the headers the environment asks for. A line with no
// name is skipped rather than sent empty.
func additionalHeaders() map[string]string {
	var out map[string]string
	for _, line := range strings.Split(os.Getenv(additionalHeadersEnv), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if name = strings.TrimSpace(name); !ok || name == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[name] = strings.TrimSpace(value)
	}
	return out
}

// errNotPublished is an index that is not there yet, which is ordinary rather
// than a failure, so [Client.Fetch] passes over it in silence.
var errNotPublished = errors.New("not published yet")

// addHeaders adds custom headers to the request if present.
func (c *Client) addHeaders(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

// load fetches and decodes a single index. What one holds is [index]; what it
// becomes is [index.parse].
func (c *Client) load(ctx context.Context, u *url.URL) (Releases, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	c.addHeaders(req)
	// #nosec G107 -- the index URL is a constant, not user input
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// Nothing there, as opposed to the server being unreachable.
		return nil, errNotPublished
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var idx index
	if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return idx.parse(), nil
}

type downloadCallback func(current, total int64)

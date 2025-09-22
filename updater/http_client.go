package updater

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Client holds the base URL, command name, allows custom HTTP client, and optional headers.
type Client struct {
	BaseURL    *url.URL
	CmdName    string
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
func NewClient(baseURL *url.URL, cmdName string, opts ...Option) *Client {
	c := &Client{
		BaseURL:    baseURL,
		CmdName:    cmdName,
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
func (c *Client) GetInfoManifest() (Manifest, error) {
	manifestURL := c.BaseURL.JoinPath(c.CmdName, "info.json").String()
	req, err := http.NewRequest("GET", manifestURL, nil)
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
	if md5Byte, err := hex.DecodeString(res.Latest.Md5sum); err != nil {
		return Manifest{}, fmt.Errorf("could not convert md5 from hex to bytes: %w", err)
	} else if len(md5Byte) != md5.Size {
		return Manifest{}, fmt.Errorf("bad md5sum in manifest: got %d bytes", len(md5Byte))
	}
	return res, nil
}

// FetchZip fetches the Debian image archive.
func (c *Client) FetchZip(zipURL string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequest("GET", zipURL, nil)
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

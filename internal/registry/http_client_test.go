// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.bug.st/f"

	"github.com/stretchr/testify/require"
)

func TestGetManifestBoardType(t *testing.T) {
	c := NewClient()
	_, err := c.GetInfoManifest(context.Background(), Ubuntu)
	require.Error(t, err)

	manifest, err := c.GetInfoManifest(context.Background(), Debian)
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Releases)
}

// newManifestServer spins up an HTTP server serving the Debian and Ubuntu
// info.json manifests. A nil body makes the corresponding endpoint return 404,
// simulating an unavailable manifest.
func newManifestServer(t *testing.T, debian, ubuntu *string) *httptest.Server {
	t.Helper()
	handler := func(body *string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			if body == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, *body)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/"+pathDebianRelease+"/info.json", handler(debian))
	mux.HandleFunc("/"+pathUbuntuRelease+"/info.json", handler(ubuntu))
	return httptest.NewServer(mux)
}

func TestGetReleaseByVersion(t *testing.T) {
	const validSha = "0000000000000000000000000000000000000000000000000000000000000000"

	debianManifest := `{
		"latest": {"version":"1.2.0","url":"https://downloads.arduino.cc/debian-1.2.0.zip","sha256":"` + validSha + `","board":"` + UnoQ + `"},
		"releases":[
			{"version":"1.0.0","url":"https://downloads.arduino.cc/debian-1.0.0.zip","sha256":"` + validSha + `","board":"` + UnoQ + `"},
			{"version":"1.1.0","url":"https://downloads.arduino.cc/debian-1.1.0.zip","sha256":"` + validSha + `","board":"` + UnoQ + `"}
		]
	}`
	ubuntuManifest := `{
		"latest": {"version":"2.0.0","url":"https://downloads.arduino.cc/ubuntu-2.0.0.zip","sha256":"` + validSha + `","board":"` + UnoQ + `"},
		"releases":[
			{"version":"1.1.0","url":"https://downloads.arduino.cc/ubuntu-1.1.0.zip","sha256":"` + validSha + `","board":"` + UnoQ + `"},
			{"version":"3.0.0","url":"https://downloads.arduino.cc/ubuntu-3.0.0.zip","sha256":"` + validSha + `","board":"` + UnoQ + `"}
		]
	}`

	tests := []struct {
		name        string
		debian      *string
		ubuntu      *string
		version     string
		boardType   string
		os          string
		wantErr     bool
		wantVersion string
		wantURLPart string
	}{
		{
			// version "latest" with no board and no OS: only the Debian manifest
			// is available, so the latest Debian release is returned.
			name:        "latest without board and os returns latest debian",
			debian:      &debianManifest,
			ubuntu:      nil,
			version:     "latest",
			wantVersion: "1.2.0",
			wantURLPart: "debian-1.2.0.zip",
		},
		{
			// A non-latest version present in both manifests and no OS specified
			// is ambiguous and must return an error.
			name:    "non-latest without os matching both manifests errors",
			debian:  &debianManifest,
			ubuntu:  &ubuntuManifest,
			version: "1.1.0",
			wantErr: true,
		},
		{
			// A non-latest version present only in the Debian manifest is returned.
			name:        "non-latest without os matching only debian",
			debian:      &debianManifest,
			ubuntu:      &ubuntuManifest,
			version:     "1.0.0",
			wantVersion: "1.0.0",
			wantURLPart: "debian-1.0.0.zip",
		},
		{
			// A non-latest version present only in the Ubuntu manifest is returned.
			name:        "non-latest without os matching only ubuntu",
			debian:      &debianManifest,
			ubuntu:      &ubuntuManifest,
			version:     "3.0.0",
			wantVersion: "3.0.0",
			wantURLPart: "ubuntu-3.0.0.zip",
		},
		{
			// When the OS is specified, only the corresponding manifest is checked,
			// so the ambiguous "1.1.0" resolves to the Debian release.
			name:        "os specified checks only that manifest",
			debian:      &debianManifest,
			ubuntu:      &ubuntuManifest,
			version:     "1.1.0",
			os:          Debian,
			wantVersion: "1.1.0",
			wantURLPart: "debian-1.1.0.zip",
		},
		{
			// When the OS is specified, a version present only in the other
			// manifest is not found.
			name:    "os specified ignores other manifest",
			debian:  &debianManifest,
			ubuntu:  &ubuntuManifest,
			version: "3.0.0",
			os:      Debian,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newManifestServer(t, tc.debian, tc.ubuntu)
			defer srv.Close()

			oldBaseURL := baseURL
			baseURL = f.Must(url.Parse(srv.URL))
			defer func() { baseURL = oldBaseURL }()

			c := NewClient()
			rel, err := c.GetReleaseByVersion(context.Background(), tc.version, tc.boardType, tc.os)
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, rel.Version)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantVersion, rel.Version)
			require.Contains(t, rel.Url, tc.wantURLPart)
		})
	}
}

// TestGetReleaseByVersionPropagatesGenuineErrors verifies that when no OS is
// specified, a genuine failure fetching a manifest (e.g. HTTP 500) is propagated
// to the caller instead of being masked as a generic "release not found".
func TestGetReleaseByVersionPropagatesGenuineErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/"+pathDebianRelease+"/info.json", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/"+pathUbuntuRelease+"/info.json", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldBaseURL := baseURL
	baseURL = f.Must(url.Parse(srv.URL))
	defer func() { baseURL = oldBaseURL }()

	c := NewClient()
	_, err := c.GetReleaseByVersion(context.Background(), "1.0.0", "", "")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrReleaseNotFound), "genuine errors must not be reported as ErrReleaseNotFound")
	// Both probes failed genuinely, so both underlying manifest URLs must appear.
	require.Contains(t, err.Error(), pathDebianRelease)
	require.Contains(t, err.Error(), pathUbuntuRelease)
}

// TestGetReleaseByVersionNotFoundIsSentinel verifies that when a version is not
// available in any manifest, the returned error wraps ErrReleaseNotFound.
func TestGetReleaseByVersionNotFoundIsSentinel(t *testing.T) {
	debianManifest := `{"latest":{"version":"1.2.0","url":"https://x/d.zip","sha256":"0000000000000000000000000000000000000000000000000000000000000000","board":"` + UnoQ + `"},"releases":[]}`
	srv := newManifestServer(t, &debianManifest, nil)
	defer srv.Close()

	oldBaseURL := baseURL
	baseURL = f.Must(url.Parse(srv.URL))
	defer func() { baseURL = oldBaseURL }()

	c := NewClient()
	_, err := c.GetReleaseByVersion(context.Background(), "9.9.9", "", "")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrReleaseNotFound))
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
)

type archiveTransport struct {
	contents []byte
	requests int
	err      error
}

func (s *archiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.requests++
	if s.err != nil {
		return nil, s.err
	}
	if req.URL.String() != "https://images.example/image.tar" {
		return nil, fmt.Errorf("unexpected request: %s", req.URL)
	}
	body := s.contents
	if req.Method == http.MethodHead {
		body = nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Length": []string{strconv.Itoa(len(s.contents))},
		},
		ContentLength: int64(len(s.contents)),
		Body:          io.NopCloser(bytes.NewReader(body)),
		Request:       req,
	}, nil
}

func setupImageDownload(t *testing.T) (*store.Store, registry.Release, *archiveTransport) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", t.TempDir())
	case "darwin":
		t.Setenv("HOME", t.TempDir())
	default:
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
	}
	t.Setenv("ARDUINO_FLASHER_ADDITIONAL_HEADERS", "")
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "image/payload.txt", Mode: 0o600, Size: int64(len("test payload"))}))
	_, err := w.Write([]byte("test payload"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	checksum := sha256.Sum256(archive.Bytes())
	rel := registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Url: "https://images.example/image.tar", FileName: "image.tar",
		Sha256: hex.EncodeToString(checksum[:]),
	}
	transport := &archiveTransport{contents: archive.Bytes()}
	original := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() { http.DefaultClient = original })
	imageStore, err := store.Default()
	require.NoError(t, err)
	return imageStore, rel, transport
}

func seedImageStore(t *testing.T, imageStore *store.Store, rel registry.Release, contents []byte) string {
	t.Helper()
	archive, _, err := imageStore.Get(context.Background(), rel, true, func(_ context.Context, dir string) error {
		return os.WriteFile(filepath.Join(dir, rel.FileName), contents, 0o600)
	})
	require.NoError(t, err)
	return archive
}

func requireExtractedPayload(t *testing.T, dir *paths.Path) {
	t.Helper()
	data, err := os.ReadFile(dir.Join("image", "payload.txt").String())
	require.NoError(t, err)
	require.Equal(t, "test payload", string(data))
}

func TestDownloadAndExtractReusesStore(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep=%t", keep), func(t *testing.T) {
			imageStore, rel, transport := setupImageDownload(t)
			archive := seedImageStore(t, imageStore, rel, transport.contents)
			scratch := paths.New(t.TempDir())
			_, err := downloadAndExtract(context.Background(), imageStore, rel, scratch, keep)
			require.NoError(t, err)
			requireExtractedPayload(t, scratch)
			require.Zero(t, transport.requests, "reusing an archive must not contact its server")
			require.NoDirExists(t, filepath.Join(filepath.Dir(archive), "image"), "extraction must stay outside the image store")
			require.NoError(t, scratch.RemoveAll())
			_, err = imageStore.Lookup(context.Background(), rel)
			require.NoError(t, err, "cleaning scratch space must not delete stored archives")
		})
	}
}

func TestDownloadAndExtractKeepsOnlyWhenRequested(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep=%t", keep), func(t *testing.T) {
			imageStore, rel, transport := setupImageDownload(t)
			scratch := paths.New(t.TempDir())
			archive, err := downloadAndExtract(context.Background(), imageStore, rel, scratch, keep)
			require.NoError(t, err)
			requireExtractedPayload(t, scratch)
			require.NotZero(t, transport.requests)
			require.NoFileExists(t, scratch.Join(rel.FileName).String(), "archives are never in the extraction directory")
			_, err = imageStore.Lookup(context.Background(), rel)
			if keep {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, store.ErrMiss)
				require.Equal(t, filepath.Join(imageStore.Dir(), ".partial", rel.FileName), archive)
			}
			imageStore.Discard(archive)
			if keep {
				require.FileExists(t, archive, "discarding leaves kept archives")
			} else {
				require.NoFileExists(t, archive)
			}
		})
	}
}

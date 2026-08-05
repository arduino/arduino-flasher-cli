// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"context"
	"testing"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

func TestDetectBoardAndSetOsAcceptsLocalImage(t *testing.T) {
	validBoards := []string{registry.UnoQ, registry.VentunoQ}
	validOSes := []string{registry.Debian, registry.Ubuntu}

	t.Run("compressed archive file", func(t *testing.T) {
		imagePath := paths.New(t.TempDir()).Join("my-image.tar.zst")
		require.NoError(t, imagePath.WriteFile([]byte("fake compressed image")))

		version, boardType, osName, err := DetectBoardAndSetOs(context.Background(), imagePath.String())

		require.NoErrorf(t, err, "a local image path %q must be accepted, not rejected as an invalid version", imagePath)
		require.NotEmpty(t, version, "a local image must resolve to a non-empty version label")
		require.Contains(t, validBoards, boardType)
		require.Contains(t, validOSes, osName)
	})

	t.Run("extracted image folder", func(t *testing.T) {
		imageDir := paths.New(t.TempDir()).Join("arduino-unoq-debian-image-20250915-173")
		require.NoError(t, imageDir.MkdirAll())

		version, boardType, osName, err := DetectBoardAndSetOs(context.Background(), imageDir.String())

		require.NoErrorf(t, err, "a local image folder %q must be accepted, not rejected as an invalid version", imageDir)
		require.NotEmpty(t, version, "a local image must resolve to a non-empty version label")
		require.Contains(t, validBoards, boardType)
		require.Contains(t, validOSes, osName)
	})
}

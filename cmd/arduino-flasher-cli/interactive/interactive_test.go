// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
)

func wizardRelease() registry.Release {
	sum := sha256.Sum256([]byte("test image"))
	return registry.Release{
		Board: registry.UnoQ.ID, OS: "debian", Version: "20250915-173",
		Sha256: hex.EncodeToString(sum[:]), FileName: "image.tar.zst",
		Url: "https://images.example/image.tar.zst", Latest: true,
	}
}

func wizardEntry(rel registry.Release) cache.Entry {
	return cache.Entry{FileName: rel.FileName}
}

func TestImageOptionsCacheLabels(t *testing.T) {
	newest := wizardRelease()
	older := newest
	older.Version, older.Latest, older.FileName = "20250914-172", false, "image-20250914-172.tar.zst"
	images := registry.Releases{newest, older}
	for _, cached := range []string{"none", "newest", "older"} {
		t.Run(cached, func(t *testing.T) {
			var inventory cache.Inventory
			labels := []string{newest.Version + " (latest)", older.Version}
			switch cached {
			case "newest":
				inventory.Images = []cache.Entry{wizardEntry(newest)}
				labels[0] += " (cached)"
			case "older":
				inventory.Images = []cache.Entry{wizardEntry(older)}
				labels[1] += " (cached)"
			}
			options := imageOptions(images, inventory)
			require.Len(t, options, len(images))
			for i, option := range options {
				require.Equal(t, labels[i], option.Key)
				require.Equal(t, images[i], option.Value, "labels must not change the release used for flashing")
			}
		})
	}
}

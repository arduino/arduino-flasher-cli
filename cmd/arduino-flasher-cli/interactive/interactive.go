// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package interactive

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/arduino/go-paths-helper"
	"github.com/dustin/go-humanize"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

const minRootSize uint64 = 9 * 1024 * 1024 * 1024 // 9GiB

var rootSizePresets = []huh.Option[string]{
	huh.NewOption("Auto-detect", ""),
	huh.NewOption("10 GB", "10GB"),
	huh.NewOption("12 GB", "12GB"),
	huh.NewOption("16 GB", "16GB"),
	huh.NewOption("20 GB", "20GB"),
	huh.NewOption("24 GB", "24GB"),
	huh.NewOption("Custom…", "custom"),
}

// Run starts the interactive wizard and performs the flash.
func Run(ctx context.Context) {
	client := registry.NewClient()

	var manifest registry.Manifest
	spinner := spinner.New().
		Title(i18n.Tr("Fetching available images...")).
		ActionWithErr(func(ctx context.Context) error {
			var err error
			manifest, err = client.GetInfoManifest(ctx)
			return err
		})
	if err := spinner.Run(); err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}

	versionOptions := make([]huh.Option[string], 0, len(manifest.Releases)+1)
	for i := len(manifest.Releases) - 1; i >= 0; i-- {
		r := manifest.Releases[i]
		label := r.Version
		if r.Version == manifest.Latest.Version {
			label += " (latest)"
		}
		versionOptions = append(versionOptions, huh.NewOption(label, r.Version))
	}

	var (
		selectedVersion string
		preserveUser    bool
		rootSizeStr     string // default "": auto-detect
		confirm         bool
	)

	// Step 1 — pick image
	imageForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select the image version to flash").
				Description("Use ↑/↓ to navigate, Enter to confirm").
				Options(versionOptions...).
				Value(&selectedVersion),
		),
	)
	if err := imageForm.RunWithContext(ctx); err != nil {
		return // user cancelled
	}

	// Step 2 — partition options
	partForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Preserve user partition?").
				Description("Keep existing user data on the board").
				Value(&preserveUser),
		),
	)
	if err := partForm.RunWithContext(ctx); err != nil {
		return
	}

	// Root size is mutually exclusive with preserve-user — only ask when user partition will be erased
	if !preserveUser {
		sizePickForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Root partition size").
					Description("Use ↑/↓ to navigate, Enter to confirm").
					Options(rootSizePresets...).
					Value(&rootSizeStr),
			),
		)
		if err := sizePickForm.RunWithContext(ctx); err != nil {
			return
		}

		// "Custom…" was chosen — ask for a free-form value
		if rootSizeStr == "custom" {
			customForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Enter root partition size").
						Description("e.g. 14GB, 15GiB — minimum 9GB").
						Placeholder("12GB").
						Validate(func(s string) error {
							s = strings.TrimSpace(s)
							if s == "" {
								return fmt.Errorf("value required")
							}
							size, err := humanize.ParseBytes(s)
							if err != nil {
								return fmt.Errorf("invalid size: %v", err)
							}
							if size < minRootSize {
								return fmt.Errorf("must be at least 9GB")
							}
							return nil
						}).
						Value(&rootSizeStr),
				),
			)
			if err := customForm.RunWithContext(ctx); err != nil {
				return
			}
		}
	}

	rootSize := uint64(0)
	if rootSizeStr != "" {
		rootSize, _ = humanize.ParseBytes(strings.TrimSpace(rootSizeStr))
	}

	// Step 3 — summary + confirm
	summary := buildSummary(selectedVersion, preserveUser, rootSize)
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Ready to flash").
				Description(summary).
				Affirmative("Flash now").
				Negative("Cancel").
				Value(&confirm),
		),
	)
	if err := confirmForm.RunWithContext(ctx); err != nil || !confirm {
		feedback.Print(i18n.Tr("Flash cancelled."))
		return
	}

	// Resolve image path (version string — Flash will download it)
	imagePath, _ := paths.New(selectedVersion).Abs()

	if err := updater.Flash(ctx, imagePath, selectedVersion, true, preserveUser, "", rootSize, nil); err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print(i18n.Tr("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper."))
}

func buildSummary(version string, preserveUser bool, rootSize uint64) string {
	userPartition := "will be erased"
	if preserveUser {
		userPartition = "preserved"
	}

	rootSizeStr := "auto-detect"
	if rootSize > 0 {
		rootSizeStr = humanize.Bytes(rootSize)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Image version:\t"+version)
	fmt.Fprintln(w, "User partition:\t"+userPartition)
	fmt.Fprintln(w, "Root size:\t"+rootSizeStr)
	w.Flush()

	buf.WriteString("\nWARNING: This will erase existing data on the board.")
	return buf.String()
}

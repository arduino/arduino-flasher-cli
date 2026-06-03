// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package interactive

import (
	"bytes"
	"context"
	"fmt"
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

const (
	giB         int64 = 1024 * 1024 * 1024
	rootSizeMin int64 = 9 * giB
	rootSizeMax int64 = 64 * giB
)

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
		rootSize        uint64 // 0 == auto-detect
		confirm         bool
	)

	// Step 1 — pick image
	imageForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.Tr("Select the image version to flash")).
				Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
				Options(versionOptions...).
				Value(&selectedVersion),
		),
	)
	if err := imageForm.RunWithContext(ctx); err != nil {
		return // user canceled
	}

	// Step 2 — partition options
	partForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Preserve user partition?")).
				Description(i18n.Tr("Keep existing user data on the board")).
				Value(&preserveUser),
		),
	)
	if err := partForm.RunWithContext(ctx); err != nil {
		return
	}

	// Root size is mutually exclusive with preserve-user — only ask when the user partition will be erased
	if !preserveUser {
		var rootSizeStr string
		changeRootSize := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Change root partition size?")).
				Description(i18n.Tr("By default, the root partition size is automatically determined,\nsplitting the available space roughly equally between the root and home partitions.")).
				Value(&confirm),
		))
		if err := changeRootSize.RunWithContext(ctx); err != nil || !confirm {
			feedback.Print(i18n.Tr("Using default root partition size (auto-detect)."))
			rootSize = 0
		} else {
			rootSizeForm := huh.NewForm(huh.NewGroup(
				huh.NewInput().
					Title(i18n.Tr("Root partition size")).
					Description(i18n.Tr("Insert a value in GB")).
					Value(&rootSizeStr).
					Validate(func(input string) error {
						if input == "" {
							return nil // allow empty input for auto-detect
						}
						sizeGB, err := humanize.ParseBytes(input + "GB")
						if err != nil {
							return fmt.Errorf("invalid size: %w", err)
						}
						if sizeGB < uint64(rootSizeMin) || sizeGB > uint64(rootSizeMax) {
							return fmt.Errorf("size must be between %d and %d GiB", rootSizeMin/giB, rootSizeMax/giB)
						}
						return nil
					}),
			))
			if err := rootSizeForm.RunWithContext(ctx); err != nil {
				return
			}

			if size, err := humanize.ParseBytes(rootSizeStr + "GB"); err != nil {
				feedback.Print(i18n.Tr("Flash canceled."))
				return
			} else {
				rootSize = size
			}
		}
	}

	// Step 3 — summary + confirm
	summary := buildSummary(selectedVersion, preserveUser, rootSize)
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Ready to flash")).
				Description(summary).
				Affirmative(i18n.Tr("Flash now")).
				Negative(i18n.Tr("Cancel")).
				Value(&confirm),
		),
	)
	if err := confirmForm.RunWithContext(ctx); err != nil || !confirm {
		feedback.Print(i18n.Tr("Flash canceled."))
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

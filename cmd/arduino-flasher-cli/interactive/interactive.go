// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package interactive

import (
	"bytes"
	"context"
	"errors"
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
)

// Run starts the interactive wizard and performs the flash.
func Run(ctx context.Context) {
	client := registry.NewClient()

	var manifest registry.Manifest
	sp := spinner.New().
		Title(i18n.Tr("Fetching available images...")).
		ActionWithErr(func(ctx context.Context) error {
			var err error
			manifest, err = client.GetInfoManifest(ctx)
			return err
		})
	if err := sp.Run(); err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}

	versionOptions := make([]huh.Option[string], 0, len(manifest.Releases))
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
		rootSizeStr     string
		confirm         bool
	)

	form := huh.NewForm(
		// Step 1 — pick image
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.Tr("Select the image version to flash")).
				Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
				Options(versionOptions...).
				Value(&selectedVersion),
		),

		// Step 2 — partition options
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Preserve user partition?")).
				Description(i18n.Tr("Keep existing user data on the board")).
				Affirmative(i18n.Tr("Yes")).
				Negative(i18n.Tr("No")).
				Value(&preserveUser),
		),

		// Step 3 — root size override (only when user partition will be erased)
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.Tr("Root partition size (optional)")).
				Description(i18n.Tr("Leave blank to split available space equally between root and home partitions.\nOtherwise, insert a value in GB (e.g. 16).")).
				Placeholder(i18n.Tr("e.g. 16")).
				Value(&rootSizeStr).
				Validate(func(input string) error {
					if input == "" {
						return nil
					}
					sizeGB, err := humanize.ParseBytes(input + "GB")
					if err != nil {
						return fmt.Errorf("invalid size: %w", err)
					}
					if sizeGB < uint64(rootSizeMin) {
						return fmt.Errorf("size must be more than %d GiB", rootSizeMin/giB)
					}
					return nil
				}),
		).WithHideFunc(func() bool { return preserveUser }),

		// Step 4 — summary + confirm (description is recomputed when any binding changes)
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Ready to flash")).
				DescriptionFunc(func() string {
					return buildSummary(selectedVersion, preserveUser, parseRootSize(rootSizeStr, preserveUser))
				}, []any{&selectedVersion, &preserveUser, &rootSizeStr}).
				Affirmative(i18n.Tr("Flash now")).
				Negative(i18n.Tr("Cancel")).
				Value(&confirm),
		),
	)

	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			feedback.Print(i18n.Tr("Flash canceled."))
			return
		}
		feedback.Fatal(i18n.Tr("error running interactive wizard: %v", err), feedback.ErrBadArgument)
	}
	if !confirm {
		feedback.Print(i18n.Tr("Flash canceled."))
		return
	}

	rootSize := parseRootSize(rootSizeStr, preserveUser)

	// Resolve image path (version string — Flash will download it)
	imagePath, _ := paths.New(selectedVersion).Abs()

	if err := updater.Flash(ctx, imagePath, selectedVersion, true, preserveUser, "", rootSize, nil); err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print(i18n.Tr("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper."))
}

// parseRootSize converts the user-supplied size string into bytes.
// Returns 0 (auto-detect) when the user partition is preserved, the user did
// not request a custom size, or the input is empty/unparseable (already
// validated by the form).
func parseRootSize(rootSizeStr string, preserveUser bool) uint64 {
	if preserveUser || rootSizeStr == "" {
		return 0
	}
	size, err := humanize.ParseBytes(rootSizeStr + "GB")
	if err != nil {
		return 0
	}
	return size
}

func buildSummary(version string, preserveUser bool, rootSize uint64) string {
	userPartition := i18n.Tr("will be erased")
	if preserveUser {
		userPartition = i18n.Tr("preserved")
	}

	rootSizeStr := i18n.Tr("auto-detect")
	if rootSize > 0 {
		rootSizeStr = humanize.Bytes(rootSize)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, i18n.Tr("Image version:")+"\t"+version)
	fmt.Fprintln(w, i18n.Tr("User partition:")+"\t"+userPartition)
	fmt.Fprintln(w, i18n.Tr("Root size:")+"\t"+rootSizeStr)
	_ = w.Flush()

	if !preserveUser {
		buf.WriteString("\n" + i18n.Tr("WARNING: This will erase existing data on the board."))
	}

	return buf.String()
}

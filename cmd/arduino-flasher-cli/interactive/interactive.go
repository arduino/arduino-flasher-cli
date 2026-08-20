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
	"strconv"
	"strings"
	"text/tabwriter"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/dustin/go-humanize"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
)

// Run starts the interactive wizard and performs the flash.
func Run(ctx context.Context) {
	client := registry.NewClient()

	// Only the UNO Q for now: a second board would need a step asking which one.
	board := registry.UnoQ

	var manifest registry.Manifest
	sp := spinner.New().
		Title(i18n.Tr("Fetching available images...")).
		ActionWithErr(func(ctx context.Context) error {
			var err error
			manifest, err = client.GetInfoManifest(ctx, board.DefaultOs)
			return err
		})
	if err := sp.Run(); err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}

	// The storage size is asked rather than read: reading it needs the programmer
	// that ships with the image, which is not downloaded yet. The sizes are
	// checked against the real GPT once it has been extracted.
	variantOptions := make([]huh.Option[registry.Variant], 0, len(board.Variants))
	for _, v := range board.Variants {
		variantOptions = append(variantOptions, huh.NewOption(v.Label, v))
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
		variant         registry.Variant
		selectedVersion string
		preserveUser    bool
		rootPctStr      string
		confirm         bool
	)

	form := huh.NewForm(
		// Step 1 — pick board variant
		huh.NewGroup(
			huh.NewSelect[registry.Variant]().
				Title(i18n.Tr("Select your %s", board.Label)).
				Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
				Options(variantOptions...).
				Value(&variant),
		),

		// Step 2 — pick image
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.Tr("Select the image version to flash")).
				Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
				Options(versionOptions...).
				Value(&selectedVersion),
		),

		// Step 3 — partition options
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Preserve user partition?")).
				Description(i18n.Tr("Keep existing user data on the board")).
				Affirmative(i18n.Tr("Yes")).
				Negative(i18n.Tr("No")).
				Value(&preserveUser),
		),

		// Step 4 — root/home split, expressed as a percentage of total
		// storage assigned to the root partition. The description is
		// re-evaluated on every keystroke (huh bindings) so the user sees
		// the resulting partition sizes live.
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.Tr("Root partition size (%% of total storage)")).
				DescriptionFunc(func() string {
					return buildSplitPreview(variant.Capacity, rootPctStr)
				}, []any{&variant, &rootPctStr}).
				Placeholder(i18n.Tr("e.g. 50 (leave blank for default)")).
				Value(&rootPctStr).
				Validate(func(input string) error {
					if input == "" {
						return nil
					}
					pct, err := parsePercentage(input)
					if err != nil {
						return err
					}
					if pct < 1 || pct > 95 {
						return fmt.Errorf("percentage must be between 1 and 95")
					}
					rootSize := rootSizeFromPct(variant.Capacity, pct)
					if rootSize < updater.MinRootSize {
						return fmt.Errorf("root partition must be at least %d GiB (try a higher percentage)", updater.MinRootSize/updater.GiB)
					}
					if rootSize+updater.SystemReservedBytes+updater.MinUserPartitionSize >= variant.Capacity {
						return fmt.Errorf("percentage too high: no space left for the home partition")
					}
					return nil
				}),
		).WithHideFunc(func() bool { return preserveUser }),

		// Step 5 — summary + confirm (description is recomputed when any binding changes)
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Ready to flash")).
				DescriptionFunc(func() string {
					return buildSummary(variant.Label, selectedVersion, preserveUser, parseRootSize(variant.Capacity, rootPctStr, preserveUser))
				}, []any{&variant, &selectedVersion, &preserveUser, &rootPctStr}).
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

	rootSize := parseRootSize(variant.Capacity, rootPctStr, preserveUser)

	opts := updater.FlashOptions{PreserveUser: preserveUser, RootSize: rootSize}
	if err := updater.DownloadAndFlash(ctx, board.ID, board.DefaultOs, selectedVersion, opts); err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print(i18n.Tr("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper."))
}

// parsePercentage parses a string like "50" or "50%" as an integer percentage.
func parsePercentage(input string) (uint64, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(input), "%"))
	pct, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid percentage: enter an integer like 50")
	}
	return pct, nil
}

// rootSizeFromPct returns the root-partition size in bytes for a given board
// storage and percentage.
func rootSizeFromPct(boardStorage, pct uint64) uint64 {
	return boardStorage * pct / 100
}

// parseRootSize converts the user-supplied percentage string into the root
// partition size in bytes (absolute), suitable to be passed to updater.Flash.
// Returns 0 (auto-detect) when the user partition is preserved, the user did
// not request a custom size, or the input is empty/unparseable (already
// validated by the form).
func parseRootSize(boardStorage uint64, rootPctStr string, preserveUser bool) uint64 {
	if preserveUser || rootPctStr == "" || boardStorage == 0 {
		return 0
	}
	pct, err := parsePercentage(rootPctStr)
	if err != nil {
		return 0
	}
	return rootSizeFromPct(boardStorage, pct)
}

// buildSplitPreview produces the live description shown under the percentage
// input field, e.g.:
//
//	Total storage: 16 GB
//	Root partition: 8.0 GiB (50%)
//	Home partition: ~6.0 GiB
func buildSplitPreview(boardStorage uint64, rootPctStr string) string {
	var buf bytes.Buffer
	toPadString := func() string {
		const padLines = 3
		n := bytes.Count(buf.Bytes(), []byte("\n"))
		buf.Write(bytes.Repeat([]byte("\n"), max(0, padLines-n)))
		return buf.String()
	}

	buf.WriteString(i18n.Tr("Total storage: %s", humanize.Bytes(boardStorage)) + "\n")

	if rootPctStr == "" {
		buf.WriteString(i18n.Tr("Leave blank to use the default split decided by the image."))
		return toPadString()
	}
	pct, err := parsePercentage(rootPctStr)
	if err != nil {
		buf.WriteString(i18n.Tr("Enter an integer percentage (1-95)."))
		return toPadString()
	}
	if pct < 1 || pct > 95 {
		buf.WriteString(i18n.Tr("Percentage must be between 1 and 95."))
		return toPadString()
	}

	rootSize := rootSizeFromPct(boardStorage, pct)
	var homeSize uint64
	if rootSize+updater.SystemReservedBytes < boardStorage {
		homeSize = boardStorage - updater.SystemReservedBytes - rootSize
	}
	fmt.Fprintf(&buf, "%s %s (%d%%)\n", i18n.Tr("Root partition:"), humanize.IBytes(rootSize), pct)
	fmt.Fprintf(&buf, "%s ~%s", i18n.Tr("Home partition:"), humanize.IBytes(homeSize))
	return toPadString()
}

func buildSummary(boardLabel, version string, preserveUser bool, rootSize uint64) string {
	userPartition := i18n.Tr("will be erased")
	if preserveUser {
		userPartition = i18n.Tr("preserved")
	}

	rootSizeStr := i18n.Tr("auto-detect")
	if rootSize > 0 {
		rootSizeStr = humanize.IBytes(rootSize)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, i18n.Tr("Board:")+"\t"+boardLabel)
	fmt.Fprintln(w, i18n.Tr("Image version:")+"\t"+version)
	fmt.Fprintln(w, i18n.Tr("User partition:")+"\t"+userPartition)
	fmt.Fprintln(w, i18n.Tr("Root size:")+"\t"+rootSizeStr)
	_ = w.Flush()

	if !preserveUser {
		buf.WriteString("\n" + i18n.Tr("WARNING: This will erase existing data on the board."))
	}

	return buf.String()
}

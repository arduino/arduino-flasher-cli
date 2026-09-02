// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package interactive

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
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
	images := fetchImages(ctx)

	boardIDs := slices.Sorted(maps.Keys(images))
	boardID := boardIDs[0]
	if len(boardIDs) > 1 {
		form := huh.NewForm(huh.NewGroup(boardSelect(boardIDs, &boardID)))
		if !runCancelable(ctx, form, i18n.Tr("Flash canceled.")) {
			return
		}
	}
	board, ok := registry.BoardByID(boardID)
	if !ok {
		feedback.Fatal(i18n.Tr("unknown board %s", boardID), feedback.ErrBadArgument)
	}

	// Asked rather than read: reading needs the programmer that ships with the
	// image, which is not downloaded yet. Checked against the real GPT later.
	variantOptions := make([]huh.Option[registry.Variant], 0, len(board.Variants))
	for _, v := range board.Variants {
		variantOptions = append(variantOptions, huh.NewOption(v.Label, v))
	}

	// Asked only when the board has images of more than one, so what follows is
	// a single distribution and its newest image is simply the first.
	boardImages := images[boardID]
	if oses := slices.Sorted(slices.Values(boardImages.OSes())); len(oses) > 1 {
		var imageOs string
		if !selectOs(ctx, oses, &imageOs) {
			return
		}
		boardImages = boardImages.Filter(imageOs, "")
	}

	// Newest first, so the first one is this board's latest.
	versionOptions := imageOptions(boardImages)

	var (
		variant      registry.Variant
		selected     registry.Release
		preserveUser bool
		rootPctStr   string
		confirm      bool
	)

	var groups []*huh.Group

	// Only for a board whose storage configurations are known: without them
	// there is no capacity to offer, and no root size to compute from it.
	if len(variantOptions) > 0 {
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[registry.Variant]().
				Title(i18n.Tr("Select your %s", board.Label)).
				Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
				Options(variantOptions...).
				Value(&variant),
		))
	}

	groups = append(groups, huh.NewGroup(
		huh.NewSelect[registry.Release]().
			Title(i18n.Tr("Select the image version to flash")).
			Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
			Options(versionOptions...).
			Value(&selected),
	))

	if board.PreserveUser {
		groups = append(groups, huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.Tr("Preserve user partition?")).
				Description(i18n.Tr("Keep existing user data on the board")).
				Affirmative(i18n.Tr("Yes")).
				Negative(i18n.Tr("No")).
				Value(&preserveUser),
		))
	}

	// The root/home split is a percentage of total storage assigned to root. The
	// description is re-evaluated on every keystroke (huh bindings) so the sizes
	// are shown live.
	if len(variantOptions) > 0 {
		groups = append(groups, huh.NewGroup(
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
		).WithHideFunc(func() bool { return preserveUser }))
	}

	boardLabel := cmp.Or(variant.Label, board.Label)
	groups = append(groups, huh.NewGroup(
		huh.NewConfirm().
			Title(i18n.Tr("Ready to flash")).
			// A group fixes its height when built, before DescriptionFunc has
			// run, so the summary is also given statically for it to measure:
			// without it the buttons fall outside the viewport. Before the func,
			// which Description would otherwise clear.
			Description(buildSummary(board, boardLabel, "", false, 0)).
			DescriptionFunc(func() string {
				return buildSummary(board, cmp.Or(variant.Label, boardLabel), selected.Version, preserveUser, parseRootSize(variant.Capacity, rootPctStr, preserveUser))
			}, []any{&variant, &selected, &preserveUser, &rootPctStr}).
			Affirmative(i18n.Tr("Flash now")).
			Negative(i18n.Tr("Cancel")).
			Value(&confirm),
	))

	form := huh.NewForm(groups...)
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
	if err := updater.DownloadAndFlash(ctx, board.ID, selected, opts); err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print(i18n.Tr("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper."))
}

// SelectImage asks which board to use and, when withVersion, which of its
// images. One form holds both, so the user can go back. False means canceled.
func SelectImage(ctx context.Context, withVersion bool) (registry.Board, string, string, bool) {
	images := fetchImages(ctx)
	boardIDs := slices.Sorted(maps.Keys(images))
	if len(boardIDs) == 0 {
		feedback.Fatal(i18n.Tr("no image is published yet"), feedback.ErrGeneric)
	}

	boardID := boardIDs[0]
	var groups []*huh.Group
	if len(boardIDs) > 1 {
		groups = append(groups, huh.NewGroup(boardSelect(boardIDs, &boardID)))
	}

	var selected registry.Release
	if withVersion {
		// Bound to the board, so going back relists the images.
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[registry.Release]().
				Title(i18n.Tr("Select the image version")).
				Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
				OptionsFunc(func() []huh.Option[registry.Release] {
					return imageOptions(images[boardID])
				}, &boardID).
				Value(&selected),
		))
	}

	if len(groups) > 0 && !runCancelable(ctx, huh.NewForm(groups...), i18n.Tr("Canceled.")) {
		return registry.Board{}, "", "", false
	}
	board, _ := registry.BoardByID(boardID)
	return board, selected.OS, selected.Version, true
}

// fetchImages reads every index once and groups what is published by board,
// newest first, so a board appears the day its images do.
func fetchImages(ctx context.Context) map[string]registry.Releases {
	var all registry.Releases
	var partial error
	sp := spinner.New().
		Context(ctx).
		Title(i18n.Tr("Fetching available images...")).
		ActionWithErr(func(ctx context.Context) error {
			releases, err := registry.NewClient().Fetch(ctx)
			if len(releases) == 0 {
				// Only fails when no index at all could be read.
				return err
			}
			all, partial = releases, err
			return nil
		})
	if err := sp.Run(); err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}
	// After the spinner has the line back, so what is on offer is not silently
	// short of an index.
	if partial != nil {
		feedback.Warning(partial.Error())
	}

	// Already newest first, so each board keeps that order.
	images := map[string]registry.Releases{}
	for _, r := range all {
		images[r.Board] = append(images[r.Board], r)
	}
	return images
}

// selectOs asks which distribution to flash, and reports whether the wizard
// should go on.
func selectOs(ctx context.Context, oses []string, imageOs *string) bool {
	options := make([]huh.Option[string], 0, len(oses))
	for _, o := range oses {
		options = append(options, huh.NewOption(o, o))
	}
	return runCancelable(ctx, huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(i18n.Tr("Select the distribution to flash")).
			Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
			Options(options...).
			Value(imageOs),
	)), i18n.Tr("Flash canceled."))
}

// runCancelable shows a form, and reports whether the caller should go on.
func runCancelable(ctx context.Context, form *huh.Form, canceled string) bool {
	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			feedback.Print(canceled)
			return false
		}
		feedback.Fatal(i18n.Tr("error running interactive wizard: %v", err), feedback.ErrBadArgument)
	}
	return true
}

// boardSelect asks which board, among those that have a published image.
func boardSelect(boardIDs []string, boardID *string) *huh.Select[string] {
	options := make([]huh.Option[string], 0, len(boardIDs))
	for _, id := range boardIDs {
		b, _ := registry.BoardByID(id)
		options = append(options, huh.NewOption(b.Label, id))
	}
	return huh.NewSelect[string]().
		Title(i18n.Tr("Select your board")).
		Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
		Options(options...).
		Value(boardID)
}

// imageOptions lists images newest first, saying which one is the latest and,
// when the board has more than one distribution, which one it comes from.
func imageOptions(images registry.Releases) []huh.Option[registry.Release] {
	oses := images.OSes()
	options := make([]huh.Option[registry.Release], 0, len(images))
	for i, rel := range images {
		label := rel.Version
		if i == 0 {
			label += " (latest)"
		}
		if len(oses) > 1 {
			label += " — " + rel.OS
		}
		options = append(options, huh.NewOption(label, rel))
	}
	return options
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

func buildSummary(board registry.Board, boardLabel, version string, preserveUser bool, rootSize uint64) string {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, i18n.Tr("Board:")+"\t"+boardLabel)
	fmt.Fprintln(w, i18n.Tr("Image version:")+"\t"+version)
	// Both come off the board's partition table, so a board without one was
	// asked neither and is reported neither.
	if board.PreserveUser {
		userPartition := i18n.Tr("will be erased")
		if preserveUser {
			userPartition = i18n.Tr("preserved")
		}
		fmt.Fprintln(w, i18n.Tr("User partition:")+"\t"+userPartition)

		rootSizeStr := i18n.Tr("auto-detect")
		if rootSize > 0 {
			rootSizeStr = humanize.IBytes(rootSize)
		}
		fmt.Fprintln(w, i18n.Tr("Root size:")+"\t"+rootSizeStr)
	}
	_ = w.Flush()

	if !preserveUser {
		buf.WriteString("\n" + i18n.Tr("WARNING: This will erase existing data on the board."))
	}

	return buf.String()
}

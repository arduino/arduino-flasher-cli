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

// image is a release together with the index it was found in, which is what the
// flash needs and the release itself does not carry.
type image struct {
	index   string
	release registry.Release
}

// Run starts the interactive wizard and performs the flash.
func Run(ctx context.Context) {
	images := fetchImages(ctx)

	boardIDs := slices.Sorted(maps.Keys(images))
	boardID := boardIDs[0]
	if len(boardIDs) > 1 {
		if !selectBoard(ctx, boardIDs, &boardID) {
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

	// Newest first, so the first one is this board's latest.
	boardImages := images[boardID]
	versionOptions := make([]huh.Option[image], 0, len(boardImages))
	for i, img := range boardImages {
		label := img.release.Version
		if i == 0 {
			label += " (latest)"
		}
		if len(registry.Indexes()) > 1 {
			label += " — " + img.index
		}
		versionOptions = append(versionOptions, huh.NewOption(label, img))
	}

	var (
		variant      registry.Variant
		selected     image
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
		huh.NewSelect[image]().
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
			DescriptionFunc(func() string {
				return buildSummary(cmp.Or(variant.Label, boardLabel), selected.release.Version, preserveUser, parseRootSize(variant.Capacity, rootPctStr, preserveUser))
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
	if err := updater.DownloadAndFlash(ctx, board.ID, selected.index, selected.release.Version, opts); err != nil {
		feedback.Fatal(i18n.Tr("error flashing the board: %v", err), feedback.ErrBadArgument)
	}
	feedback.Print(i18n.Tr("\nThe board has been successfully flashed. You can now power-cycle the board (unplug and re-plug). Remember to remove the jumper."))
}

// fetchImages reads every index and groups what is published by board, newest
// first. Boards are offered by what exists, so one appears the day its images do.
func fetchImages(ctx context.Context) map[string][]image {
	client := registry.NewClient()
	images := map[string][]image{}

	var errs []error
	sp := spinner.New().
		Context(ctx).
		Title(i18n.Tr("Fetching available images...")).
		ActionWithErr(func(ctx context.Context) error {
			for _, index := range registry.Indexes() {
				manifest, err := client.GetInfoManifest(ctx, index)
				if err != nil {
					// One index being unreachable should not hide the others.
					errs = append(errs, err)
					continue
				}
				for _, r := range manifest.Releases {
					// An index names a board by its label, and can hold releases
					// of boards this flasher knows nothing about.
					b, ok := registry.BoardByLabel(r.Board)
					if !ok {
						continue
					}
					images[b.ID] = append(images[b.ID], image{index: index, release: r})
				}
			}
			if len(images) == 0 {
				return errors.Join(errs...)
			}
			return nil
		})
	if err := sp.Run(); err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}

	for board := range images {
		slices.SortFunc(images[board], func(a, b image) int {
			return strings.Compare(b.release.Version, a.release.Version)
		})
	}
	return images
}

// selectBoard asks which board to flash, and reports whether the wizard should
// go on.
func selectBoard(ctx context.Context, boardIDs []string, boardID *string) bool {
	options := make([]huh.Option[string], 0, len(boardIDs))
	for _, id := range boardIDs {
		b, _ := registry.BoardByID(id)
		options = append(options, huh.NewOption(b.Label, id))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(i18n.Tr("Select your board")).
			Description(i18n.Tr("Use ↑/↓ to navigate, Enter to confirm")).
			Options(options...).
			Value(boardID),
	))
	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			feedback.Print(i18n.Tr("Flash canceled."))
			return false
		}
		feedback.Fatal(i18n.Tr("error running interactive wizard: %v", err), feedback.ErrBadArgument)
	}
	return true
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

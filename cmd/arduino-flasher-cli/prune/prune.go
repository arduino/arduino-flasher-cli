// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package prune

import (
	"os"
	"path/filepath"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/store"
)

// NewPruneCmd creates the command that removes everything in the image store except the stored images.
func NewPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove partial downloads and other files the flasher left",
		Long: `Remove everything in the image store except the stored images: partial
downloads, images that a flash did not keep, and temporary directories that a
crash left. Run it when no flash is running.`,
		Example: " " + os.Args[0] + " prune\n",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			imageStore, err := store.Default()
			if err != nil {
				feedback.Fatal(i18n.Tr("error opening the image store: %v", err), feedback.ErrGeneric)
			}
			freed, err := imageStore.Prune()
			if err != nil {
				feedback.Fatal(i18n.Tr("error pruning the image store: %v", err), feedback.ErrGeneric)
			}
			feedback.PrintResult(pruneResult{Directory: filepath.Dir(imageStore.Dir()), Freed: freed})
		},
	}
}

type pruneResult struct {
	Directory string `json:"directory"`
	Freed     int64  `json:"freed"`
}

func (r pruneResult) Data() interface{} {
	return r
}

func (r pruneResult) String() string {
	// Prune returns only non-negative sizes.
	return i18n.Tr("Image store pruned: %s\nFreed: %s", r.Directory, humanize.IBytes(uint64(r.Freed))) //nolint:gosec
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package remove

import (
	"cmp"
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
)

// NewRemoveCmd creates the command that removes stored images from the image store.
func NewRemoveCmd() *cobra.Command {
	var osStr, version string
	var all bool
	cmd := &cobra.Command{
		Use:   "remove [board]",
		Short: "Remove stored images from the image store",
		Long: `Remove a stored image from the image store, or all of them with --all.

The image is selected as in download: the most recent one unless a --version is
specified, and --os selects the distribution.
`,
		Example: " " + os.Args[0] + " remove unoq\n" +
			" " + os.Args[0] + " remove unoq --version 20250915-173\n" +
			" " + os.Args[0] + " remove --all\n",
		Args: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) == 1) {
				return errors.New(i18n.Tr("give a board, or --all"))
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			imageStore, err := store.Default()
			if err != nil {
				feedback.Fatal(i18n.Tr("error opening the image store: %v", err), feedback.ErrGeneric)
			}
			names := []string{}
			if all {
				if names, err = imageStore.Stored(); err != nil {
					feedback.Fatal(err.Error(), feedback.ErrGeneric)
				}
			} else {
				board, ok := registry.BoardByID(args[0])
				if !ok {
					feedback.Fatal(i18n.Tr("%s is not a board, use one of: %s", args[0], strings.Join(registry.BoardIDs(), ", ")), feedback.ErrBadArgument)
				}
				releases, err := registry.NewClient().Fetch(cmd.Context())
				if err != nil {
					feedback.Warning(err.Error())
				}
				rel, err := releases.Resolve(cmp.Or(osStr, board.DefaultOs), board.ID, version)
				if err != nil {
					feedback.Fatal(i18n.Tr("error looking up the image: %v", err), feedback.ErrBadArgument)
				}
				names = append(names, rel.FileName)
			}
			for _, name := range names {
				if err := imageStore.Remove(name); err != nil {
					feedback.Fatal(i18n.Tr("error removing the image: %v", err), feedback.ErrBadArgument)
				}
			}
			feedback.PrintResult(removeResult{Removed: names})
		},
	}
	cmd.Flags().StringVarP(&version, "version", "v", "", "Version of the image to remove. Leave empty for latest")
	cmd.Flags().StringVar(&osStr, "os", "", "Distribution of the image, if more than one is available")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Remove all stored images")
	return cmd
}

type removeResult struct {
	Removed []string `json:"removed"`
}

func (r removeResult) Data() interface{} {
	return r
}

func (r removeResult) String() string {
	if len(r.Removed) == 0 {
		return i18n.Tr("No stored images to remove.")
	}
	return i18n.Tr("Removed images:") + "\n  " + strings.Join(r.Removed, "\n  ")
}

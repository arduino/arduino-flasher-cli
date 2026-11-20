// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package images

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
	"go.bug.st/f"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/store"
	"github.com/arduino/arduino-flasher-cli/internal/tablestyle"
)

// NewImagesCmd creates the commands for inspecting and removing the kept images.
func NewImagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "images",
		Short: "Inspect and remove the kept Linux images",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newListCmd(), newRemoveCmd())
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the kept images and their disk usage",
		Long: `List the kept images without accessing the network.

Shows the archive name, size and download date of the kept images, and the size
of the partial downloads and of the other files, which prune removes.
`,
		Example: " " + os.Args[0] + " images list\n" +
			" " + os.Args[0] + " images list --format json\n",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			c, err := store.Default()
			if err != nil {
				feedback.Fatal(i18n.Tr("error opening the image store: %v", err), feedback.ErrGeneric)
			}
			inventory, err := c.List()
			if err != nil {
				feedback.Fatal(i18n.Tr("error listing the image store: %v", err), feedback.ErrGeneric)
			}
			feedback.PrintResult(listResult{
				Directory:   c.Dir(),
				PartialSize: inventory.PartialSize,
				OtherSize:   inventory.OtherSize,
				Images: f.Map(inventory.Images, func(image store.Entry) keptImage {
					return keptImage{
						FileName:     image.FileName,
						Size:         image.Size,
						DownloadedAt: image.DownloadedAt,
						Path:         filepath.Join(c.Dir(), image.FileName),
					}
				}),
			})
		},
	}
}

func newRemoveCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "remove [file...]",
		Short: "Remove kept images",
		Long:  "Remove the kept images with the file names that images list shows, or all of them with --all.",
		Example: " " + os.Args[0] + " images remove arduino-unoq-debian-image-20250915-173.tar.zst\n" +
			" " + os.Args[0] + " images remove --all\n",
		Args: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return errors.New(i18n.Tr("give the file names of the images to remove, or --all"))
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			c, err := store.Default()
			if err != nil {
				feedback.Fatal(i18n.Tr("error opening the image store: %v", err), feedback.ErrGeneric)
			}
			if all {
				inventory, err := c.List()
				if err != nil {
					feedback.Fatal(i18n.Tr("error listing the image store: %v", err), feedback.ErrGeneric)
				}
				args = f.Map(inventory.Images, func(image store.Entry) string { return image.FileName })
			}
			for _, name := range args {
				if err := c.Remove(name); err != nil {
					feedback.Fatal(i18n.Tr("error removing the image: %v", err), feedback.ErrBadArgument)
				}
			}
			feedback.PrintResult(removeResult{Removed: args})
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Remove all kept images")
	return cmd
}

type listResult struct {
	Directory   string      `json:"directory"`
	Images      []keptImage `json:"images"`
	PartialSize int64       `json:"partial_size"`
	OtherSize   int64       `json:"other_size"`
}

type keptImage struct {
	FileName     string    `json:"file_name"`
	Size         int64     `json:"size"`
	DownloadedAt time.Time `json:"downloaded_at"`
	Path         string    `json:"path,omitempty"`
}

func (r listResult) Data() interface{} {
	return r
}

func (r listResult) String() string {
	const (
		columnFile       = "FILE"
		columnSize       = "SIZE"
		columnDownloaded = "DOWNLOADED"
	)
	header := i18n.Tr("Image store: %s", r.Directory)
	footer := ""
	if r.PartialSize > 0 || r.OtherSize > 0 {
		// List returns only non-negative sizes.
		footer = "\n\n" + i18n.Tr("Partial downloads: %s\nOther files: %s\nRun prune to remove them.",
			humanize.IBytes(uint64(r.PartialSize)), humanize.IBytes(uint64(r.OtherSize))) //nolint:gosec
	}
	if len(r.Images) == 0 {
		return header + "\n\n" + i18n.Tr("No images are kept.") + footer
	}
	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{columnFile, columnSize, columnDownloaded})
	t.SetColumnConfigs([]table.ColumnConfig{
		{Name: columnFile, Align: text.AlignLeft},
		{Name: columnSize, Align: text.AlignLeft},
		{Name: columnDownloaded, Align: text.AlignLeft},
	})
	for _, image := range r.Images {
		// List returns only non-negative archive sizes.
		size := humanize.IBytes(uint64(image.Size)) //nolint:gosec
		t.AppendRow(table.Row{image.FileName, size, image.DownloadedAt.Format(time.RFC3339)})
	}
	return header + "\n\n" + t.Render() + footer
}

type removeResult struct {
	Removed []string `json:"removed"`
}

func (r removeResult) Data() interface{} {
	return r
}

func (r removeResult) String() string {
	if len(r.Removed) == 0 {
		return i18n.Tr("No kept images to remove.")
	}
	return i18n.Tr("Removed images:") + "\n  " + strings.Join(r.Removed, "\n  ")
}

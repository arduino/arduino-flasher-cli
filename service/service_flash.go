// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/codeclysm/extract/v4"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

const (
	taskExtract = "extract"
	taskFlash   = "flash"
)

func (s *flasherServerImpl) Flash(req *flasher.FlashRequest, stream flasher.Flasher_FlashServer) error {
	// An image is a version of one board for one distribution, so all three are
	// required: List reports them, and the board cannot be read before flashing.
	if req.GetBoard() == "" {
		return fmt.Errorf("no board requested, it must be one of: %s", strings.Join(registry.BoardIDs(), ", "))
	}
	board, ok := registry.BoardByID(req.GetBoard())
	if !ok {
		return fmt.Errorf("%q is not a board, use one of: %s", req.GetBoard(), strings.Join(registry.BoardIDs(), ", "))
	}
	if req.GetVersion() == "" {
		return fmt.Errorf("no image version requested, call List to pick one")
	}
	if req.GetOs() == "" {
		return fmt.Errorf("no image distribution requested, call List to pick one")
	}

	responseCallback := NewSynchronizedSend(stream.Send).Send
	ctx := stream.Context()
	var downloadCB flasher.DownloadProgressCB = func(msg *flasher.DownloadProgress) {
		_ = responseCallback(&flasher.FlashResponse{
			Message: &flasher.FlashResponse_DownloadProgress{
				DownloadProgress: msg,
			},
		})
	}
	extractCB := func(msg *flasher.TaskProgress) {
		_ = responseCallback(&flasher.FlashResponse{
			Message: &flasher.FlashResponse_ExtractionProgress{
				ExtractionProgress: msg,
			},
		})
	}
	flashCB := func(msg *flasher.TaskProgress) {
		_ = responseCallback(&flasher.FlashResponse{
			Message: &flasher.FlashResponse_FlashProgress{
				FlashProgress: msg,
			},
		})
	}

	client := registry.NewClient()

	releases, fetchErr := client.Fetch(ctx)
	rel, err := releases.Resolve(req.GetOs(), board.ID, req.GetVersion())
	if err != nil {
		// An index that could not be read may be the one holding it.
		return fmt.Errorf("could not get release info: %w", errors.Join(err, fetchErr))
	}

	imageStore, err := store.Default()
	if err != nil {
		return err
	}
	// Extract in a private directory, never in the client-supplied parent.
	tmpPath, err := store.TempDir("grpc-flash-", req.GetTempPath())
	if err != nil {
		return err
	}
	defer func() { _ = tmpPath.RemoveAll() }()

	downloadCB.Start(rel.Url, rel.Version)
	archive, found, err := imageStore.Get(ctx, rel, req.GetKeep(), func(ctx context.Context, dir string) error {
		_, err := client.DownloadFile(ctx, paths.New(dir), rel, downloadCB.Update)
		return err
	})
	if err != nil {
		downloadCB.End(false, err.Error())
		return err
	}
	message := ""
	if found {
		message = "Using stored image"
	}
	downloadCB.End(true, message)

	tmpZipFile, err := paths.New(archive).Open()
	if err != nil {
		return fmt.Errorf("could not open archive: %w", err)
	}
	extractPath := tmpPath.Join("extracted")
	extractCB(&flasher.TaskProgress{Name: taskExtract, Message: "Extracting image archive"})
	err = extract.Archive(ctx, tmpZipFile, extractPath.String(), func(s string) string {
		extractCB(&flasher.TaskProgress{Name: taskExtract, Message: s})
		return s
	})
	// Discard needs the archive closed.
	_ = tmpZipFile.Close()
	if err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	extractCB(&flasher.TaskProgress{Name: taskExtract, Completed: true})

	flashCB(&flasher.TaskProgress{Name: taskFlash, Message: "Flashing image"})
	if err := s.flashBoard(ctx, req.GetSerial(), extractPath, rel.Version, board.ID, req.GetPreserveUser(), 0, func(fe updater.FlashEvent) {
		flashCB(&flasher.TaskProgress{
			Name:     taskFlash,
			Message:  fe.Log,
			Progress: int64(fe.Progress),
			Total:    int64(fe.Total),
		})
	}); err != nil {
		return err
	}
	flashCB(&flasher.TaskProgress{Name: taskFlash, Completed: true})
	imageStore.Discard(archive)

	return responseCallback(&flasher.FlashResponse{
		Message: &flasher.FlashResponse_Result_{
			Result: &flasher.FlashResponse_Result{},
		},
	})
}

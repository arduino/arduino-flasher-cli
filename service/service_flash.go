// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/codeclysm/extract/v4"

	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
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

	if stream == nil {
		return errors.New("no flash response stream provided")
	}
	ctx, cancel := context.WithCancelCause(stream.Context())
	defer cancel(nil)
	if err := context.Cause(ctx); err != nil {
		return err
	}
	// Stop work if the client can no longer receive progress. CancelCause keeps
	// the first send error and is safe for callbacks from different goroutines.
	syncSend := NewSynchronizedSend(func(msg *flasher.FlashResponse) error {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		if err := stream.Send(msg); err != nil {
			cancel(err)
			return err
		}
		return nil
	})
	responseCallback := syncSend.Send
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

	// Own a private directory, never the client-supplied parent. Shared cached
	// archives live outside this directory and survive all request outcomes.
	tmpPath, err := updater.SetTempDir("grpc-flash-", req.GetTempPath())
	if err != nil {
		return err
	}
	defer func() { _ = tmpPath.RemoveAll() }()

	downloadCB.Start(rel.Url, rel.Version)
	imageCache, err := cache.Default()
	if err != nil {
		downloadCB.End(false, err.Error())
		return err
	}
	archive, err := imageCache.Lookup(ctx, rel)
	fromCache := err == nil
	var tmpZip *paths.Path
	if fromCache {
		tmpZip = paths.New(archive)
		info, statErr := os.Stat(archive)
		if statErr != nil {
			downloadCB.End(false, statErr.Error())
			return statErr
		}
		downloadCB.Update(info.Size(), info.Size())
	} else if errors.Is(err, cache.ErrMiss) {
		if req.GetCache() {
			archive, err = imageCache.Store(ctx, rel, func(ctx context.Context, dir string) error {
				_, err := client.DownloadFile(ctx, paths.New(dir), rel, downloadCB.Update)
				return err
			})
			tmpZip = paths.New(archive)
		} else {
			tmpZip, err = client.DownloadFile(ctx, tmpPath, rel, downloadCB.Update)
		}
	}
	if err != nil {
		downloadCB.End(false, err.Error())
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return err
	}
	message := ""
	if fromCache {
		message = "Using cached image"
	}
	downloadCB.End(true, message)
	if err := context.Cause(ctx); err != nil {
		return err
	}

	tmpZipFile, err := tmpZip.Open()
	if err != nil {
		return fmt.Errorf("could not open archive: %w", err)
	}
	defer tmpZipFile.Close()

	extractPath := tmpPath.Join("extracted")
	extractCB(&flasher.TaskProgress{Name: taskExtract, Message: "Extracting image archive"})
	if err := extract.Archive(ctx, tmpZipFile, extractPath.String(), func(s string) string {
		extractCB(&flasher.TaskProgress{Name: taskExtract, Message: s})
		return s
	}); err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	extractCB(&flasher.TaskProgress{Name: taskExtract, Completed: true})

	flashCB(&flasher.TaskProgress{Name: taskFlash, Message: "Flashing image"})
	if err := context.Cause(ctx); err != nil {
		return err
	}
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

	return responseCallback(&flasher.FlashResponse{
		Message: &flasher.FlashResponse_Result_{
			Result: &flasher.FlashResponse_Result{},
		},
	})
}

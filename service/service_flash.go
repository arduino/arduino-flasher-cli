// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"fmt"

	"github.com/arduino/go-paths-helper"
	"github.com/codeclysm/extract/v4"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/updater"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

const (
	taskExtract = "extract"
	taskFlash   = "flash"
)

func (s *flasherServerImpl) Flash(req *flasher.FlashRequest, stream flasher.Flasher_FlashServer) (outErr error) {

	// Setup callback functions
	var responseCallback func(*flasher.FlashResponse) error
	if stream != nil {
		syncSend := NewSynchronizedSend(stream.Send)
		responseCallback = syncSend.Send
	} else {
		responseCallback = func(*flasher.FlashResponse) error { return nil }
	}
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

	tmpPath := paths.New(req.GetTempPath())
	defer func() {
		// if the flash was successful, we can clean up the temporary files.
		if outErr == nil {
			_ = tmpPath.RemoveAll()
		}
	}()

	rel, err := client.GetReleaseByVersion(ctx, req.GetVersion(), boardToString(req.GetBoard()), osToString(req.GetOs()))
	if err != nil {
		return fmt.Errorf("could not get release info: %w", err)
	}

	downloadCB.Start(rel.Url, rel.Version)
	tmpZip, err := client.DownloadFile(ctx, tmpPath, rel, downloadCB.Update)
	if err != nil {
		// FIXME: Maybe this is redundant?
		downloadCB.End(false, err.Error())
		return err
	}
	downloadCB.End(true, "")

	tmpZipFile, err := tmpZip.Open()
	if err != nil {
		return fmt.Errorf("could not open archive: %w", err)
	}
	defer tmpZipFile.Close()

	extractPath := tmpPath.Join("extracted")
	defer func() {
		_ = extractPath.RemoveAll()
	}()
	extractCB(&flasher.TaskProgress{Name: taskExtract, Message: "Extracting image archive"})
	if err := extract.Archive(ctx, tmpZipFile, extractPath.String(), func(s string) string {
		extractCB(&flasher.TaskProgress{Name: taskExtract, Message: s})
		return s
	}); err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	extractCB(&flasher.TaskProgress{Name: taskExtract, Completed: true})

	flashCB(&flasher.TaskProgress{Name: taskFlash, Message: "Flashing image"})
	if err := updater.FlashBoard(ctx, req.Serial, extractPath, rel.Version, boardToString(req.GetBoard()), req.GetPreserveUser(), 0, func(fe updater.FlashEvent) {
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

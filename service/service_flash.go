// This file is part of arduino-flasher-cli.
//
// Copyright 2025 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-flasher-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to
// modify or otherwise use the software for commercial activities involving the
// Arduino software without disclosing the source code of your own applications.
// To purchase a commercial license, send an email to license@arduino.cc.

package service

import (
	"fmt"

	"github.com/arduino/go-paths-helper"
	"github.com/codeclysm/extract/v4"
	"go.bug.st/downloader/v2"

	"github.com/arduino/arduino-flasher-cli/internal/updater"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

func (s *flasherServerImpl) Flash(req *flasher.FlashRequest, stream flasher.Flasher_FlashServer) error {
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

	client := updater.NewClient()

	rel, err := client.GetReleaseByVersion(ctx, req.GetVersion())
	if err != nil {
		return fmt.Errorf("could not get release info: %w", err)
	}

	tmpZip := paths.New(req.GetTempPath(), "arduino-unoq-debian-image-"+rel.Version+".tar.zst")
	defer func() { _ = tmpZip.RemoveAll() }()

	downloadCB.Start(rel.Url, rel.Version)
	if err := client.DownloadFile(ctx, tmpZip, rel, downloadCB.Update, downloader.Config{}); err != nil {
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

	extractCB(&flasher.TaskProgress{Name: "extract", Message: "Extracting image archive"})
	if err := extract.Archive(ctx, tmpZipFile, tmpZip.Parent().String(), func(s string) string {
		extractCB(&flasher.TaskProgress{Name: "extract", Message: s})
		return s
	}); err != nil {
		return fmt.Errorf("could not extract archive: %w", err)
	}
	extractCB(&flasher.TaskProgress{Name: "extract", Completed: true})

	imagePath := tmpZip.Parent().Join("arduino-unoq-debian-image-" + rel.Version)
	defer func() { _ = imagePath.RemoveAll() }()

	flashCB(&flasher.TaskProgress{Name: "flash", Message: "Flashing image"})
	if err := updater.FlashBoard(ctx, imagePath.String(), rel.Version, req.GetPreserveUser(), func(fe updater.FlashEvent) {
		flashCB(&flasher.TaskProgress{
			Name:     "flash",
			Message:  fe.Log,
			Progress: int64(fe.Progress),
			Total:    int64(fe.MaxProgress),
		})

	}); err != nil {
		return err
	}
	flashCB(&flasher.TaskProgress{Name: "flash", Completed: true})

	return responseCallback(&flasher.FlashResponse{
		Message: &flasher.FlashResponse_Result_{
			Result: &flasher.FlashResponse_Result{},
		},
	})
}

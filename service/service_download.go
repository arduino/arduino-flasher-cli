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
	"go.bug.st/downloader/v2"

	"github.com/arduino/arduino-flasher-cli/internal/updater"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

func (s *flasherServerImpl) Download(req *flasher.DownloadRequest, stream flasher.Flasher_DownloadServer) error {
	syncSend := NewSynchronizedSend(stream.Send)
	ctx := stream.Context()
	downloadCB := func(p *flasher.DownloadProgress) {
		_ = syncSend.Send(&flasher.DownloadResponse{
			Message: &flasher.DownloadResponse_Progress{Progress: p},
		})
	}

	client := updater.NewClient()
	manifest, err := client.GetInfoManifest(ctx)
	if err != nil {
		return err
	}

	var rel *updater.Release
	if req.Version == "latest" || req.Version == manifest.Latest.Version {
		rel = &manifest.Latest
	} else {
		for _, r := range manifest.Releases {
			if req.Version == r.Version {
				rel = &r
				break
			}
		}
	}

	if rel == nil {
		return fmt.Errorf("could not find Debian image %s", req.Version)
	}

	tmpZip := paths.New(req.GetDownloadPath(), "arduino-unoq-debian-image-"+rel.Version+".tar.zst")

	if err := updater.DownloadFile(ctx, tmpZip, rel.Url, rel.Version, downloadCB, downloader.Config{}); err != nil {
		return err
	}

	return syncSend.Send(&flasher.DownloadResponse{
		Message: &flasher.DownloadResponse_Result_{
			Result: &flasher.DownloadResponse_Result{},
		},
	})
}

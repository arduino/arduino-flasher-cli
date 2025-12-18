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
	"cmp"
	"context"
	"slices"

	"github.com/arduino/arduino-flasher-cli/internal/updater"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
	"go.bug.st/f"
)

func (s *flasherServerImpl) List(ctx context.Context, req *flasher.ListRequest) (*flasher.ListResponse, error) {
	client := updater.NewClient()

	manifest, err := client.GetInfoManifest(ctx)
	if err != nil {
		return nil, err
	}

	releases := f.Map(manifest.Releases, func(r updater.Release) *flasher.Release {
		return &flasher.Release{
			BuildId: r.Version,
			Latest:  r.Version == manifest.Latest.Version,
		}
	})
	slices.SortFunc(releases, func(a, b *flasher.Release) int {
		if a.Latest {
			if b.Latest {
				return 0
			}
			return -1
		} else if b.Latest {
			return +1
		}
		return cmp.Compare(b.BuildId, a.BuildId)
	})

	return &flasher.ListResponse{Releases: releases}, nil
}

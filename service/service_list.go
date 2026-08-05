// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"cmp"
	"context"
	"slices"

	"go.bug.st/f"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

func (s *flasherServerImpl) List(ctx context.Context, req *flasher.ListRequest) (*flasher.ListResponse, error) {
	client := registry.NewClient()

	var releases []*flasher.Release
	board := boardToString(req.GetBoard())
	os := osToString(req.GetOs())
	manifest, err := client.GetInfoManifest(ctx, os)
	if err != nil {
		return nil, err
	}
	manifest.Releases = f.Filter(manifest.Releases, func(r registry.Release) bool {
		return r.Board == board
	})
	releases = f.Map(manifest.Releases, func(r registry.Release) *flasher.Release {
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
			return 1
		}
		return cmp.Compare(b.BuildId, a.BuildId)
	})

	return &flasher.ListResponse{Releases: releases}, nil
}

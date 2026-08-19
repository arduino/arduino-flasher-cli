// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

func (s *flasherServerImpl) List(ctx context.Context, req *flasher.ListRequest) (*flasher.ListResponse, error) {
	client := registry.NewClient()

	board := boardFromRPC(req.GetBoard())
	indexes := registry.Indexes()
	if os := osFromRPC(req.GetOs()); os != "" {
		indexes = []string{os}
	}

	var releases []*flasher.Release
	var errs []error
	for _, index := range indexes {
		manifest, err := client.GetInfoManifest(ctx, index)
		if err != nil {
			// One index being unreachable should not hide the others.
			errs = append(errs, err)
			continue
		}
		// Oldest first, so the last match is this board's most recent, which is
		// not necessarily the index's own latest.
		var matching []*flasher.Release
		for _, r := range manifest.Releases {
			// An index names a board by its label, so that is what a release
			// carries and what has to be turned back into a board.
			released, ok := registry.BoardByLabel(r.Board)
			if !ok || (board != "" && released.ID != board) {
				continue
			}
			matching = append(matching, &flasher.Release{
				BuildId: r.Version,
				Board:   boardToRPC(released.ID),
			})
		}
		if len(matching) > 0 {
			matching[len(matching)-1].Latest = true
		}
		releases = append(releases, matching...)
	}
	if len(errs) == len(indexes) {
		return nil, errors.Join(errs...)
	}
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

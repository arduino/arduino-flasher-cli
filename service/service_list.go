// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/store"
	flasher "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/v1"
)

func (s *flasherServerImpl) List(ctx context.Context, req *flasher.ListRequest) (*flasher.ListResponse, error) {
	// Filtering by a board that is not one of ours returns nothing, which reads
	// as nothing being published.
	if board := req.GetBoard(); board != "" {
		if _, ok := registry.BoardByID(board); !ok {
			return nil, fmt.Errorf("%q is not a board, use one of: %s", board, strings.Join(registry.BoardIDs(), ", "))
		}
	}

	// One fetch, then filtered: the index is not stored, so asking again is a round
	// trip for what is already in hand.
	all, err := registry.NewClient().Fetch(ctx)
	if err != nil && len(all) == 0 {
		// Nothing could be read, as opposed to nothing being published.
		return nil, err
	}

	// An unnamed board or distribution matches any. Already most recent first,
	// which is the order the response promises.
	matched := all.Filter(req.GetOs(), req.GetBoard())

	// An unavailable image store does not hide the published images.
	var inventory store.Inventory
	if imageStore, err := store.Default(); err == nil {
		inventory, _ = imageStore.List()
	}

	releases := make([]*flasher.Release, 0, len(matched))
	for _, r := range matched {
		releases = append(releases, &flasher.Release{
			BuildId: r.Version,
			Latest:  r.Latest,
			Board:   r.Board,
			Os:      r.OS,
			Kept:    inventory.Contains(r),
		})
	}

	return &flasher.ListResponse{Releases: releases}, nil
}

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

package commands

import (
	"context"

	"github.com/arduino/arduino-flasher-cli/internal/updater"
	rpc "github.com/arduino/arduino-flasher-cli/rpc/cc/arduino/flasher/cli/commands/v1"
)

func (s *arduinoCoreServerImpl) List(ctx context.Context, req *rpc.ListRequest) (*rpc.ListResponse, error) {
	client := updater.NewClient()

	manifest, err := client.GetInfoManifest(ctx)
	if err != nil {
		return nil, err
	}

	resp := &rpc.ListResponse{}
	for i := len(manifest.Releases) - 1; i >= 0; i-- {
		latest := false
		if manifest.Releases[i].Version == manifest.Latest.Version {
			latest = true
		}
		resp.Releases = append(resp.Releases, &rpc.Release{BuildId: manifest.Releases[i].Version, Latest: latest})
	}

	return resp, nil
}

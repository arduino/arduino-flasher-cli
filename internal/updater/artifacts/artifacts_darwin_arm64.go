// This file is part of arduino-flasher-cli.
//
// Copyright 2025 ARDUINO SA (http://www.arduino.cc/)
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	_ "embed"
)

//go:embed resources_darwin_arm64/qdl
var QdlBinary []byte

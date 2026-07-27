// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	_ "embed"
)

//go:embed read.xml
var ReadXML []byte

//go:embed detect.xml
var DetectXML []byte

//go:embed prog_firehose_ddr.elf
var ProgElf []byte

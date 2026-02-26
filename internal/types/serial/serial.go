// This file is part of arduino-flasher-cli.
//
// Copyright (C) Arduino s.r.l. and/or its affiliated companies
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

package serial

import (
	"fmt"
	"strconv"
)

type Serial struct {
	num int64
}

func FromNum(numStr string) (Serial, error) {
	num, err := strconv.ParseInt(numStr, 10, 64)
	return Serial{num: num}, err
}

func FromHex(hexStr string) (Serial, error) {
	num, err := strconv.ParseInt(hexStr, 16, 64)
	return Serial{num: num}, err
}

func (s Serial) Hex() string {
	return fmt.Sprintf("%08X", s.num)
}

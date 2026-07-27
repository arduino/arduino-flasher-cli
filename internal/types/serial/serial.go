// This file is part of arduino-flasher-cli.
//
// Copyright (C) Arduino s.r.l. and/or its affiliated companies
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package serial

import (
	"fmt"
	"strconv"
	"strings"
)

type Serial struct {
	num int64
}

func FromNum(numStr string) (Serial, error) {
	num, err := strconv.ParseInt(numStr, 10, 64)
	return Serial{num: num}, err
}

func FromHex(hexStr string) (Serial, error) {
	s := strings.TrimPrefix(strings.TrimPrefix(hexStr, "0x"), "0X")
	num, err := strconv.ParseInt(s, 16, 64)
	return Serial{num: num}, err
}

func (s Serial) Hex() string {
	return fmt.Sprintf("%08X", s.num)
}

// Decimal returns the serial as a base-10 string.
func (s Serial) Decimal() string {
	return strconv.FormatInt(s.num, 10)
}

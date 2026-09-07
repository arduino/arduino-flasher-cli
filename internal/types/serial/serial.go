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

// Parse converts a serial passed as a decimal or hexadecimal integer.
//
// A 0x/0X prefix forces hexadecimal. Otherwise the value is parsed as decimal,
// falling back to hexadecimal when it is not a valid decimal integer (e.g.
// 1A2B3C4D). Note that a value made only of digits is inherently ambiguous and
// is always interpreted as decimal; use the 0x prefix to force hexadecimal.
func Parse(str string) (Serial, error) {
	if strings.HasPrefix(str, "0x") || strings.HasPrefix(str, "0X") {
		return FromHex(str)
	}
	if s, err := FromNum(str); err == nil {
		return s, nil
	}
	return FromHex(str)
}

func (s Serial) Hex() string {
	return fmt.Sprintf("%08X", s.num)
}

// Decimal returns the serial as a base-10 string.
func (s Serial) Decimal() string {
	return strconv.FormatInt(s.num, 10)
}

// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package helper

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
)

// CallbackWriter is a custom writer that processes each line calling the callback.
type CallbackWriter struct {
	callback func(line string)
	buffer   []byte
}

// NewCallbackWriter creates a new CallbackWriter.
func NewCallbackWriter(process func(line string)) *CallbackWriter {
	return &CallbackWriter{
		callback: process,
		buffer:   make([]byte, 0, 1024),
	}
}

// Write implements the io.Writer interface.
func (p *CallbackWriter) Write(data []byte) (int, error) {
	p.buffer = append(p.buffer, data...)
	for {
		idx := bytes.IndexByte(p.buffer, '\n')
		if idx == -1 {
			break
		}
		line := p.buffer[:idx] // Do not include \n
		p.buffer = p.buffer[idx+1:]
		p.callback(string(line))
	}
	return len(data), nil
}

func DecodeUTF16(b []byte) string {
	var ints []uint16
	for j := 0; j < len(b); j += 2 {
		r := binary.LittleEndian.Uint16(b[j : j+2])
		if r == 0 {
			break
		}
		ints = append(ints, r)
	}
	return string(utf16.Decode(ints))
}

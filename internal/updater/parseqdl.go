// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package updater

import (
	"encoding/xml"
	"strings"

	"github.com/arduino/go-paths-helper"
)

type Op int

const (
	Waiting Op = iota
	Flashed
	Unknown
)

type QDLLogLine struct {
	Op  Op
	Log string
}

func parseQdlLogLine(line string) QDLLogLine {
	lower := strings.ToLower(line)

	switch {
	case strings.HasPrefix(lower, "waiting for"):
		return QDLLogLine{
			Op:  Waiting,
			Log: line,
		}
	case strings.HasPrefix(lower, "flashed"):
		return QDLLogLine{
			Op:  Flashed,
			Log: line,
		}
	default:
		return QDLLogLine{
			Op:  Unknown,
			Log: line,
		}
	}
}

func getTotalPartition(path *paths.Path) (int, error) {
	rawProgramFile, err := parseRawProgramFile(path)
	if err != nil {
		return 0, err
	}

	var total int
	for _, program := range rawProgramFile.Programs {
		if program.Filename != "" {
			total++
		}
	}

	return total, nil
}

type RawProgramFile struct {
	Programs []Program `xml:"program"`
}

type Program struct {
	Filename string `xml:"filename,attr"`
}

func parseRawProgramFile(path *paths.Path) (RawProgramFile, error) {
	f, err := path.Open()
	if err != nil {
		return RawProgramFile{}, err
	}
	defer f.Close()

	var data RawProgramFile
	if err := xml.NewDecoder(f).Decode(&data); err != nil {
		return RawProgramFile{}, err
	}

	return data, nil
}

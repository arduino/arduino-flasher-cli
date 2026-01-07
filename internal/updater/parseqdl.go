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

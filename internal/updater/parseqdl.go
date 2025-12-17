package updater

import (
	"bufio"
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
	f, err := path.Open()
	if err != nil {
		return 0, err
	}

	r := bufio.NewScanner(f)
	var total int
	for r.Scan() {
		c := strings.Count(r.Text(), "<program")
		total += c
	}
	if err := r.Err(); err != nil {
		return 0, err
	}
	return total, nil
}

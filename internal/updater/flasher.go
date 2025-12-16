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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"iter"
	"log/slog"
	"regexp"
	"runtime"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/shirou/gopsutil/v4/disk"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/updater/artifacts"
)

const GiB = uint64(1024 * 1024 * 1024)
const DownloadDiskSpace = uint64(12)
const ExtractDiskSpace = uint64(10)

func Flash(ctx context.Context, imagePath *paths.Path, version string, forceYes bool, tempDir string) iter.Seq2[QDLEvent, error] {
	return func(yield func(QDLEvent, error) bool) {
		if !imagePath.Exist() {
			temp, err := SetTempDir("download-", tempDir)
			if err != nil {
				yield(QDLEvent{}, fmt.Errorf("error creating a temporary directory to download the archive: %v", err))
			}
			defer func() { _ = temp.RemoveAll() }()

			// Check if there is enough free disk space before downloading and extracting an image
			d, err := disk.Usage(temp.String())
			if err != nil {
				yield(QDLEvent{}, err)
				return
			}
			if d.Free/GiB < DownloadDiskSpace {
				yield(QDLEvent{}, fmt.Errorf("download and extraction requires up to %d GiB of free space", DownloadDiskSpace))
				return
			}

			tempImagePath, v, err := DownloadAndExtract(ctx, version, temp)
			if err != nil {
				yield(QDLEvent{}, fmt.Errorf("could not download and extract the image: %v", err))
				return
			}

			version = v
			imagePath = tempImagePath
		} else if !imagePath.IsDir() {
			temp, err := SetTempDir("extract-", tempDir)
			if err != nil {
				yield(QDLEvent{}, fmt.Errorf("error creating a temporary directory to extract the archive: %v", err))
				return
			}
			defer func() { _ = temp.RemoveAll() }()

			// Check if there is enough free disk space before extracting an image
			d, err := disk.Usage(temp.String())
			if err != nil {
				yield(QDLEvent{}, err)
				return
			}
			if d.Free/GiB < ExtractDiskSpace {
				yield(QDLEvent{}, fmt.Errorf("extraction requires up to %d GiB of free space", ExtractDiskSpace))
				return
			}

			err = ExtractImage(ctx, imagePath, temp)
			if err != nil {
				yield(QDLEvent{}, fmt.Errorf("could not extract the image: %v", err))
				return
			}

			tempContent, err := temp.ReadDir(paths.AndFilter(paths.FilterDirectories(), paths.FilterPrefixes("arduino-unoq-debian-image-")))
			if err != nil {
				yield(QDLEvent{}, fmt.Errorf("could not read extracted image directory: %v", err))
				return
			}

			imagePath = tempContent[0]
		}

		// forward the iterator
		FlashBoard(ctx, imagePath.String(), version)(yield)
	}
}

func FlashBoard(ctx context.Context, downloadedImagePath string, version string) iter.Seq2[QDLEvent, error] {
	return func(yield func(QDLEvent, error) bool) {
		var flashDir *paths.Path
		for _, entry := range []string{"flash", "flash_UnoQ"} {
			if p := paths.New(downloadedImagePath, entry); p.Exist() {
				flashDir = p
				break
			}
		}
		if flashDir == nil {
			yield(QDLEvent{}, fmt.Errorf("could not find the `flash` directory"))
			return
		}

		qdlDir, err := paths.MkTempDir("", "qdl-")
		if err != nil {
			_ = yield(QDLEvent{}, err)
			return
		}
		defer func() { _ = qdlDir.RemoveAll() }()

		qdlPath := qdlDir.Join("qdl")
		if runtime.GOOS == "windows" {
			qdlPath = qdlDir.Join("qdl.exe")
		}

		err = qdlPath.WriteFile(artifacts.QdlBinary)
		if err != nil {
			_ = yield(QDLEvent{}, err)
			return
		}
		err = qdlPath.Chmod(0755)
		if err != nil {
			_ = yield(QDLEvent{}, err)
			return
		}

		getTotalPartition := func(path *paths.Path) (int, error) {
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

		// TODO: add logic to preserve the user partition
		feedback.Print(i18n.Tr("Flashing with qdl"))
		cmd, err := paths.NewProcess(nil, qdlPath.String(), "--allow-missing", "--storage", "emmc", "prog_firehose_ddr.elf", "rawprogram0.xml", "patch0.xml")
		if err != nil {
			_ = yield(QDLEvent{}, err)
			return
		}
		// Setting the directory is needed because rawprogram0.xml contains relative file paths
		cmd.SetDir(flashDir.String())

		total, err := getTotalPartition(flashDir.Join("rawprogram0.xml"))
		if err != nil {
			_ = yield(QDLEvent{}, err)
			return
		}

		w := NewCallbackWriter(func(line string) {
			progress, err := parseQdlProgress(line)
			progress.NumPartitions = total
			if err == nil {
				yield(QDLEvent{
					Type:     TypeProgress,
					Progress: &progress,
				}, nil)
			} else {
				slog.Debug("Could not parse qdl progress line", "line", line, "error", err)
			}
		})
		cmd.RedirectStderrTo(w)
		cmd.RedirectStdoutTo(w)
		if err := cmd.RunWithinContext(ctx); err != nil {
			_ = yield(QDLEvent{}, err)
			return
		}
	}
}

type QDLEventType int

const (
	TypeProgress QDLEventType = 1
	// TypeWaiting  QDLEventType = 2
	// TypeDone     QDLEventType = 3
)

type QDLEvent struct {
	Type     QDLEventType
	Progress *QDLProgress
}

type QDLProgress struct {
	Op            string
	Name          string
	Status        string
	Speed         int // kB/s
	NumPartitions int
}

var qdlProgressRegex = regexp.MustCompile(`(\w)\s+(".*?")\s+(\w+)(?:\s+at\s+(\d+kB/s))?`)

func parseQdlProgress(line string) (QDLProgress, error) {
	matches := qdlProgressRegex.FindStringSubmatch(line)
	if matches == nil {
		return QDLProgress{}, fmt.Errorf("line does not match progress format")
	}

	parseSpeed := func(speedStr string) int {
		if speedStr == "" {
			return 0
		}
		var speed int
		fmt.Sscanf(speedStr, "%dkB/s", &speed)
		return speed
	}

	return QDLProgress{
		Op:     matches[1],
		Name:   matches[2],
		Status: matches[3],
		Speed:  parseSpeed(matches[4]),
	}, nil
}

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

func once2[X any, Y any](x X, y Y) iter.Seq2[X, Y] {
	return func(yield func(X, Y) bool) {
		_ = yield(x, y)
	}
}

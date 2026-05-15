package updater

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/arduino/arduino-flasher-cli/internal/helper"
	"github.com/arduino/go-paths-helper"
)

type GptTable struct {
	raw           []byte
	rootPartition PartitionEntry
	userPartition PartitionEntry
}

type PartitionEntry struct {
	PosFistLBA uint64
	FirstLBA   uint64
	PosLastLBA uint64
	LastLBA    uint64
}

const userdataPartitionName = "userdata"
const rootfsPartitionName = "rootfs"

func ParseGptTable(gptFile *paths.Path) (GptTable, error) {
	f, err := gptFile.Open()
	if err != nil {
		return GptTable{}, fmt.Errorf("Failed to open GPT file: %v", err)
	}
	defer f.Close()

	buf, err := io.ReadAll(f)
	if err != nil {
		return GptTable{}, fmt.Errorf("Failed to read GPT file: %v", err)
	}
	_ = f.Close()

	originalBuf := buf
	i := uint64(0)

	i, buf = i+512, buf[512:] // Skip MBR
	// signature := buf[0:8]

	sizePartitionEntry := binary.LittleEndian.Uint32(buf[84 : 84+4])

	i, buf = i+512, buf[512:] // Skip GPT header

	var userPartition, rootPartition PartitionEntry
	for len(buf) >= int(sizePartitionEntry) {
		partitionTypeGuid := buf[0:16]
		// uniquePartitionGuid := buf[16 : 16+16]
		firstLBA := binary.LittleEndian.Uint64(buf[32 : 32+8])
		lastLBA := binary.LittleEndian.Uint64(buf[40 : 40+8])
		// attributes := binary.LittleEndian.Uint64(buf[48 : 48+8])
		partitionName := helper.DecodeUTF16(buf[56 : 56+72])

		if partitionTypeGuid[0] == 0 {
			break
		}

		if partitionName == rootfsPartitionName {
			rootPartition.PosFistLBA = i + 32
			rootPartition.PosLastLBA = i + 40
			rootPartition.FirstLBA = firstLBA
			rootPartition.LastLBA = lastLBA
		}
		if partitionName == userdataPartitionName {
			userPartition.PosFistLBA = i + 32
			userPartition.PosLastLBA = i + 40
			userPartition.FirstLBA = firstLBA
			userPartition.LastLBA = lastLBA
		}

		// { //DEBUG
		// 	fmt.Printf("Partition Type GUID: %x\n", partitionTypeGuid)
		// 	fmt.Printf("Unique Partition GUID: %x\n", uniquePartitionGuid)
		// 	fmt.Printf("First LBA: %d (%f)Gb\n", firstLBA, float64(firstLBA*512)/1024/1024/1024)
		// 	fmt.Printf("Last LBA: %d (%f)Gb\n", lastLBA, float64(lastLBA*512)/1024/1024/1024)
		// 	fmt.Printf("Attributes: %x\n", attributes)
		// 	fmt.Printf("Partition Name: %s\n", partitionName)
		// }

		i, buf = i+uint64(sizePartitionEntry), buf[sizePartitionEntry:]
	}

	return GptTable{
		raw:           originalBuf,
		rootPartition: rootPartition,
		userPartition: userPartition,
	}, nil

}

func (t GptTable) ResizeRoot(gptFile *paths.Path, size uint64) error {
	newBuff := make([]byte, len(t.raw))
	copy(newBuff, t.raw)

	newSize := size / 512
	rootLastLBA := t.rootPartition.FirstLBA + newSize - 1
	binary.LittleEndian.PutUint64(newBuff[t.rootPartition.PosLastLBA:], rootLastLBA)
	binary.LittleEndian.PutUint64(newBuff[t.userPartition.PosFistLBA:], rootLastLBA+1)
	binary.LittleEndian.PutUint64(newBuff[t.userPartition.PosLastLBA:], rootLastLBA)

	return gptFile.WriteFile(newBuff)
}

var startSectorRegex = regexp.MustCompile(`start_sector="(\d+)"`)
var startByteHexRegex = regexp.MustCompile(`start_byte_hex="0x[0-9a-fA-F]+"`)
var numPartitionsSectorsRegex = regexp.MustCompile(`num_partition_sectors="\d+"`)
var sizeInKbRegex = regexp.MustCompile(`size_in_KB="\d+.\d"`)

func MoveUserdata(rawProgramFile *paths.Path, size uint64) error {
	f, err := rawProgramFile.Open()
	if err != nil {
		return fmt.Errorf("Failed to open rawprogram0.xml file: %v", err)
	}
	defer f.Close()

	newSize := size / 512

	scanner := bufio.NewScanner(f)
	newFileContent := make([]byte, 0, 1024)
	rootfsStartSector := uint64(0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, rootfsPartitionName) {
			match := startSectorRegex.FindStringSubmatch(line)
			if len(match) > 1 {
				rootfsStartSector, err = strconv.ParseUint(match[1], 10, 64)
				if err != nil {
					return err
				}
			}
			line = numPartitionsSectorsRegex.ReplaceAllString(line, fmt.Sprintf(`num_partition_sectors="%d"`, newSize))
			line = sizeInKbRegex.ReplaceAllString(line, fmt.Sprintf(`size_in_KB="%.1f"`, float64(size)/1024))
		}
		if strings.Contains(line, userdataPartitionName) {
			line = startSectorRegex.ReplaceAllString(line, fmt.Sprintf(`start_sector="%d"`, rootfsStartSector+newSize))
			newSizeHex := fmt.Sprintf("0x%x", (rootfsStartSector+newSize)*512)
			line = startByteHexRegex.ReplaceAllString(line, fmt.Sprintf(`start_byte_hex="%s"`, newSizeHex))
		}

		newFileContent = append(newFileContent, line...)
		newFileContent = append(newFileContent, '\n')
	}
	_ = f.Close()

	return rawProgramFile.WriteFile([]byte(newFileContent))
}

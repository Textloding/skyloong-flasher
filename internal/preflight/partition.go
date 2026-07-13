package preflight

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

const partitionEntrySize = 32

func ParsePartitionTable(raw []byte) ([]Partition, error) {
	parts := make([]Partition, 0, len(raw)/partitionEntrySize)
	for offset := 0; offset < len(raw); offset += partitionEntrySize {
		remaining := raw[offset:]
		if len(remaining) < partitionEntrySize {
			if allErased(remaining) {
				break
			}
			return nil, fmt.Errorf("分区表在偏移 %#x 处条目不完整：只有 %d 字节", offset, len(remaining))
		}

		entry := remaining[:partitionEntrySize]
		magic := binary.LittleEndian.Uint16(entry[0:2])
		if magic == 0xFFFF || magic == 0xEBEB {
			break
		}
		if magic != 0x50AA {
			return nil, fmt.Errorf("分区表在偏移 %#x 处 magic 无效：%#04x", offset, magic)
		}

		parts = append(parts, Partition{
			Type:    entry[2],
			Subtype: entry[3],
			Offset:  binary.LittleEndian.Uint32(entry[4:8]),
			Size:    binary.LittleEndian.Uint32(entry[8:12]),
			Label:   strings.TrimRight(string(entry[12:28]), "\x00\xff"),
			Flags:   binary.LittleEndian.Uint32(entry[28:32]),
		})
	}
	return parts, nil
}

func ValidatePartitions(parts []Partition) []CheckResult {
	ordered := append([]Partition(nil), parts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Offset < ordered[j].Offset
	})

	checks := make([]CheckResult, 0)
	for index := 1; index < len(ordered); index++ {
		previous := ordered[index-1]
		current := ordered[index]
		previousEnd := uint64(previous.Offset) + uint64(previous.Size)
		if uint64(current.Offset) < previousEnd {
			checks = append(checks, CheckResult{
				Code:      "partition_overlap",
				Status:    StatusWarning,
				Title:     "固件分区发生重叠",
				Summary:   fmt.Sprintf("分区 %q 与 %q 的地址范围重叠", partitionName(previous), partitionName(current)),
				Technical: fmt.Sprintf("%s end=%#x, %s offset=%#x", partitionName(previous), previousEnd, partitionName(current), current.Offset),
			})
		}
	}
	return checks
}

func MinimumFlashSize(parts []Partition) uint64 {
	var highestEnd uint64
	for _, part := range parts {
		end := uint64(part.Offset) + uint64(part.Size)
		if end > highestEnd {
			highestEnd = end
		}
	}
	if highestEnd == 0 {
		return 0
	}
	for _, megabytes := range []uint64{1, 2, 4, 8, 16, 32, 64, 128} {
		capacity := megabytes * 1024 * 1024
		if highestEnd <= capacity {
			return capacity
		}
	}
	return highestEnd
}

func allErased(raw []byte) bool {
	return len(raw) > 0 && bytes.Count(raw, []byte{0xFF}) == len(raw)
}

func partitionName(part Partition) string {
	if part.Label != "" {
		return part.Label
	}
	return fmt.Sprintf("%#x", part.Offset)
}

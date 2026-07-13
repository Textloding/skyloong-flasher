package preflight

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Textloding/skyloong-flasher/internal/packagekit"
)

func InspectFirmware(analysis *packagekit.Analysis) FirmwareInspection {
	inspection := FirmwareInspection{
		Checks:       make([]CheckResult, 0),
		RawTechnical: make([]string, 0),
	}
	if analysis == nil {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:    "firmware_analysis_missing",
			Status:  StatusError,
			Title:   "尚未解析固件包",
			Summary: "请先选择并解析一个固件压缩包",
		})
		return inspection
	}

	inspection.Requirements = FirmwareRequirements{
		Chip:               strings.TrimSpace(analysis.Chip),
		HardwareVersion:    strings.TrimSpace(analysis.HardwareVersion),
		Display:            strings.TrimSpace(analysis.Display),
		MinimumFlashBytes:  analysis.MinimumFlashBytes,
		RequiredPSRAMBytes: analysis.MinimumPSRAMBytes,
		RequiresOctalPSRAM: strings.EqualFold(strings.TrimSpace(analysis.PSRAMMode), "octal"),
		FlashFiles:         make([]FlashRegion, 0, len(analysis.FlashFiles)),
	}
	inspection.RawTechnical = append(inspection.RawTechnical,
		fmt.Sprintf("chip=%q hardware=%q display=%q", analysis.Chip, analysis.HardwareVersion, analysis.Display),
		fmt.Sprintf("metadata flash=%d psram=%d psram_mode=%q", analysis.MinimumFlashBytes, analysis.MinimumPSRAMBytes, analysis.PSRAMMode),
	)

	filesByOffset := make(map[uint64]packagekit.FlashFile, len(analysis.FlashFiles))
	for _, file := range analysis.FlashFiles {
		offset, err := parseFlashOffset(file.Offset)
		if err != nil {
			inspection.Checks = append(inspection.Checks, CheckResult{
				Code:      "flash_file_offset",
				Status:    StatusWarning,
				Title:     "固件写入地址无效",
				Summary:   fmt.Sprintf("文件 %s 的写入地址无法识别", file.Path),
				Technical: err.Error(),
			})
			continue
		}
		if file.Size < 0 {
			inspection.Checks = append(inspection.Checks, CheckResult{
				Code:      "flash_file_size",
				Status:    StatusWarning,
				Title:     "固件文件大小无效",
				Summary:   fmt.Sprintf("文件 %s 的大小不能为负数", file.Path),
				Technical: fmt.Sprintf("size=%d", file.Size),
			})
			continue
		}

		region := FlashRegion{Offset: offset, Size: uint64(file.Size), Path: file.Path}
		inspection.Requirements.FlashFiles = append(inspection.Requirements.FlashFiles, region)
		filesByOffset[offset] = file
		inspection.RawTechnical = append(inspection.RawTechnical,
			fmt.Sprintf("flash file offset=%#x size=%#x path=%s", offset, file.Size, file.Path),
		)
	}

	parts, partitionOK := inspectPartitionTable(&inspection, filesByOffset[0x8000])
	if partitionOK {
		inspection.Requirements.Partitions = parts
		partitionMinimum := MinimumFlashSize(parts)
		if partitionMinimum > inspection.Requirements.MinimumFlashBytes {
			inspection.Requirements.MinimumFlashBytes = partitionMinimum
		}
		partitionChecks := ValidatePartitions(parts)
		if len(partitionChecks) == 0 {
			inspection.Checks = append(inspection.Checks, CheckResult{
				Code:    "partition_layout",
				Status:  StatusPass,
				Title:   "固件分区布局有效",
				Summary: "分区之间没有地址重叠",
			})
		} else {
			inspection.Checks = append(inspection.Checks, partitionChecks...)
		}
	}

	checkRequiredFlashFiles(&inspection, filesByOffset, parts, partitionOK)
	checkFlashRegionOverlap(&inspection)
	if partitionOK {
		checkAppImageSize(&inspection, filesByOffset, parts)
	}
	checkFirmwareMetadata(&inspection, analysis)

	return inspection
}

func inspectPartitionTable(inspection *FirmwareInspection, table packagekit.FlashFile) ([]Partition, bool) {
	if table.Path == "" {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "partition_table",
			Status:    StatusWarning,
			Title:     "固件包缺少分区表",
			Summary:   "没有找到写入 0x8000 的分区表文件",
			Technical: "required offset 0x8000",
		})
		return nil, false
	}
	raw, err := os.ReadFile(table.Path)
	if err != nil {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "partition_table",
			Status:    StatusWarning,
			Title:     "无法读取固件分区表",
			Summary:   "分区表文件不存在或当前不可读取",
			Technical: err.Error(),
		})
		inspection.RawTechnical = append(inspection.RawTechnical, "partition table read failed: "+err.Error())
		return nil, false
	}
	parts, err := ParsePartitionTable(raw)
	if err != nil {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "partition_table",
			Status:    StatusWarning,
			Title:     "固件分区表无法解析",
			Summary:   "分区表可能已损坏，或不属于这个固件包",
			Technical: err.Error(),
		})
		inspection.RawTechnical = append(inspection.RawTechnical, "partition table parse failed: "+err.Error())
		return nil, false
	}
	if len(parts) == 0 {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "partition_table",
			Status:    StatusWarning,
			Title:     "固件分区表为空",
			Summary:   "分区表没有包含任何有效分区",
			Technical: "partition count=0",
		})
		inspection.RawTechnical = append(inspection.RawTechnical, "partition table parse failed: no entries")
		return nil, false
	}
	inspection.Checks = append(inspection.Checks, CheckResult{
		Code:    "partition_table",
		Status:  StatusPass,
		Title:   "固件分区表可读取",
		Summary: fmt.Sprintf("已解析 %d 个分区", len(parts)),
	})
	inspection.RawTechnical = append(inspection.RawTechnical, fmt.Sprintf("partition count=%d", len(parts)))
	return parts, true
}

func checkRequiredFlashFiles(inspection *FirmwareInspection, files map[uint64]packagekit.FlashFile, parts []Partition, partitionOK bool) {
	required := []uint64{0x0, 0x8000, 0x10000}
	if partitionOK {
		if app, ok := firstAppPartition(parts); ok {
			required = append(required, uint64(app.Offset))
		}
	}

	missing := make([]string, 0)
	for _, offset := range required {
		if _, ok := files[offset]; !ok {
			missing = append(missing, hexOffset(offset))
		}
	}
	if len(missing) > 0 {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "missing_flash_file",
			Status:    StatusWarning,
			Title:     "固件包文件不完整",
			Summary:   "缺少必需写入位置对应的固件文件",
			Technical: "missing offsets: " + strings.Join(missing, ", "),
		})
		return
	}
	inspection.Checks = append(inspection.Checks, CheckResult{
		Code:    "required_flash_files",
		Status:  StatusPass,
		Title:   "固件包文件完整",
		Summary: "Bootloader、分区表、OTA 数据和应用镜像均已找到",
	})
}

func checkFlashRegionOverlap(inspection *FirmwareInspection) {
	regions := append([]FlashRegion(nil), inspection.Requirements.FlashFiles...)
	sort.SliceStable(regions, func(i, j int) bool { return regions[i].Offset < regions[j].Offset })
	found := false
	for index := 1; index < len(regions); index++ {
		previous := regions[index-1]
		current := regions[index]
		previousEnd := previous.Offset + previous.Size
		if current.Offset >= previousEnd {
			continue
		}
		found = true
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "flash_region_overlap",
			Status:    StatusWarning,
			Title:     "固件文件写入范围重叠",
			Summary:   "多个固件文件会写入同一段 Flash 地址",
			Technical: fmt.Sprintf("%s ends at %#x; %s starts at %#x", previous.Path, previousEnd, current.Path, current.Offset),
		})
	}
	if !found {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:    "flash_region_layout",
			Status:  StatusPass,
			Title:   "固件写入布局有效",
			Summary: "固件文件写入范围没有重叠",
		})
	}
}

func checkAppImageSize(inspection *FirmwareInspection, files map[uint64]packagekit.FlashFile, parts []Partition) {
	app, ok := firstAppPartition(parts)
	if !ok {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "app_partition",
			Status:    StatusWarning,
			Title:     "分区表缺少应用分区",
			Summary:   "没有找到可容纳主固件的 app 分区",
			Technical: "no partition with type 0x00",
		})
		return
	}
	file, ok := files[uint64(app.Offset)]
	if !ok {
		return
	}
	if file.Size > int64(app.Size) {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "app_image_too_large",
			Status:    StatusWarning,
			Title:     "应用固件超过分区容量",
			Summary:   "主固件文件无法完整放入第一个应用分区",
			Technical: fmt.Sprintf("image=%#x partition=%#x offset=%#x", file.Size, app.Size, app.Offset),
		})
		return
	}
	inspection.Checks = append(inspection.Checks, CheckResult{
		Code:    "app_image_size",
		Status:  StatusPass,
		Title:   "应用固件大小有效",
		Summary: "主固件文件可以放入应用分区",
	})
}

func checkFirmwareMetadata(inspection *FirmwareInspection, analysis *packagekit.Analysis) {
	missing := make([]string, 0)
	if strings.TrimSpace(analysis.HardwareVersion) == "" {
		missing = append(missing, "hardware_version")
	}
	if strings.TrimSpace(analysis.Display) == "" {
		missing = append(missing, "display")
	}
	if analysis.MinimumFlashBytes == 0 {
		missing = append(missing, "minimum_flash_bytes")
	}
	if analysis.MinimumPSRAMBytes == 0 {
		missing = append(missing, "minimum_psram_bytes")
	}
	if strings.TrimSpace(analysis.PSRAMMode) == "" {
		missing = append(missing, "psram_mode")
	}
	if len(missing) > 0 {
		inspection.Checks = append(inspection.Checks, CheckResult{
			Code:      "firmware_metadata",
			Status:    StatusUnknown,
			Title:     "固件硬件要求不完整",
			Summary:   "固件包没有声明全部硬件版本和内存要求",
			Technical: "missing metadata: " + strings.Join(missing, ", "),
		})
		return
	}
	inspection.Checks = append(inspection.Checks, CheckResult{
		Code:    "firmware_metadata",
		Status:  StatusPass,
		Title:   "固件硬件要求已声明",
		Summary: "固件包包含硬件版本、Flash 和 PSRAM 要求",
	})
}

func firstAppPartition(parts []Partition) (Partition, bool) {
	var first Partition
	found := false
	for _, part := range parts {
		if part.Type != 0x00 || (found && part.Offset >= first.Offset) {
			continue
		}
		first = part
		found = true
	}
	return first, found
}

func parseFlashOffset(value string) (uint64, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "0x")
	if normalized == "" {
		return 0, fmt.Errorf("empty flash offset %q", value)
	}
	offset, err := strconv.ParseUint(normalized, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid flash offset %q: %w", value, err)
	}
	return offset, nil
}

func hexOffset(offset uint64) string {
	return fmt.Sprintf("0x%x", offset)
}

package preflight

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Textloding/skyloong-flasher/internal/packagekit"
)

func TestWindowsFilesystemPathNormalization(t *testing.T) {
	if runtime.GOOS != "windows" {
		path := "/tmp/firmware/partition-table.bin"
		if got := windowsFilesystemPath(path); got != path {
			t.Fatalf("windowsFilesystemPath(%q) = %q, want unchanged path", path, got)
		}
		return
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "drive path",
			path: `C:\firmware\partition-table.bin`,
			want: `\\?\C:\firmware\partition-table.bin`,
		},
		{
			name: "UNC path",
			path: `\\server\share\partition-table.bin`,
			want: `\\?\UNC\server\share\partition-table.bin`,
		},
		{
			name: "already extended",
			path: `\\?\C:\firmware\partition-table.bin`,
			want: `\\?\C:\firmware\partition-table.bin`,
		},
		{
			name: "relative path",
			path: `firmware\partition-table.bin`,
			want: `firmware\partition-table.bin`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := windowsFilesystemPath(test.path); got != test.want {
				t.Fatalf("windowsFilesystemPath(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func completeFirmwareAnalysis(t *testing.T) *packagekit.Analysis {
	t.Helper()
	root := t.TempDir()
	partitionRaw := partitionTable(validPartitionFixtures()...)

	files := []struct {
		offset string
		name   string
		size   int64
		raw    []byte
	}{
		{offset: "0x0", name: "bootloader.bin", size: 0x7000},
		{offset: "0x8000", name: "partition-table.bin", size: int64(len(partitionRaw)), raw: partitionRaw},
		{offset: "0x10000", name: "ota_data_initial.bin", size: 0x2000},
		{offset: "0x20000", name: "firmware.bin", size: 0x400000},
	}

	flashFiles := make([]packagekit.FlashFile, 0, len(files))
	for _, file := range files {
		path := filepath.Join(root, file.name)
		raw := file.raw
		if raw == nil {
			raw = []byte{0}
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", file.name, err)
		}
		flashFiles = append(flashFiles, packagekit.FlashFile{
			Offset: file.offset,
			Path:   path,
			Size:   file.size,
		})
	}

	return &packagekit.Analysis{
		CanFlash:          true,
		Chip:              "esp32s3",
		HardwareVersion:   "SCM_V4.0",
		Display:           "320x240-st7789-8bit-parallel",
		MinimumFlashBytes: 16 * 1024 * 1024,
		MinimumPSRAMBytes: 8 * 1024 * 1024,
		PSRAMMode:         "octal",
		FlashFiles:        flashFiles,
	}
}

func TestInspectFirmwareReadsRequirementsAndCompleteLayout(t *testing.T) {
	analysis := completeFirmwareAnalysis(t)
	inspection := InspectFirmware(analysis)

	want := FirmwareRequirements{
		Chip:               "esp32s3",
		HardwareVersion:    "SCM_V4.0",
		Display:            "320x240-st7789-8bit-parallel",
		MinimumFlashBytes:  16 * 1024 * 1024,
		RequiredPSRAMBytes: 8 * 1024 * 1024,
		RequiresOctalPSRAM: true,
	}
	if inspection.Requirements.Chip != want.Chip ||
		inspection.Requirements.HardwareVersion != want.HardwareVersion ||
		inspection.Requirements.Display != want.Display ||
		inspection.Requirements.MinimumFlashBytes != want.MinimumFlashBytes ||
		inspection.Requirements.RequiredPSRAMBytes != want.RequiredPSRAMBytes ||
		inspection.Requirements.RequiresOctalPSRAM != want.RequiresOctalPSRAM {
		t.Fatalf("requirements = %#v, want metadata %#v", inspection.Requirements, want)
	}
	if len(inspection.Requirements.Partitions) != 4 || len(inspection.Requirements.FlashFiles) != 4 {
		t.Fatalf("requirements layout = %#v", inspection.Requirements)
	}
	if len(inspection.RawTechnical) == 0 {
		t.Fatal("RawTechnical is empty")
	}
	if checksWithStatus(inspection.Checks, StatusWarning, StatusError, StatusUnknown) != 0 {
		t.Fatalf("unexpected non-pass checks: %#v", inspection.Checks)
	}
}

func TestInspectFirmwareChecksEveryRequiredOffset(t *testing.T) {
	tests := []struct {
		name   string
		offset uint64
	}{
		{name: "bootloader", offset: 0x0},
		{name: "partition table", offset: 0x8000},
		{name: "OTA data", offset: 0x10000},
		{name: "first app partition", offset: 0x20000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analysis := completeFirmwareAnalysis(t)
			analysis.FlashFiles = removeFlashFile(analysis.FlashFiles, test.offset)

			inspection := InspectFirmware(analysis)
			check, ok := findCheck(inspection.Checks, "missing_flash_file")
			if !ok || check.Status != StatusWarning || !strings.Contains(check.Technical, formatOffset(test.offset)) {
				t.Fatalf("checks = %#v, want missing_flash_file warning for %#x", inspection.Checks, test.offset)
			}
		})
	}
}

func TestInspectFirmwareReportsOverlappingWriteRegions(t *testing.T) {
	analysis := completeFirmwareAnalysis(t)
	analysis.FlashFiles[0].Size = 0x9000

	inspection := InspectFirmware(analysis)
	check, ok := findCheck(inspection.Checks, "flash_region_overlap")
	if !ok || check.Status != StatusWarning {
		t.Fatalf("checks = %#v, want flash_region_overlap warning", inspection.Checks)
	}
}

func TestCheckFlashRegionOverlapReportsAddressOverflow(t *testing.T) {
	inspection := FirmwareInspection{
		Requirements: FirmwareRequirements{
			FlashFiles: []FlashRegion{
				{Offset: ^uint64(0) - 0xF, Size: 0x20, Path: "overflow.bin"},
				{Offset: 0x10, Size: 0x10, Path: "low-address.bin"},
			},
		},
	}

	checkFlashRegionOverlap(&inspection)

	check, ok := findCheck(inspection.Checks, "flash_region_overflow")
	if !ok || check.Status != StatusWarning {
		t.Fatalf("checks = %#v, want flash_region_overflow warning", inspection.Checks)
	}
	if _, ok := findCheck(inspection.Checks, "flash_region_layout"); ok {
		t.Fatalf("checks = %#v, overflow must not produce layout pass", inspection.Checks)
	}
}

func TestInspectFirmwareReportsAppImageLargerThanPartition(t *testing.T) {
	analysis := completeFirmwareAnalysis(t)
	for index := range analysis.FlashFiles {
		if analysis.FlashFiles[index].Offset == "0x20000" {
			analysis.FlashFiles[index].Size = 0x500001
		}
	}

	inspection := InspectFirmware(analysis)
	check, ok := findCheck(inspection.Checks, "app_image_too_large")
	if !ok || check.Status != StatusWarning || !strings.Contains(check.Technical, "0x500001") {
		t.Fatalf("checks = %#v, want app_image_too_large warning", inspection.Checks)
	}
}

func TestInspectFirmwareTreatsMissingMetadataAsUnknown(t *testing.T) {
	analysis := completeFirmwareAnalysis(t)
	analysis.HardwareVersion = ""
	analysis.Display = ""
	analysis.MinimumFlashBytes = 0
	analysis.MinimumPSRAMBytes = 0
	analysis.PSRAMMode = ""

	inspection := InspectFirmware(analysis)
	check, ok := findCheck(inspection.Checks, "firmware_metadata")
	if !ok || check.Status != StatusUnknown {
		t.Fatalf("checks = %#v, want firmware_metadata unknown", inspection.Checks)
	}
	if inspection.Requirements.MinimumFlashBytes != 16*1024*1024 {
		t.Fatalf("partition-derived minimum Flash = %#x, want 16MB", inspection.Requirements.MinimumFlashBytes)
	}
}

func TestInspectFirmwareReportsMissingOrUnparseablePartitionTable(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		analysis := completeFirmwareAnalysis(t)
		analysis.FlashFiles = removeFlashFile(analysis.FlashFiles, 0x8000)
		inspection := InspectFirmware(analysis)
		check, ok := findCheck(inspection.Checks, "partition_table")
		if !ok || check.Status != StatusWarning {
			t.Fatalf("checks = %#v, want partition_table warning", inspection.Checks)
		}
	})

	t.Run("unparseable", func(t *testing.T) {
		analysis := completeFirmwareAnalysis(t)
		for index := range analysis.FlashFiles {
			if analysis.FlashFiles[index].Offset != "0x8000" {
				continue
			}
			if err := os.WriteFile(analysis.FlashFiles[index].Path, []byte{0x34, 0x12}, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		inspection := InspectFirmware(analysis)
		check, ok := findCheck(inspection.Checks, "partition_table")
		if !ok || check.Status != StatusWarning || check.Technical == "" {
			t.Fatalf("checks = %#v, want partition_table warning with technical detail", inspection.Checks)
		}
	})

	t.Run("empty", func(t *testing.T) {
		analysis := completeFirmwareAnalysis(t)
		for index := range analysis.FlashFiles {
			if analysis.FlashFiles[index].Offset != "0x8000" {
				continue
			}
			if err := os.WriteFile(analysis.FlashFiles[index].Path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		inspection := InspectFirmware(analysis)
		check, ok := findCheck(inspection.Checks, "partition_table")
		if !ok || check.Status != StatusWarning {
			t.Fatalf("checks = %#v, want empty partition_table warning", inspection.Checks)
		}
	})
}

func TestInspectFirmwareDoesNotMutateAnalysis(t *testing.T) {
	analysis := completeFirmwareAnalysis(t)
	wantCanFlash := analysis.CanFlash
	wantFiles := append([]packagekit.FlashFile(nil), analysis.FlashFiles...)

	_ = InspectFirmware(analysis)

	if analysis.CanFlash != wantCanFlash {
		t.Fatalf("CanFlash changed from %v to %v", wantCanFlash, analysis.CanFlash)
	}
	if !reflect.DeepEqual(analysis.FlashFiles, wantFiles) {
		t.Fatalf("FlashFiles mutated:\n got %#v\nwant %#v", analysis.FlashFiles, wantFiles)
	}
}

func removeFlashFile(files []packagekit.FlashFile, offset uint64) []packagekit.FlashFile {
	want := formatOffset(offset)
	result := make([]packagekit.FlashFile, 0, len(files)-1)
	for _, file := range files {
		parsed, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(file.Offset), "0x"), 16, 64)
		if err == nil && formatOffset(parsed) == want {
			continue
		}
		result = append(result, file)
	}
	return result
}

func findCheck(checks []CheckResult, code string) (CheckResult, bool) {
	for _, check := range checks {
		if check.Code == code {
			return check, true
		}
	}
	return CheckResult{}, false
}

func checksWithStatus(checks []CheckResult, statuses ...string) int {
	wanted := make(map[string]bool, len(statuses))
	for _, status := range statuses {
		wanted[status] = true
	}
	count := 0
	for _, check := range checks {
		if wanted[check.Status] {
			count++
		}
	}
	return count
}

func formatOffset(offset uint64) string {
	return "0x" + strconv.FormatUint(offset, 16)
}

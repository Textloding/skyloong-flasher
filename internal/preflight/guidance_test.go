package preflight

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompareAddsGuidanceToCompatibilityChecks(t *testing.T) {
	tests := []struct {
		name     string
		firmware FirmwareInspection
		device   DeviceProbe
		code     string
	}{
		{
			name:     "chip mismatch",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{Chip: "ESP32-S3"}},
			device:   DeviceProbe{Capabilities: DeviceCapabilities{Chip: "ESP32-C3"}},
			code:     "chip_mismatch",
		},
		{
			name:     "flash too small",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{MinimumFlashBytes: 16 * 1024 * 1024}},
			device:   DeviceProbe{Capabilities: DeviceCapabilities{FlashBytes: 8 * 1024 * 1024}},
			code:     "flash_too_small",
		},
		{
			name:     "psram too small",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{RequiredPSRAMBytes: 8 * 1024 * 1024}},
			device:   DeviceProbe{Capabilities: DeviceCapabilities{PSRAMKnown: true, PSRAMBytes: 2 * 1024 * 1024}},
			code:     "psram_too_small",
		},
		{
			name:     "psram unknown",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{RequiredPSRAMBytes: 8 * 1024 * 1024}},
			device:   DeviceProbe{Capabilities: DeviceCapabilities{PSRAMKnown: false}},
			code:     "psram_unknown",
		},
		{
			name: "psram mode mismatch",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{
				RequiredPSRAMBytes: 8 * 1024 * 1024,
				RequiresOctalPSRAM: true,
			}},
			device: DeviceProbe{Capabilities: DeviceCapabilities{PSRAMKnown: true, PSRAMBytes: 8 * 1024 * 1024, PSRAMMode: "quad"}},
			code:   "psram_mode_mismatch",
		},
		{
			name:   "secure boot enabled",
			device: DeviceProbe{Capabilities: DeviceCapabilities{SecureBoot: TriStateEnabled}},
			code:   "secure_boot_enabled",
		},
		{
			name:   "flash encryption enabled",
			device: DeviceProbe{Capabilities: DeviceCapabilities{FlashEncryption: TriStateEnabled}},
			code:   "flash_encryption_enabled",
		},
		{
			name:     "hardware unknown",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{HardwareVersion: "V4.0"}},
			code:     "hardware_unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Compare(tt.firmware, tt.device)
			check, ok := findCheck(report.Checks, tt.code)
			if !ok {
				t.Fatalf("checks = %#v, want %s", report.Checks, tt.code)
			}
			assertChineseGuidance(t, check)
		})
	}
}

func TestFlashGuidanceIncludesActualAndRequiredCapacity(t *testing.T) {
	report := Compare(
		FirmwareInspection{Requirements: FirmwareRequirements{MinimumFlashBytes: 16 * 1024 * 1024}},
		DeviceProbe{Capabilities: DeviceCapabilities{FlashBytes: 8 * 1024 * 1024}},
	)
	check, _ := findCheck(report.Checks, "flash_too_small")

	if !strings.Contains(check.Resolution, "8 MB") || !strings.Contains(check.Resolution, "16 MB") {
		t.Errorf("Resolution = %q, want actual and required capacities", check.Resolution)
	}
	if strings.Contains(check.Resolution, "降低波特率") && !strings.Contains(check.Resolution, "不能") {
		t.Errorf("Resolution = %q, must not imply baud rate can fix capacity", check.Resolution)
	}
}

func TestV4HardwareGuidanceExplainsWiringAndSilkscreen(t *testing.T) {
	report := Compare(
		FirmwareInspection{Requirements: FirmwareRequirements{HardwareVersion: "V4.0"}},
		DeviceProbe{},
	)
	check, _ := findCheck(report.Checks, "hardware_unknown")
	steps := strings.Join(check.Steps, "\n")

	for _, want := range []string{"V3/V4 显示接线不同", "反复刷机不能修复接线差异", "不能自动区分"} {
		if !strings.Contains(check.Resolution, want) {
			t.Errorf("Resolution = %q, want %q", check.Resolution, want)
		}
	}
	for _, want := range []string{"主板丝印", "V4.0", "对应的固件包"} {
		if !strings.Contains(steps, want) {
			t.Errorf("Steps = %#v, want %q", check.Steps, want)
		}
	}
	if strings.Contains(check.Resolution, "USB ID 能自动区分") {
		t.Errorf("Resolution = %q, must not claim USB ID distinguishes board revisions", check.Resolution)
	}
}

func TestGuidancePreservesExistingTechnicalDetail(t *testing.T) {
	const technical = "firmware chip=esp32s3 device chip=esp32c3"
	report := Compare(
		FirmwareInspection{Checks: []CheckResult{{
			Code:      "chip_mismatch",
			Status:    StatusWarning,
			Technical: technical,
		}}},
		DeviceProbe{},
	)
	check, _ := findCheck(report.Checks, "chip_mismatch")

	if check.Technical != technical {
		t.Errorf("Technical = %q, want unchanged %q", check.Technical, technical)
	}
}

func TestDownloadAndConnectionGuidanceUsesExactOrderedSteps(t *testing.T) {
	want := []string{
		"断开屏幕的 USB 连接",
		"按住 BOOT/下载键并重新插入 USB",
		"看到新的 COM 串口后松开 BOOT/下载键",
		"返回工具点击“重新检测”",
	}
	for _, code := range []string{"connection_failed", "download_mode_required"} {
		t.Run(code, func(t *testing.T) {
			report := Compare(FirmwareInspection{}, DeviceProbe{Checks: []CheckResult{{
				Code:   code,
				Status: StatusError,
			}}})
			check, _ := findCheck(report.Checks, code)

			if !reflect.DeepEqual(check.Steps, want) {
				t.Errorf("Steps = %#v, want %#v", check.Steps, want)
			}
			assertChineseGuidance(t, check)
		})
	}
}

func TestPortBusyGuidanceNamesCommonSerialPrograms(t *testing.T) {
	report := Compare(FirmwareInspection{}, DeviceProbe{Checks: []CheckResult{{
		Code:   "port_busy",
		Status: StatusError,
	}}})
	check, _ := findCheck(report.Checks, "port_busy")
	steps := strings.Join(check.Steps, "\n")

	for _, want := range []string{"串口助手", "Arduino/PlatformIO监视器", "重新检测"} {
		if !strings.Contains(steps, want) {
			t.Errorf("Steps = %#v, want %q", check.Steps, want)
		}
	}
	assertChineseGuidance(t, check)
}

func TestUnknownCapabilityCodesReceiveSpecificGuidance(t *testing.T) {
	tests := []struct {
		name     string
		firmware FirmwareInspection
		device   DeviceProbe
		code     string
	}{
		{name: "chip", firmware: FirmwareInspection{Requirements: FirmwareRequirements{Chip: "ESP32-S3"}}, code: "chip_unknown"},
		{name: "flash", firmware: FirmwareInspection{Requirements: FirmwareRequirements{MinimumFlashBytes: 16 * 1024 * 1024}}, code: "flash_unknown"},
		{
			name: "psram mode",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{
				RequiredPSRAMBytes: 8 * 1024 * 1024,
				RequiresOctalPSRAM: true,
			}},
			device: DeviceProbe{Capabilities: DeviceCapabilities{PSRAMKnown: true, PSRAMBytes: 8 * 1024 * 1024}},
			code:   "psram_mode_unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Compare(tt.firmware, tt.device)
			check, _ := findCheck(report.Checks, tt.code)
			assertChineseGuidance(t, check)
		})
	}
}

func TestProbeErrorGuidanceCoversStableCodes(t *testing.T) {
	for _, code := range []string{"port_missing", "timeout", "unknown_probe_error"} {
		t.Run(code, func(t *testing.T) {
			report := Compare(FirmwareInspection{}, DeviceProbe{Checks: []CheckResult{{
				Code:      code,
				Status:    StatusError,
				Technical: "original esptool output",
			}}})
			check, _ := findCheck(report.Checks, code)

			assertChineseGuidance(t, check)
			if check.Technical != "original esptool output" {
				t.Errorf("Technical = %q, want original output", check.Technical)
			}
		})
	}
}

func TestUnknownProbeErrorGetsChineseSummaryAndKeepsTechnical(t *testing.T) {
	const technical = "A fatal error occurred: unexpected probe failure"
	report := Compare(FirmwareInspection{}, DeviceProbe{Checks: []CheckResult{{
		Code:      "unknown_probe_error",
		Status:    StatusError,
		Title:     "Unexpected error",
		Summary:   "probe failed for an unknown reason",
		Technical: technical,
	}}})
	check, _ := findCheck(report.Checks, "unknown_probe_error")

	if check.Title != "设备探测失败" || check.Summary != "设备探测失败，当前错误无法归入已知类型" {
		t.Errorf("Title/Summary = %q/%q, want generic Chinese message", check.Title, check.Summary)
	}
	if check.Technical != technical {
		t.Errorf("Technical = %q, want %q", check.Technical, technical)
	}
}

func TestFirmwareAndPartitionNonPassChecksReceiveGuidance(t *testing.T) {
	codes := []struct {
		code   string
		status string
	}{
		{code: "firmware_analysis_missing", status: StatusError},
		{code: "flash_file_offset", status: StatusWarning},
		{code: "flash_file_size", status: StatusWarning},
		{code: "partition_table", status: StatusWarning},
		{code: "missing_flash_file", status: StatusWarning},
		{code: "flash_region_overflow", status: StatusWarning},
		{code: "flash_region_overlap", status: StatusWarning},
		{code: "partition_overlap", status: StatusWarning},
		{code: "app_partition", status: StatusWarning},
		{code: "app_image_too_large", status: StatusWarning},
		{code: "firmware_metadata", status: StatusUnknown},
	}
	checks := make([]CheckResult, 0, len(codes))
	for _, item := range codes {
		checks = append(checks, CheckResult{
			Code:      item.code,
			Status:    item.status,
			Summary:   "untranslated low-level detail",
			Technical: "technical for " + item.code,
		})
	}

	report := Compare(FirmwareInspection{Checks: checks}, DeviceProbe{})
	for _, item := range codes {
		t.Run(item.code, func(t *testing.T) {
			check, _ := findCheck(report.Checks, item.code)
			assertChineseGuidance(t, check)
			if check.Technical != "technical for "+item.code {
				t.Errorf("Technical = %q, want original detail", check.Technical)
			}
		})
	}

	missing, _ := findCheck(report.Checks, "missing_flash_file")
	if joined := strings.Join(missing.Steps, "\n"); !strings.Contains(joined, "Bootloader、分区表、OTA 数据和应用镜像") {
		t.Errorf("missing_flash_file Steps = %#v, want complete package contents", missing.Steps)
	}
}

func TestUnknownSecurityStatesReceiveGuidance(t *testing.T) {
	report := Compare(FirmwareInspection{}, DeviceProbe{Capabilities: DeviceCapabilities{
		SecureBoot:      TriStateUnknown,
		FlashEncryption: TriStateUnknown,
	}})
	for _, code := range []string{"secure_boot_unknown", "flash_encryption_unknown"} {
		check, _ := findCheck(report.Checks, code)
		assertChineseGuidance(t, check)
	}
}

func TestHardwareMismatchUsesV4WiringGuidance(t *testing.T) {
	report := Compare(FirmwareInspection{
		Requirements: FirmwareRequirements{HardwareVersion: "V4.0"},
		Checks: []CheckResult{{
			Code:      "hardware_mismatch",
			Status:    StatusWarning,
			Technical: "firmware=V4.0 board=V3.0",
		}},
	}, DeviceProbe{})
	check, _ := findCheck(report.Checks, "hardware_mismatch")

	for _, want := range []string{"V3/V4 显示接线不同", "反复刷机不能修复接线差异"} {
		if !strings.Contains(check.Resolution, want) {
			t.Errorf("Resolution = %q, want %q", check.Resolution, want)
		}
	}
	if joined := strings.Join(check.Steps, "\n"); !strings.Contains(joined, "主板丝印") || !strings.Contains(joined, "对应的固件包") {
		t.Errorf("Steps = %#v, want silkscreen and matching-package guidance", check.Steps)
	}
}

func TestFallbackGuidanceCoversEveryNonPassStatus(t *testing.T) {
	for _, status := range []string{StatusWarning, StatusUnknown, StatusError} {
		t.Run(status, func(t *testing.T) {
			code := "future_" + status
			report := Compare(FirmwareInspection{Checks: []CheckResult{{
				Code:   code,
				Status: status,
			}}}, DeviceProbe{})
			check, _ := findCheck(report.Checks, code)
			assertChineseGuidance(t, check)
		})
	}
}

func assertChineseGuidance(t *testing.T, check CheckResult) {
	t.Helper()
	if strings.TrimSpace(check.Resolution) == "" {
		t.Errorf("%s Resolution is empty", check.Code)
	}
	if len(check.Steps) == 0 {
		t.Errorf("%s Steps are empty", check.Code)
	}
	if check.Status != StatusPass && isASCII(check.Resolution) {
		t.Errorf("%s Resolution = %q, want Chinese guidance", check.Code, check.Resolution)
	}
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}

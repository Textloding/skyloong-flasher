package preflight

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompareNormalizesEquivalentChipNames(t *testing.T) {
	firmware := FirmwareInspection{
		Requirements: FirmwareRequirements{Chip: "esp32s3"},
	}
	device := DeviceProbe{
		Capabilities: DeviceCapabilities{
			Chip:            "ESP32-S3",
			SecureBoot:      TriStateDisabled,
			FlashEncryption: TriStateDisabled,
		},
	}

	report := Compare(firmware, device)

	check, ok := findCheck(report.Checks, "chip_compatible")
	if !ok || check.Status != StatusPass {
		t.Fatalf("checks = %#v, want chip_compatible pass", report.Checks)
	}
	if report.Overall != "compatible" {
		t.Errorf("Overall = %q, want compatible", report.Overall)
	}
}

func TestCompareReportsChipMismatch(t *testing.T) {
	firmware := FirmwareInspection{
		Requirements: FirmwareRequirements{Chip: "ESP32-S3"},
	}
	device := DeviceProbe{
		Capabilities: DeviceCapabilities{Chip: "ESP32-C3"},
	}

	report := Compare(firmware, device)

	check, ok := findCheck(report.Checks, "chip_mismatch")
	if !ok || check.Status != StatusWarning {
		t.Fatalf("checks = %#v, want chip_mismatch warning", report.Checks)
	}
	if report.Overall != "risk" {
		t.Errorf("Overall = %q, want risk", report.Overall)
	}
}

func TestCompareMergesChecksFieldsAndRawLogsWithoutFlashEligibility(t *testing.T) {
	firmware := FirmwareInspection{
		Requirements: FirmwareRequirements{
			Chip:       "ESP32-S3",
			FlashFiles: []FlashRegion{{Offset: 0x10000, Size: 0x20000, Path: "app.bin"}},
		},
		Checks:       []CheckResult{{Code: "firmware_check", Status: StatusPass}},
		RawTechnical: []string{"firmware raw"},
	}
	device := DeviceProbe{
		Capabilities: DeviceCapabilities{
			Chip:            "ESP32-S3",
			Revision:        "v0.2",
			SecureBoot:      TriStateDisabled,
			FlashEncryption: TriStateDisabled,
		},
		Checks:    []CheckResult{{Code: "device_check", Status: StatusPass}},
		RawOutput: []string{"device raw"},
	}

	report := Compare(firmware, device)

	if !reflect.DeepEqual(report.Firmware, firmware.Requirements) {
		t.Errorf("Firmware = %#v, want %#v", report.Firmware, firmware.Requirements)
	}
	if !reflect.DeepEqual(report.Device, device.Capabilities) {
		t.Errorf("Device = %#v, want %#v", report.Device, device.Capabilities)
	}
	if len(report.Checks) < 2 || report.Checks[0].Code != "firmware_check" || report.Checks[1].Code != "device_check" {
		t.Errorf("Checks = %#v, want firmware and device checks first", report.Checks)
	}
	wantRaw := []string{"firmware raw", "device raw"}
	if !reflect.DeepEqual(report.RawLog, wantRaw) {
		t.Errorf("RawLog = %#v, want %#v", report.RawLog, wantRaw)
	}
	if _, found := reflect.TypeOf(report).FieldByName("CanFlash"); found {
		t.Fatal("Report must not expose a CanFlash mechanism")
	}
}

func TestCompareReportsFlashCapacity(t *testing.T) {
	t.Run("too small", func(t *testing.T) {
		report := Compare(
			FirmwareInspection{Requirements: FirmwareRequirements{MinimumFlashBytes: 16 * 1024 * 1024}},
			DeviceProbe{Capabilities: DeviceCapabilities{FlashBytes: 8 * 1024 * 1024}},
		)

		check, ok := findCheck(report.Checks, "flash_too_small")
		if !ok || check.Status != StatusWarning {
			t.Fatalf("checks = %#v, want flash_too_small warning", report.Checks)
		}
		if !strings.Contains(check.Summary, "8 MB") || !strings.Contains(check.Summary, "16 MB") {
			t.Errorf("Summary = %q, want actual and required capacities", check.Summary)
		}
		if report.Overall != "risk" {
			t.Errorf("Overall = %q, want risk", report.Overall)
		}
	})

	t.Run("sufficient", func(t *testing.T) {
		report := Compare(
			FirmwareInspection{Requirements: FirmwareRequirements{MinimumFlashBytes: 8 * 1024 * 1024}},
			DeviceProbe{Capabilities: DeviceCapabilities{FlashBytes: 16 * 1024 * 1024}},
		)

		check, ok := findCheck(report.Checks, "flash_compatible")
		if !ok || check.Status != StatusPass {
			t.Fatalf("checks = %#v, want flash_compatible pass", report.Checks)
		}
	})
}

func TestCompareReportsPSRAMCapacity(t *testing.T) {
	tests := []struct {
		name       string
		known      bool
		actual     uint64
		wantCode   string
		wantStatus string
	}{
		{name: "sufficient", known: true, actual: 8 * 1024 * 1024, wantCode: "psram_compatible", wantStatus: StatusPass},
		{name: "insufficient", known: true, actual: 2 * 1024 * 1024, wantCode: "psram_too_small", wantStatus: StatusWarning},
		{name: "unknown", known: false, wantCode: "psram_unknown", wantStatus: StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Compare(
				FirmwareInspection{Requirements: FirmwareRequirements{RequiredPSRAMBytes: 8 * 1024 * 1024}},
				DeviceProbe{Capabilities: DeviceCapabilities{PSRAMKnown: tt.known, PSRAMBytes: tt.actual}},
			)

			check, ok := findCheck(report.Checks, tt.wantCode)
			if !ok || check.Status != tt.wantStatus {
				t.Fatalf("checks = %#v, want %s %s", report.Checks, tt.wantCode, tt.wantStatus)
			}
		})
	}
}

func TestCompareReportsOctalPSRAMModeMismatch(t *testing.T) {
	report := Compare(
		FirmwareInspection{Requirements: FirmwareRequirements{
			RequiredPSRAMBytes: 8 * 1024 * 1024,
			RequiresOctalPSRAM: true,
		}},
		DeviceProbe{Capabilities: DeviceCapabilities{
			PSRAMKnown: true,
			PSRAMBytes: 8 * 1024 * 1024,
			PSRAMMode:  "quad",
		}},
	)

	check, ok := findCheck(report.Checks, "psram_mode_mismatch")
	if !ok || check.Status != StatusWarning {
		t.Fatalf("checks = %#v, want psram_mode_mismatch warning", report.Checks)
	}
}

func TestCompareTreatsMissingCapabilitiesAsUnknown(t *testing.T) {
	tests := []struct {
		name     string
		firmware FirmwareInspection
		device   DeviceProbe
		code     string
	}{
		{
			name:     "chip",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{Chip: "ESP32-S3"}},
			code:     "chip_unknown",
		},
		{
			name:     "flash",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{MinimumFlashBytes: 16 * 1024 * 1024}},
			code:     "flash_unknown",
		},
		{
			name: "psram mode",
			firmware: FirmwareInspection{Requirements: FirmwareRequirements{
				RequiredPSRAMBytes: 8 * 1024 * 1024,
				RequiresOctalPSRAM: true,
			}},
			device: DeviceProbe{Capabilities: DeviceCapabilities{
				PSRAMKnown: true,
				PSRAMBytes: 8 * 1024 * 1024,
			}},
			code: "psram_mode_unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Compare(tt.firmware, tt.device)
			check, ok := findCheck(report.Checks, tt.code)
			if !ok || check.Status != StatusUnknown {
				t.Fatalf("checks = %#v, want %s unknown", report.Checks, tt.code)
			}
			if report.Overall != "unknown" {
				t.Errorf("Overall = %q, want unknown", report.Overall)
			}
		})
	}
}

func TestCompareReportsMatchingOctalPSRAMMode(t *testing.T) {
	report := Compare(
		FirmwareInspection{Requirements: FirmwareRequirements{
			RequiredPSRAMBytes: 8 * 1024 * 1024,
			RequiresOctalPSRAM: true,
		}},
		DeviceProbe{Capabilities: DeviceCapabilities{
			PSRAMKnown: true,
			PSRAMBytes: 8 * 1024 * 1024,
			PSRAMMode:  "octal",
		}},
	)

	check, ok := findCheck(report.Checks, "psram_mode_compatible")
	if !ok || check.Status != StatusPass {
		t.Fatalf("checks = %#v, want psram_mode_compatible pass", report.Checks)
	}
}

func TestCompareReportsSecurityStates(t *testing.T) {
	tests := []struct {
		name       string
		secureBoot TriState
		encryption TriState
		wantCode   string
		wantStatus string
	}{
		{name: "secure boot enabled", secureBoot: TriStateEnabled, encryption: TriStateDisabled, wantCode: "secure_boot_enabled", wantStatus: StatusWarning},
		{name: "secure boot disabled", secureBoot: TriStateDisabled, encryption: TriStateDisabled, wantCode: "secure_boot_disabled", wantStatus: StatusPass},
		{name: "secure boot unknown", secureBoot: TriStateUnknown, encryption: TriStateDisabled, wantCode: "secure_boot_unknown", wantStatus: StatusUnknown},
		{name: "flash encryption enabled", secureBoot: TriStateDisabled, encryption: TriStateEnabled, wantCode: "flash_encryption_enabled", wantStatus: StatusWarning},
		{name: "flash encryption disabled", secureBoot: TriStateDisabled, encryption: TriStateDisabled, wantCode: "flash_encryption_disabled", wantStatus: StatusPass},
		{name: "flash encryption unknown", secureBoot: TriStateDisabled, encryption: TriStateUnknown, wantCode: "flash_encryption_unknown", wantStatus: StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Compare(FirmwareInspection{}, DeviceProbe{Capabilities: DeviceCapabilities{
				SecureBoot:      tt.secureBoot,
				FlashEncryption: tt.encryption,
			}})

			check, ok := findCheck(report.Checks, tt.wantCode)
			if !ok || check.Status != tt.wantStatus {
				t.Fatalf("checks = %#v, want %s %s", report.Checks, tt.wantCode, tt.wantStatus)
			}
		})
	}
}

func TestCompareTreatsZeroValueSecurityStatesAsUnknown(t *testing.T) {
	report := Compare(FirmwareInspection{}, DeviceProbe{Capabilities: DeviceCapabilities{}})

	for _, code := range []string{"secure_boot_unknown", "flash_encryption_unknown"} {
		check, ok := findCheck(report.Checks, code)
		if !ok || check.Status != StatusUnknown {
			t.Errorf("checks = %#v, want %s unknown", report.Checks, code)
		}
	}
	if report.Overall != "unknown" {
		t.Errorf("Overall = %q, want unknown", report.Overall)
	}
}

func TestCompareReportsHardwareVersionUnknownForGenericUSBProbe(t *testing.T) {
	report := Compare(
		FirmwareInspection{Requirements: FirmwareRequirements{HardwareVersion: "V4.0"}},
		DeviceProbe{Capabilities: DeviceCapabilities{
			Chip:            "ESP32-S3",
			SecureBoot:      TriStateDisabled,
			FlashEncryption: TriStateDisabled,
		}},
	)

	check, ok := findCheck(report.Checks, "hardware_unknown")
	if !ok || check.Status != StatusUnknown {
		t.Fatalf("checks = %#v, want hardware_unknown unknown", report.Checks)
	}
	if !strings.Contains(check.Summary, "V4.0") || !strings.Contains(check.Summary, "无法") {
		t.Errorf("Summary = %q, want target version and automatic-detection limitation", check.Summary)
	}
	if report.Overall != "unknown" {
		t.Errorf("Overall = %q, want unknown", report.Overall)
	}
}

func TestCompareProbeErrorForcesErrorOverall(t *testing.T) {
	probeCheck := CheckResult{
		Code:      "unknown_probe_error",
		Status:    StatusError,
		Title:     "设备探测失败",
		Summary:   "设备探测失败",
		Technical: "unexpected probe failure",
	}
	report := Compare(
		FirmwareInspection{Checks: []CheckResult{{Code: "firmware_warning", Status: StatusWarning}}},
		DeviceProbe{Checks: []CheckResult{probeCheck}, RawOutput: []string{"probe raw"}},
	)

	if report.Overall != "error" {
		t.Errorf("Overall = %q, want error", report.Overall)
	}
	check, ok := findCheck(report.Checks, "unknown_probe_error")
	if !ok || check.Technical != probeCheck.Technical {
		t.Errorf("checks = %#v, want probe error with original Technical", report.Checks)
	}
}

func TestCompareOverallPriority(t *testing.T) {
	tests := []struct {
		name    string
		checks  []CheckResult
		overall string
	}{
		{name: "error over warning", checks: []CheckResult{{Status: StatusWarning}, {Status: StatusError}}, overall: "error"},
		{name: "warning over unknown", checks: []CheckResult{{Status: StatusUnknown}, {Status: StatusWarning}}, overall: "risk"},
		{name: "unknown over pass", checks: []CheckResult{{Status: StatusPass}, {Status: StatusUnknown}}, overall: "unknown"},
		{name: "all pass", checks: []CheckResult{{Status: StatusPass}}, overall: "compatible"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Compare(FirmwareInspection{Checks: tt.checks}, DeviceProbe{Capabilities: DeviceCapabilities{
				SecureBoot:      TriStateDisabled,
				FlashEncryption: TriStateDisabled,
			}})
			if report.Overall != tt.overall {
				t.Errorf("Overall = %q, want %q", report.Overall, tt.overall)
			}
		})
	}
}

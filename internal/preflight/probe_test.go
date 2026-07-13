package preflight

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

func TestParseEsptoolOutputReadsDeviceCapabilities(t *testing.T) {
	output := strings.Join([]string{
		"Chip is ESP32-S3 (QFN56) (revision v0.2)",
		"Features: WiFi, BLE, Embedded PSRAM 8MB (AP_3v3)",
		"Detected flash size: 16MB",
		"Secure Boot: Disabled",
		"Flash Encryption: Disabled",
	}, "\r\n")

	got := parseEsptoolOutput(output)
	if got.Chip != "ESP32-S3" {
		t.Errorf("Chip = %q, want ESP32-S3", got.Chip)
	}
	if got.Revision != "v0.2" {
		t.Errorf("Revision = %q, want v0.2", got.Revision)
	}
	if got.FlashBytes != 16*1024*1024 {
		t.Errorf("FlashBytes = %d, want %d", got.FlashBytes, 16*1024*1024)
	}
	if got.FlashDescription != "16MB" {
		t.Errorf("FlashDescription = %q, want 16MB", got.FlashDescription)
	}
	if !got.PSRAMKnown || got.PSRAMBytes != 8*1024*1024 {
		t.Errorf("PSRAM known/bytes = %t/%d, want true/%d", got.PSRAMKnown, got.PSRAMBytes, 8*1024*1024)
	}
	if got.PSRAMDescription != "Embedded PSRAM" {
		t.Errorf("PSRAMDescription = %q, want Embedded PSRAM", got.PSRAMDescription)
	}
	if got.PSRAMMode != "octal" {
		t.Errorf("PSRAMMode = %q, want octal", got.PSRAMMode)
	}
	if got.SecureBoot != TriStateDisabled {
		t.Errorf("SecureBoot = %q, want %q", got.SecureBoot, TriStateDisabled)
	}
	if got.FlashEncryption != TriStateDisabled {
		t.Errorf("FlashEncryption = %q, want %q", got.FlashEncryption, TriStateDisabled)
	}
	wantLines := strings.Split(output, "\r\n")
	if !reflect.DeepEqual(got.RawOutput, wantLines) {
		t.Errorf("RawOutput = %#v, want %#v", got.RawOutput, wantLines)
	}
}

func TestParseEsptoolOutputLeavesMissingPSRAMUnknown(t *testing.T) {
	got := parseEsptoolOutput("Features: WiFi, BLE\nDetected flash size: 8MB")

	if got.PSRAMKnown {
		t.Fatal("PSRAMKnown = true, want false when esptool reports no PSRAM details")
	}
	if got.PSRAMBytes != 0 || got.PSRAMDescription != "" || got.PSRAMMode != "" {
		t.Fatalf("unexpected PSRAM values: %+v", got)
	}
	if got.FlashBytes != 8*1024*1024 {
		t.Errorf("FlashBytes = %d, want %d", got.FlashBytes, 8*1024*1024)
	}
	if got.SecureBoot != TriStateUnknown || got.FlashEncryption != TriStateUnknown {
		t.Errorf("security states = %q/%q, want unknown/unknown", got.SecureBoot, got.FlashEncryption)
	}
}

func TestParseEsptoolOutputReadsEnabledSecurityStates(t *testing.T) {
	got := parseEsptoolOutput("Secure Boot: Enabled\nFlash Encryption: Enabled")

	if got.SecureBoot != TriStateEnabled || got.FlashEncryption != TriStateEnabled {
		t.Errorf("security states = %q/%q, want enabled/enabled", got.SecureBoot, got.FlashEncryption)
	}
}

func TestClassifyProbeErrorUsesStableCodesAndChineseMessages(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		err         error
		wantCode    string
		wantSummary string
	}{
		{
			name:        "access denied",
			output:      "PermissionError(13, 'Access is denied.')",
			err:         errors.New("exit status 2"),
			wantCode:    "port_busy",
			wantSummary: "串口正被其他程序占用",
		},
		{
			name:        "port missing",
			output:      "Could not open COM99, the system cannot find the file specified",
			err:         errors.New("exit status 2"),
			wantCode:    "port_missing",
			wantSummary: "没有找到所选串口",
		},
		{
			name:        "failed connection",
			output:      "A fatal error occurred: Failed to connect to ESP32-S3: No serial data received.",
			err:         errors.New("exit status 2"),
			wantCode:    "connection_failed",
			wantSummary: "无法连接设备",
		},
		{
			name:        "wrong boot mode",
			output:      "Wrong boot mode detected (0x13)! The chip needs to be in download mode.",
			err:         errors.New("exit status 2"),
			wantCode:    "download_mode_required",
			wantSummary: "设备未进入下载模式",
		},
		{
			name:        "timeout",
			output:      "Connecting...",
			err:         context.DeadlineExceeded,
			wantCode:    "timeout",
			wantSummary: "设备探测超时",
		},
		{
			name:        "unknown",
			output:      "unexpected probe failure",
			err:         errors.New("exit status 1"),
			wantCode:    "unknown_probe_error",
			wantSummary: "设备探测失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProbeError(tt.output, tt.err)
			if got.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tt.wantCode)
			}
			if got.Status != StatusError {
				t.Errorf("Status = %q, want %q", got.Status, StatusError)
			}
			if got.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", got.Summary, tt.wantSummary)
			}
			if !strings.Contains(got.Technical, tt.output) {
				t.Errorf("Technical = %q, want raw output %q", got.Technical, tt.output)
			}
			if !strings.Contains(got.Technical, tt.err.Error()) {
				t.Errorf("Technical = %q, want error %q", got.Technical, tt.err)
			}
		})
	}
}

func TestProbeDeviceRunsReadOnlyCommandsInOrderWithIndependentTimeouts(t *testing.T) {
	originalRunner := runProbeCommand
	t.Cleanup(func() { runProbeCommand = originalRunner })

	var calls [][]string
	var deadlines []time.Duration
	runProbeCommand = func(ctx context.Context, _ runtimekit.Status, args []string) (probeCommandOutput, error) {
		calls = append(calls, append([]string(nil), args...))
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("probe command context has no deadline")
		}
		deadlines = append(deadlines, time.Until(deadline))
		switch args[len(args)-1] {
		case "flash_id":
			return probeCommandOutput{
				stdout: "Chip is ESP32-S3 (QFN56) (revision v0.2)\nDetected flash size: 16MB",
				stderr: "Features: WiFi, BLE, Embedded PSRAM 8MB (AP_3v3)",
			}, nil
		case "get_security_info":
			return probeCommandOutput{
				stdout: "Secure Boot: Disabled\nFlash Encryption: Disabled",
			}, nil
		default:
			return probeCommandOutput{}, errors.New("unexpected command")
		}
	}

	var logged []string
	probe := ProbeDevice(context.Background(), executableProbeStatus(), "COM7", 460800, func(line string) {
		logged = append(logged, line)
	})

	wantCalls := [][]string{
		{"-p", "COM7", "-b", "460800", "flash_id"},
		{"-p", "COM7", "-b", "460800", "get_security_info"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("probe calls = %#v, want %#v", calls, wantCalls)
	}
	if len(deadlines) != 2 {
		t.Fatalf("deadline count = %d, want 2", len(deadlines))
	}
	for index, remaining := range deadlines {
		if remaining <= 19*time.Second || remaining > 20*time.Second {
			t.Errorf("command %d timeout = %v, want an independent 20s timeout", index, remaining)
		}
	}
	for _, call := range calls {
		joined := strings.ToLower(strings.Join(call, " "))
		if strings.Contains(joined, "erase") || strings.Contains(joined, "write") {
			t.Fatalf("probe used destructive command: %q", joined)
		}
	}
	if probe.Capabilities.Chip != "ESP32-S3" || probe.Capabilities.FlashBytes != 16*1024*1024 {
		t.Errorf("capabilities = %+v, want parsed chip and Flash", probe.Capabilities)
	}
	if probe.Capabilities.PSRAMBytes != 8*1024*1024 || probe.Capabilities.PSRAMMode != "octal" {
		t.Errorf("PSRAM capabilities = %+v", probe.Capabilities)
	}
	if probe.Capabilities.SecureBoot != TriStateDisabled || probe.Capabilities.FlashEncryption != TriStateDisabled {
		t.Errorf("security capabilities = %+v", probe.Capabilities)
	}
	wantRaw := []string{
		"Chip is ESP32-S3 (QFN56) (revision v0.2)",
		"Detected flash size: 16MB",
		"Features: WiFi, BLE, Embedded PSRAM 8MB (AP_3v3)",
		"Secure Boot: Disabled",
		"Flash Encryption: Disabled",
	}
	if !reflect.DeepEqual(probe.RawOutput, wantRaw) {
		t.Errorf("RawOutput = %#v, want %#v", probe.RawOutput, wantRaw)
	}
	if !reflect.DeepEqual(probe.Capabilities.RawOutput, wantRaw) {
		t.Errorf("Capabilities.RawOutput = %#v, want %#v", probe.Capabilities.RawOutput, wantRaw)
	}
	if !reflect.DeepEqual(logged, wantRaw) {
		t.Errorf("logged lines = %#v, want %#v", logged, wantRaw)
	}
	if len(probe.Checks) != 2 || probe.Checks[0].Status != StatusPass || probe.Checks[1].Status != StatusPass {
		t.Errorf("Checks = %+v, want two pass checks", probe.Checks)
	}
}

func TestProbeDeviceRetainsFlashDataWhenSecurityQueryFails(t *testing.T) {
	originalRunner := runProbeCommand
	t.Cleanup(func() { runProbeCommand = originalRunner })

	call := 0
	runProbeCommand = func(_ context.Context, _ runtimekit.Status, args []string) (probeCommandOutput, error) {
		call++
		if args[len(args)-1] == "flash_id" {
			return probeCommandOutput{stdout: "Chip is ESP32-S3 (revision v0.1)\nDetected flash size: 8MB"}, nil
		}
		return probeCommandOutput{stderr: "PermissionError(13, 'Access is denied.')"}, errors.New("exit status 2")
	}

	probe := ProbeDevice(context.Background(), executableProbeStatus(), "COM8", 115200, nil)

	if call != 2 {
		t.Fatalf("runner calls = %d, want 2", call)
	}
	if probe.Capabilities.Chip != "ESP32-S3" || probe.Capabilities.FlashBytes != 8*1024*1024 {
		t.Errorf("capabilities lost after security failure: %+v", probe.Capabilities)
	}
	check, ok := findProbeCheck(probe.Checks, "port_busy")
	if !ok {
		t.Fatalf("Checks = %+v, want port_busy", probe.Checks)
	}
	if !strings.Contains(check.Technical, "Access is denied") {
		t.Errorf("Technical = %q, want security command output", check.Technical)
	}
	if !containsLine(probe.RawOutput, "Detected flash size: 8MB") || !containsLine(probe.RawOutput, "PermissionError(13, 'Access is denied.')") {
		t.Errorf("RawOutput = %#v, want both command outputs", probe.RawOutput)
	}
}

func TestProbeDeviceStopsAfterFlashIDTimeout(t *testing.T) {
	originalRunner := runProbeCommand
	t.Cleanup(func() { runProbeCommand = originalRunner })

	call := 0
	runProbeCommand = func(ctx context.Context, _ runtimekit.Status, _ []string) (probeCommandOutput, error) {
		call++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("probe command context has no deadline")
		}
		return probeCommandOutput{stderr: "Connecting..."}, context.DeadlineExceeded
	}

	probe := ProbeDevice(context.Background(), executableProbeStatus(), "COM9", 0, nil)

	if call != 1 {
		t.Fatalf("runner calls = %d, want 1 after flash_id timeout", call)
	}
	if _, ok := findProbeCheck(probe.Checks, "timeout"); !ok {
		t.Fatalf("Checks = %+v, want timeout", probe.Checks)
	}
}

func TestProbeDeviceRejectsMissingPortWithoutRunningCommand(t *testing.T) {
	originalRunner := runProbeCommand
	t.Cleanup(func() { runProbeCommand = originalRunner })

	runProbeCommand = func(_ context.Context, _ runtimekit.Status, _ []string) (probeCommandOutput, error) {
		t.Fatal("runner should not be called for an empty port")
		return probeCommandOutput{}, nil
	}

	probe := ProbeDevice(context.Background(), executableProbeStatus(), "", 460800, nil)

	check, ok := findProbeCheck(probe.Checks, "port_missing")
	if !ok {
		t.Fatalf("Checks = %+v, want port_missing", probe.Checks)
	}
	if check.Summary != "没有选择设备串口" {
		t.Errorf("Summary = %q, want readable Chinese text", check.Summary)
	}
}

func executableProbeStatus() runtimekit.Status {
	return runtimekit.Status{
		Available: true,
		Kind:      runtimekit.KindExecutable,
		ToolPath:  "esptool.exe",
	}
}

func findProbeCheck(checks []CheckResult, code string) (CheckResult, bool) {
	for _, check := range checks {
		if check.Code == code {
			return check, true
		}
	}
	return CheckResult{}, false
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

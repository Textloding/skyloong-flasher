package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Textloding/skyloong-flasher/internal/packagekit"
	"github.com/Textloding/skyloong-flasher/internal/preflight"
	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

func TestNewAppConfiguresDeviceProbe(t *testing.T) {
	app := NewApp()
	if app.probeDevice == nil {
		t.Fatal("NewApp() probeDevice is nil")
	}
}

func TestDetectCompatibilityRejectsMissingAnalysis(t *testing.T) {
	app := NewApp()

	report, err := app.DetectCompatibility(CompatibilityRequest{Port: "COM7", Baud: 460800})
	if err == nil {
		t.Fatal("DetectCompatibility() error = nil, want missing analysis error")
	}
	if report != nil {
		t.Fatalf("DetectCompatibility() report = %#v, want nil", report)
	}
	if !strings.Contains(err.Error(), "请先选择并解析固件包") {
		t.Fatalf("DetectCompatibility() error = %q, want understandable Chinese guidance", err)
	}
}

func TestDetectCompatibilityRejectsEmptyPortBeforeProbe(t *testing.T) {
	app := NewApp()
	app.current = &packagekit.Analysis{ProjectName: "firmware"}
	probeCalled := false
	app.probeDevice = func(context.Context, runtimekit.Status, string, int, func(string)) preflight.DeviceProbe {
		probeCalled = true
		return preflight.DeviceProbe{}
	}

	report, err := app.DetectCompatibility(CompatibilityRequest{Port: " \t "})
	if err == nil {
		t.Fatal("DetectCompatibility() error = nil, want empty port error")
	}
	if report != nil {
		t.Fatalf("DetectCompatibility() report = %#v, want nil", report)
	}
	if !strings.Contains(err.Error(), "请选择设备串口") {
		t.Fatalf("DetectCompatibility() error = %q, want understandable Chinese guidance", err)
	}
	if probeCalled {
		t.Fatal("probeDevice called for an empty port")
	}
}

func TestDetectCompatibilityGeneratesReportLogsProbeAndPreservesFlashState(t *testing.T) {
	makeDetectedRuntimeAvailable(t)

	app := NewApp()
	app.cacheDir = t.TempDir()
	analysis := &packagekit.Analysis{
		ProjectName:       "firmware",
		CanFlash:          true,
		Chip:              "esp32s3",
		HardwareVersion:   "SCM_V4.0",
		MinimumFlashBytes: 16 * 1024 * 1024,
		MinimumPSRAMBytes: 8 * 1024 * 1024,
		PSRAMMode:         "octal",
		WriteFlashArgs:    []string{"--flash_mode", "dio"},
		FlashFiles: []packagekit.FlashFile{
			{Offset: "0x0", Path: "bootloader.bin", Size: 4096},
			{Offset: "0x10000", Path: "firmware.bin", Size: 65536},
		},
	}
	app.current = analysis
	wantCanFlash := analysis.CanFlash
	wantWriteFlashArgs := append([]string(nil), analysis.WriteFlashArgs...)
	wantFlashFiles := append([]packagekit.FlashFile(nil), analysis.FlashFiles...)

	app.probeDevice = func(ctx context.Context, status runtimekit.Status, port string, baud int, log func(string)) preflight.DeviceProbe {
		if !status.Available {
			t.Fatal("probeDevice received unavailable runtime")
		}
		if port != "COM7" {
			t.Fatalf("probeDevice port = %q, want COM7", port)
		}
		if baud != 460800 {
			t.Fatalf("probeDevice baud = %d, want 460800", baud)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("probeDevice context has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining < 45*time.Second || remaining > 51*time.Second {
			t.Fatalf("probeDevice context deadline remaining = %v, want about 50s", remaining)
		}
		log("esptool 原始探测行")
		return preflight.DeviceProbe{
			Capabilities: preflight.DeviceCapabilities{
				Chip:            "ESP32-S3",
				FlashBytes:      16 * 1024 * 1024,
				PSRAMKnown:      true,
				PSRAMBytes:      8 * 1024 * 1024,
				PSRAMMode:       "octal",
				SecureBoot:      preflight.TriStateDisabled,
				FlashEncryption: preflight.TriStateDisabled,
			},
			RawOutput: []string{"esptool 原始探测行"},
		}
	}

	report, err := app.DetectCompatibility(CompatibilityRequest{Port: " COM7 ", Baud: 460800})
	if err != nil {
		t.Fatalf("DetectCompatibility() error = %v", err)
	}
	if report == nil {
		t.Fatal("DetectCompatibility() report = nil")
	}
	if report.Overall == "" || report.Device.Chip != "ESP32-S3" {
		t.Fatalf("DetectCompatibility() report = %#v", report)
	}
	if !containsTestString(report.RawLog, "esptool 原始探测行") {
		t.Fatalf("report.RawLog = %#v, want raw probe line", report.RawLog)
	}

	history := app.GetLogHistory()
	for _, want := range []string{"开始兼容性检测", "esptool 原始探测行", "兼容性检测结果：overall=" + report.Overall, "兼容性检测结束"} {
		if !containsTestSubstring(history, want) {
			t.Fatalf("log history = %#v, want line containing %q", history, want)
		}
	}
	if analysis.CanFlash != wantCanFlash {
		t.Fatalf("CanFlash changed from %v to %v", wantCanFlash, analysis.CanFlash)
	}
	if !reflect.DeepEqual(analysis.WriteFlashArgs, wantWriteFlashArgs) {
		t.Fatalf("WriteFlashArgs mutated:\n got %#v\nwant %#v", analysis.WriteFlashArgs, wantWriteFlashArgs)
	}
	if !reflect.DeepEqual(analysis.FlashFiles, wantFlashFiles) {
		t.Fatalf("FlashFiles mutated:\n got %#v\nwant %#v", analysis.FlashFiles, wantFlashFiles)
	}
}

func TestDetectCompatibilityCancelsPreviousDetectionWithoutStaleCleanupCancelingLatest(t *testing.T) {
	makeDetectedRuntimeAvailable(t)

	app := NewApp()
	app.cacheDir = t.TempDir()
	app.current = &packagekit.Analysis{ProjectName: "firmware", CanFlash: true}
	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	allowFirstReturn := make(chan struct{})
	firstProbeReturned := make(chan struct{})
	secondStarted := make(chan struct{})
	secondCanceled := make(chan struct{})
	app.probeDevice = func(ctx context.Context, _ runtimekit.Status, port string, _ int, _ func(string)) preflight.DeviceProbe {
		switch port {
		case "COM1":
			close(firstStarted)
			<-ctx.Done()
			close(firstCanceled)
			<-allowFirstReturn
			close(firstProbeReturned)
		case "COM2":
			close(secondStarted)
			<-ctx.Done()
			close(secondCanceled)
		}
		return preflight.DeviceProbe{}
	}

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = app.DetectCompatibility(CompatibilityRequest{Port: "COM1", Baud: 460800})
	}()
	waitForTestSignal(t, firstStarted, "first detection to start")

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		_, _ = app.DetectCompatibility(CompatibilityRequest{Port: "COM2", Baud: 460800})
	}()
	waitForTestSignal(t, firstCanceled, "second detection to cancel the first")
	assertTestSignalBlocked(t, secondStarted, "second detection while the first probe has not returned")

	close(allowFirstReturn)
	waitForTestSignal(t, firstProbeReturned, "first probe to return")
	waitForTestSignal(t, firstDone, "first detection to return")
	waitForTestSignal(t, secondStarted, "second detection to start after the first returned")
	select {
	case <-secondCanceled:
		t.Fatal("first detection cleanup canceled the active second detection")
	default:
	}

	app.CancelCompatibilityDetection()
	waitForTestSignal(t, secondCanceled, "explicit cancellation to stop the second detection")
	waitForTestSignal(t, secondDone, "second detection to return")
}

func TestCancelCompatibilityDetectionIsIdempotent(t *testing.T) {
	makeDetectedRuntimeAvailable(t)

	app := NewApp()
	app.cacheDir = t.TempDir()
	app.current = &packagekit.Analysis{ProjectName: "firmware", CanFlash: true}
	started := make(chan struct{})
	canceled := make(chan struct{})
	allowProbeReturn := make(chan struct{})
	probeReturned := make(chan struct{})
	app.probeDevice = func(ctx context.Context, _ runtimekit.Status, _ string, _ int, _ func(string)) preflight.DeviceProbe {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-allowProbeReturn
		close(probeReturned)
		return preflight.DeviceProbe{}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = app.DetectCompatibility(CompatibilityRequest{Port: "COM7", Baud: 460800})
	}()
	waitForTestSignal(t, started, "detection to start")

	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		app.CancelCompatibilityDetection()
	}()
	waitForTestSignal(t, canceled, "detection cancellation")
	assertTestSignalBlocked(t, cancelDone, "CancelCompatibilityDetection while the probe has not returned")
	close(allowProbeReturn)
	waitForTestSignal(t, probeReturned, "probe to return")
	waitForTestSignal(t, cancelDone, "CancelCompatibilityDetection to return")
	waitForTestSignal(t, done, "detection to return")
	app.CancelCompatibilityDetection()
	app.CancelCompatibilityDetection()
}

func TestStartFlashCancelsCompatibilityDetectionBeforeEarlyReturn(t *testing.T) {
	makeDetectedRuntimeAvailable(t)

	app := NewApp()
	app.cacheDir = t.TempDir()
	app.current = &packagekit.Analysis{ProjectName: "firmware", CanFlash: true}
	started := make(chan struct{})
	canceled := make(chan struct{})
	allowProbeReturn := make(chan struct{})
	probeReturned := make(chan struct{})
	app.probeDevice = func(ctx context.Context, _ runtimekit.Status, _ string, _ int, _ func(string)) preflight.DeviceProbe {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-allowProbeReturn
		close(probeReturned)
		return preflight.DeviceProbe{}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = app.DetectCompatibility(CompatibilityRequest{Port: "COM7", Baud: 460800})
	}()
	waitForTestSignal(t, started, "detection to start")

	app.mu.Lock()
	app.current = nil
	app.mu.Unlock()
	flashDone := make(chan error, 1)
	go func() {
		flashDone <- app.StartFlash(FlashRequest{Port: "COM7", Baud: 460800})
	}()
	waitForTestSignal(t, canceled, "StartFlash to cancel detection")
	assertTestErrorBlocked(t, flashDone, "StartFlash while the probe has not returned")
	close(allowProbeReturn)
	waitForTestSignal(t, probeReturned, "probe to return")
	waitForTestSignal(t, done, "detection to return")
	select {
	case err := <-flashDone:
		if err == nil {
			t.Fatal("StartFlash() error = nil, want missing analysis error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartFlash to return after the probe exited")
	}
}

func TestMergeBuildAnalysisMetadataFillsEveryMissingField(t *testing.T) {
	source := sourceAnalysisWithMetadata()
	built := &packagekit.Analysis{}

	mergeBuildAnalysisMetadata(built, source)

	if built.HardwareVersion != source.HardwareVersion {
		t.Errorf("HardwareVersion = %q, want %q", built.HardwareVersion, source.HardwareVersion)
	}
	if built.Display != source.Display {
		t.Errorf("Display = %q, want %q", built.Display, source.Display)
	}
	if built.MinimumFlashBytes != source.MinimumFlashBytes {
		t.Errorf("MinimumFlashBytes = %d, want %d", built.MinimumFlashBytes, source.MinimumFlashBytes)
	}
	if built.MinimumPSRAMBytes != source.MinimumPSRAMBytes {
		t.Errorf("MinimumPSRAMBytes = %d, want %d", built.MinimumPSRAMBytes, source.MinimumPSRAMBytes)
	}
	if built.PSRAMMode != source.PSRAMMode {
		t.Errorf("PSRAMMode = %q, want %q", built.PSRAMMode, source.PSRAMMode)
	}
}

func TestMergeBuildAnalysisMetadataDoesNotOverwriteArtifactFields(t *testing.T) {
	tests := []struct {
		name string
		set  func(*packagekit.Analysis)
		get  func(*packagekit.Analysis) interface{}
		want interface{}
	}{
		{
			name: "hardware version",
			set:  func(analysis *packagekit.Analysis) { analysis.HardwareVersion = "artifact-v5" },
			get:  func(analysis *packagekit.Analysis) interface{} { return analysis.HardwareVersion },
			want: "artifact-v5",
		},
		{
			name: "display",
			set:  func(analysis *packagekit.Analysis) { analysis.Display = "artifact-display" },
			get:  func(analysis *packagekit.Analysis) interface{} { return analysis.Display },
			want: "artifact-display",
		},
		{
			name: "minimum flash bytes",
			set:  func(analysis *packagekit.Analysis) { analysis.MinimumFlashBytes = 32 * 1024 * 1024 },
			get:  func(analysis *packagekit.Analysis) interface{} { return analysis.MinimumFlashBytes },
			want: uint64(32 * 1024 * 1024),
		},
		{
			name: "minimum PSRAM bytes",
			set:  func(analysis *packagekit.Analysis) { analysis.MinimumPSRAMBytes = 16 * 1024 * 1024 },
			get:  func(analysis *packagekit.Analysis) interface{} { return analysis.MinimumPSRAMBytes },
			want: uint64(16 * 1024 * 1024),
		},
		{
			name: "PSRAM mode",
			set:  func(analysis *packagekit.Analysis) { analysis.PSRAMMode = "quad" },
			get:  func(analysis *packagekit.Analysis) interface{} { return analysis.PSRAMMode },
			want: "quad",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := sourceAnalysisWithMetadata()
			built := &packagekit.Analysis{}
			test.set(built)

			mergeBuildAnalysisMetadata(built, source)

			if got := test.get(built); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("artifact field = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLogHistoryKeepsFirstAndLastLine(t *testing.T) {
	app := NewApp()
	app.cacheDir = t.TempDir()
	if err := app.prepareLogFile(); err != nil {
		t.Fatalf("prepareLogFile() error = %v", err)
	}

	for i := 0; i < 220; i++ {
		app.logLine("line " + formatTestNumber(i))
	}

	history := app.GetLogHistory()
	if len(history) != 220 {
		t.Fatalf("history len = %d, want 220", len(history))
	}
	if history[0] != "line 000" {
		t.Fatalf("first log = %q", history[0])
	}
	if history[len(history)-1] != "line 219" {
		t.Fatalf("last log = %q", history[len(history)-1])
	}

	raw, err := os.ReadFile(app.logFile)
	if err != nil {
		t.Fatalf("ReadFile(logFile) error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "line 000") || !strings.Contains(text, "line 219") {
		t.Fatalf("log file should contain first and last line, got:\n%s", text)
	}
}

func TestPrepareCacheDirsCreatesExpectedFolders(t *testing.T) {
	app := NewApp()
	app.cacheDir = t.TempDir()
	componentCache := filepath.Join(t.TempDir(), "idf-components")
	t.Setenv("SKYLOONG_COMPONENT_CACHE_PATH", componentCache)

	if err := app.prepareCacheDirs(); err != nil {
		t.Fatalf("prepareCacheDirs() error = %v", err)
	}

	for _, dir := range []string{"downloads", "packages", "runtime", "tools", "logs"} {
		if info, err := os.Stat(filepath.Join(app.cacheDir, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected %s directory to exist, info=%v err=%v", dir, info, err)
		}
	}
	if info, err := os.Stat(runtimekit.ComponentCachePath(app.cacheDir)); err != nil || !info.IsDir() {
		t.Fatalf("expected component cache directory to exist, info=%v err=%v", info, err)
	}
}

func TestPackageWorkspaceCandidatesPreferShortDriveRoot(t *testing.T) {
	t.Setenv("SKYLOONG_PACKAGE_WORKSPACE_PATH", "")

	candidates := packageWorkspaceCandidates(`C:\Users\Administrator\AppData\Local\SkyloongFlasher`)
	if len(candidates) < 2 {
		t.Fatalf("expected package workspace fallback candidates, got %#v", candidates)
	}
	if candidates[0] != filepath.Join(`C:\`, "P") {
		t.Fatalf("first package workspace = %q, want %q", candidates[0], filepath.Join(`C:\`, "P"))
	}
}

func TestPackageWorkspaceCandidatesRespectOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "pkg")
	t.Setenv("SKYLOONG_PACKAGE_WORKSPACE_PATH", override)

	candidates := packageWorkspaceCandidates(`C:\Users\Administrator\AppData\Local\SkyloongFlasher`)
	if candidates[0] != override {
		t.Fatalf("first package workspace = %q, want override %q", candidates[0], override)
	}
}

func makeDetectedRuntimeAvailable(t *testing.T) {
	t.Helper()
	toolDir := t.TempDir()
	toolPath := filepath.Join(toolDir, "esptool.exe")
	if err := os.WriteFile(toolPath, []byte("test executable"), 0o755); err != nil {
		t.Fatalf("WriteFile(esptool.exe) error = %v", err)
	}
	t.Setenv("PATH", toolDir)
	if status := runtimekit.DetectIn(t.TempDir()); !status.Available {
		t.Fatalf("runtimekit.DetectIn() = %#v, want available test runtime", status)
	}
}

func sourceAnalysisWithMetadata() *packagekit.Analysis {
	return &packagekit.Analysis{
		HardwareVersion:   "SCM_V4.0",
		Display:           "320x240-st7789-8bit-parallel",
		MinimumFlashBytes: 16 * 1024 * 1024,
		MinimumPSRAMBytes: 8 * 1024 * 1024,
		PSRAMMode:         "octal",
	}
}

func containsTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsTestSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func waitForTestSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func assertTestSignalBlocked(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatalf("unexpected completion of %s", description)
	case <-time.After(100 * time.Millisecond):
	}
}

func assertTestErrorBlocked(t *testing.T, signal <-chan error, description string) {
	t.Helper()
	select {
	case err := <-signal:
		t.Fatalf("unexpected completion of %s: %v", description, err)
	case <-time.After(100 * time.Millisecond):
	}
}

func formatTestNumber(n int) string {
	return string(rune('0'+n/100)) + string(rune('0'+n/10%10)) + string(rune('0'+n%10))
}

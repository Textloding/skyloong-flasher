package flasher

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Textloding/skyloong-flasher/internal/packagekit"
	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

func TestBuildCommandForPythonEsptool(t *testing.T) {
	analysis := &packagekit.Analysis{
		Root:           `C:\fw\build`,
		CanFlash:       true,
		Chip:           "esp32s3",
		WriteFlashArgs: []string{"--flash_mode", "dio", "--flash_size", "detect", "--flash_freq", "80m"},
		Before:         "default_reset",
		After:          "hard_reset",
		FlashFiles: []packagekit.FlashFile{
			{Offset: "0x0", Path: `C:\fw\build\bootloader\bootloader.bin`},
			{Offset: "0x20000", Path: `C:\fw\build\GK87-Screen.bin`},
		},
	}
	status := runtimekit.Status{Available: true, Kind: runtimekit.KindPythonScript, PythonPath: "python", ToolPath: "esptool.py"}

	cmd, err := BuildCommand(status, analysis, "COM3", 460800)
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{"python", "esptool.py", "-p", "COM3", "--chip", "esp32s3", "write_flash", "0x20000"} {
		if !strings.Contains(got, want) {
			t.Fatalf("command %q missing %q", got, want)
		}
	}
}

func TestBuildCommandForEIMEsptool(t *testing.T) {
	analysis := &packagekit.Analysis{
		CanFlash:       true,
		Chip:           "esp32s3",
		WriteFlashArgs: []string{"--flash_mode", "dio"},
		Before:         "default_reset",
		After:          "hard_reset",
		FlashFiles: []packagekit.FlashFile{
			{Offset: "0x0", Path: `C:\fw path\bootloader.bin`},
			{Offset: "0x20000", Path: `C:\fw path\GK87-Screen.bin`},
		},
	}
	status := runtimekit.Status{
		Available:   true,
		Kind:        runtimekit.KindEIM,
		EIMPath:     `C:\tools\eim.exe`,
		EIMJsonPath: `C:\cache\eim`,
		IDFVersion:  "v5.1.4",
	}

	cmd, err := BuildCommand(status, analysis, "COM5", 460800)
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{"eim.exe", "run", "esptool.py", "-p COM5", "write_flash", "\"C:\\fw path\\GK87-Screen.bin\"", "v5.1.4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("command %q missing %q", got, want)
		}
	}
}

func TestBuildCommandPreservesFlashArgumentOrder(t *testing.T) {
	analysis := &packagekit.Analysis{
		CanFlash:       true,
		Chip:           "esp32s3",
		WriteFlashArgs: []string{"--flash_mode", "dio", "--flash_size", "detect"},
		Before:         "default_reset",
		After:          "hard_reset",
		FlashFiles: []packagekit.FlashFile{
			{Offset: "0x0", Path: `C:\firmware files\bootloader.bin`},
			{Offset: "0x20000", Path: `C:\firmware files\application.bin`},
		},
	}
	status := runtimekit.Status{
		Available: true,
		Kind:      runtimekit.KindExecutable,
		ToolPath:  `C:\ESP Tools\esptool.exe`,
	}

	cmd, err := BuildCommand(status, analysis, "COM12", 921600)
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	want := []string{
		status.ToolPath,
		"-p", "COM12",
		"-b", "921600",
		"--before", "default_reset",
		"--after", "hard_reset",
		"--chip", "esp32s3",
		"write_flash",
		"--flash_mode", "dio",
		"--flash_size", "detect",
		"0x0", `C:\firmware files\bootloader.bin`,
		"0x20000", `C:\firmware files\application.bin`,
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("BuildCommand() args = %#v, want %#v", cmd.Args, want)
	}
}

func TestBuildCommandReportsUnsupportedRuntimeInChinese(t *testing.T) {
	analysis := &packagekit.Analysis{CanFlash: true}
	status := runtimekit.Status{Available: true, Kind: runtimekit.KindMissing}

	_, err := BuildCommand(status, analysis, "COM3", 460800)
	if err == nil {
		t.Fatal("BuildCommand() error = nil, want unsupported runtime error")
	}
	if !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("BuildCommand() error = %q, want readable Chinese text", err)
	}
}

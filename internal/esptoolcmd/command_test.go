package esptoolcmd

import (
	"reflect"
	"testing"

	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

func TestBuildExecutablePreservesFlashIDArguments(t *testing.T) {
	status := runtimekit.Status{
		Available: true,
		Kind:      runtimekit.KindExecutable,
		ToolPath:  `C:\Program Files\Espressif\esptool.exe`,
	}
	args := []string{"-p", "COM7", "-b", "460800", "--chip", "esp32s3", "flash_id"}

	cmd, err := Build(status, args...)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := append([]string{status.ToolPath}, args...)
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("Build() args = %#v, want %#v", cmd.Args, want)
	}
}

func TestBuildPythonScriptPreservesSecurityArgumentsAndPathsWithSpaces(t *testing.T) {
	status := runtimekit.Status{
		Available:  true,
		Kind:       runtimekit.KindPythonScript,
		PythonPath: `C:\Python Runtime\python.exe`,
		ToolPath:   `C:\ESP Tools\esptool.py`,
	}
	args := []string{"-p", "COM8", "-b", "115200", "get_security_info"}

	cmd, err := Build(status, args...)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := append([]string{status.PythonPath, status.ToolPath}, args...)
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("Build() args = %#v, want %#v", cmd.Args, want)
	}
}

func TestBuildEIMPreservesWrappedArgumentsAndPathsWithSpaces(t *testing.T) {
	status := runtimekit.Status{
		Available:   true,
		Kind:        runtimekit.KindEIM,
		EIMPath:     `C:\EIM Runtime\eim.exe`,
		EIMJsonPath: `C:\EIM Config\runtime`,
		IDFVersion:  "v5.1.4",
	}
	args := []string{"-p", "COM9", "-b", "460800", "flash_id", `C:\Probe Logs\result.txt`}

	cmd, err := Build(status, args...)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []string{
		status.EIMPath,
		"run",
		"--esp-idf-json-path",
		status.EIMJsonPath,
		`esptool.py -p COM9 -b 460800 flash_id "C:\Probe Logs\result.txt"`,
		status.IDFVersion,
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("Build() args = %#v, want %#v", cmd.Args, want)
	}
}

func TestBuildRejectsUnsupportedRuntime(t *testing.T) {
	_, err := Build(runtimekit.Status{Available: true, Kind: runtimekit.KindMissing}, "flash_id")
	if err == nil {
		t.Fatal("Build() error = nil, want unsupported runtime error")
	}
}

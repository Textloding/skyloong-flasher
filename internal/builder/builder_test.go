package builder

import (
	"errors"
	"strings"
	"testing"

	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

func TestBuildCommandWithExportScript(t *testing.T) {
	status := runtimekit.Status{CanBuild: true, ExportScript: `C:\esp\esp-idf\export.ps1`}
	cmd, err := BuildCommand(status, `C:\work\SKYLOONG`)
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "export.ps1") || !strings.Contains(got, "idf.py build") {
		t.Fatalf("unexpected command: %q", got)
	}
}

func TestBuildCommandWithIDFPy(t *testing.T) {
	status := runtimekit.Status{CanBuild: true, IDFPyPath: `C:\tools\idf.py`}
	cmd, err := BuildCommand(status, `C:\work\SKYLOONG`)
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "idf.py") || !strings.Contains(got, "build") {
		t.Fatalf("unexpected command: %q", got)
	}
}

func TestBuildCommandWithIDFPyPrefixesPortableGitPath(t *testing.T) {
	status := runtimekit.Status{CanBuild: true, IDFPyPath: `C:\tools\idf.py`, GitPath: `C:\cache\tools\git\cmd\git.exe`}
	cmd, err := BuildCommand(status, `C:\work\SKYLOONG`)
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	if pathValue := envValue(cmd.Env, "PATH"); !strings.HasPrefix(pathValue, `C:\cache\tools\git\cmd;`) {
		t.Fatalf("portable Git cmd dir should be first in PATH, got %q", pathValue)
	}
}

func TestBuildCommandWithEIMRuntime(t *testing.T) {
	status := runtimekit.Status{
		CanBuild:           true,
		Kind:               runtimekit.KindEIM,
		EIMPath:            `C:\tools\eim.exe`,
		EIMJsonPath:        `C:\cache\eim`,
		IDFVersion:         "v5.1.4",
		ComponentCachePath: `C:\cache\cm`,
	}
	cmd, err := BuildCommand(status, `C:\work\SKYLOONG`)
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{"eim.exe", "run", "idf.py build", "v5.1.4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("unexpected command: %q missing %q", got, want)
		}
	}
	if got := envValue(cmd.Env, "IDF_COMPONENT_CACHE_PATH"); got != `C:\cache\cm` {
		t.Fatalf("IDF_COMPONENT_CACHE_PATH = %q", got)
	}
}

func TestBuildLogStateTreatsCMakeFailureAsError(t *testing.T) {
	state := newBuildLogState()
	for _, line := range []string{
		"Running cmake in directory C:\\work\\build",
		"CMake Error at C:/esp-idf/tools/cmake/build.cmake:540 (message):",
		"cmake failed with exit code 1",
	} {
		state.Observe(line)
	}

	err := state.Err(nil)
	if err == nil {
		t.Fatalf("expected CMake failure to become build error")
	}
	if !strings.Contains(err.Error(), "ESP-IDF 构建失败") || !strings.Contains(err.Error(), "cmake failed with exit code 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildLogStatePrefersSpecificComponentCachePathMessage(t *testing.T) {
	state := newBuildLogState()
	for _, line := range []string{
		"CMake Error at C:/esp-idf/tools/cmake/build.cmake:540 (message):",
		"FileNotFoundError: [Errno 2] No such file or directory:",
		`'C:\Users\Administrator\AppData\Local\Espressif\ComponentManager\Cache\service_d92d8f1e\espressif__esp-serial-flasher_1.11.0_6f5d6859\test\target-example-src\hello-world-ESP32-src\build-ram-esp32h2\esp-idf\spi_flash\CMakeFiles\__idf_spi_flash.dir\cache_utils.c.obj'`,
	} {
		state.Observe(line)
	}

	err := state.Err(nil)
	if err == nil {
		t.Fatalf("expected component cache path failure")
	}
	if !strings.Contains(err.Error(), "组件缓存路径过长") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildLogStateIncludesProcessError(t *testing.T) {
	state := newBuildLogState()
	err := state.Err(errors.New("exit status 1"))
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("expected process error, got %v", err)
	}
}

func TestBuildCommandRejectsMissingRuntime(t *testing.T) {
	_, err := BuildCommand(runtimekit.Status{}, `C:\work\SKYLOONG`)
	if err == nil {
		t.Fatalf("expected missing build runtime error")
	}
}

func envValue(env []string, key string) string {
	for _, item := range env {
		gotKey, value, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(gotKey, key) {
			return value
		}
	}
	return ""
}

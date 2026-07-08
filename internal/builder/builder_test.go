package builder

import (
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

func TestBuildCommandWithEIMRuntime(t *testing.T) {
	status := runtimekit.Status{
		CanBuild:    true,
		Kind:        runtimekit.KindEIM,
		EIMPath:     `C:\tools\eim.exe`,
		EIMJsonPath: `C:\cache\eim`,
		IDFVersion:  "v5.1.4",
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
}

func TestBuildCommandRejectsMissingRuntime(t *testing.T) {
	_, err := BuildCommand(runtimekit.Status{}, `C:\work\SKYLOONG`)
	if err == nil {
		t.Fatalf("expected missing build runtime error")
	}
}

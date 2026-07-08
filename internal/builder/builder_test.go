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

func TestBuildCommandRejectsMissingRuntime(t *testing.T) {
	_, err := BuildCommand(runtimekit.Status{}, `C:\work\SKYLOONG`)
	if err == nil {
		t.Fatalf("expected missing build runtime error")
	}
}

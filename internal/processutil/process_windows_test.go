//go:build windows

package processutil

import "testing"

func TestCommandHidesWindowsConsole(t *testing.T) {
	cmd := Command("powershell", "-NoProfile", "-Command", "Write-Output ok")
	if cmd.SysProcAttr == nil {
		t.Fatalf("SysProcAttr must be set")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatalf("HideWindow must be true")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CREATE_NO_WINDOW flag must be set")
	}
}

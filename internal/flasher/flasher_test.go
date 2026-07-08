package flasher

import (
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

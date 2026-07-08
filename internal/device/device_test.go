package device

import (
	"strings"
	"testing"
)

func TestScoreFlashModeEspressifDevice(t *testing.T) {
	d := Score(Device{
		Name:        "USB JTAG/serial debug unit (COM3)",
		Port:        "COM3",
		PNPDeviceID: "USB\\VID_303A&PID_1001\\1234",
	})

	if !d.CanFlash || d.Mode != ModeFlash || d.Score < 90 {
		t.Fatalf("unexpected scored flash device: %#v", d)
	}
}

func TestScoreRuntimeKeyboardDevice(t *testing.T) {
	d := Score(Device{
		Name:        "STC USB Keyboard",
		PNPDeviceID: "USB\\VID_34BF&PID_FF0E\\1234",
	})

	if d.CanFlash || d.Mode != ModeRuntime || d.Score < 40 {
		t.Fatalf("unexpected scored runtime device: %#v", d)
	}
}

func TestCom1IsLowConfidence(t *testing.T) {
	d := Score(Device{Name: "通信端口 (COM1)", Port: "COM1", PNPDeviceID: "ACPI\\PNP0501\\0"})
	if d.CanFlash || d.Score > 10 {
		t.Fatalf("COM1 should be low confidence: %#v", d)
	}
}

func TestScoreHintsAreReadableChinese(t *testing.T) {
	cases := []struct {
		name string
		in   Device
		want string
	}{
		{
			name: "unknown",
			in:   Device{Name: "USB Serial Device"},
			want: "未识别为屏幕刷机串口",
		},
		{
			name: "flash mode",
			in:   Device{Name: "USB JTAG/serial debug unit (COM3)", Port: "COM3", PNPDeviceID: "USB\\VID_303A&PID_1001\\1234"},
			want: "已检测到 ESP32-S3 下载模式串口，可刷机",
		},
		{
			name: "runtime keyboard",
			in:   Device{Name: "STC USB Keyboard", PNPDeviceID: "USB\\VID_34BF&PID_FF0E\\1234"},
			want: "检测到 SKYLOONG 键盘运行态，请进入 BOOT/下载模式后刷机",
		},
		{
			name: "com1",
			in:   Device{Name: "通信端口 (COM1)", Port: "COM1", PNPDeviceID: "ACPI\\PNP0501\\0"},
			want: "COM1 通常是电脑主板串口，不建议选择",
		},
		{
			name: "generic usb serial",
			in:   Device{Name: "USB 串行设备 (COM7)", Port: "COM7", PNPDeviceID: "USB\\VID_1A86&PID_7523\\1234"},
			want: "检测到 USB 串口，需进一步确认是否为 ESP32-S3",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := Score(tt.in)
			if got.Hint != tt.want {
				t.Fatalf("hint should be readable Chinese\nwant: %q\n got: %q", tt.want, got.Hint)
			}
			if strings.Contains(got.Hint, "�") || strings.Contains(got.Hint, "閫") || strings.Contains(got.Hint, "妫") || strings.Contains(got.Hint, "鏈") {
				t.Fatalf("hint still looks mojibake: %q", got.Hint)
			}
		})
	}
}

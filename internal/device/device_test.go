package device

import "testing"

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

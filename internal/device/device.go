package device

import (
	"regexp"
	"sort"
	"strings"
)

const (
	ModeUnknown = "unknown"
	ModeRuntime = "runtime"
	ModeFlash   = "flash"
)

type Device struct {
	Name        string `json:"name"`
	Port        string `json:"port"`
	PNPDeviceID string `json:"pnpDeviceId"`
	Mode        string `json:"mode"`
	CanFlash    bool   `json:"canFlash"`
	Score       int    `json:"score"`
	Hint        string `json:"hint"`
}

func Score(d Device) Device {
	id := strings.ToUpper(d.PNPDeviceID)
	name := strings.ToUpper(d.Name)
	if d.Port == "" {
		d.Port = extractPort(d.Name)
	}
	d.Mode = ModeUnknown
	d.Hint = "未识别为屏幕刷机串口"

	if strings.Contains(id, "VID_303A&PID_1001") || strings.Contains(name, "USB JTAG/SERIAL") {
		d.Mode = ModeFlash
		d.CanFlash = d.Port != ""
		d.Score = 96
		d.Hint = "已检测到 ESP32-S3 下载模式串口，可刷机"
		return d
	}

	if strings.Contains(id, "VID_34BF&PID_FF0E") {
		d.Mode = ModeRuntime
		d.CanFlash = false
		d.Score = 48
		d.Hint = "检测到 SKYLOONG 键盘运行态，请进入 BOOT/下载模式后刷机"
		return d
	}

	if strings.EqualFold(d.Port, "COM1") && strings.Contains(id, "ACPI\\PNP0501") {
		d.Score = 5
		d.Hint = "COM1 通常是电脑主板串口，不建议选择"
		return d
	}

	if d.Port != "" && strings.Contains(name, "USB") {
		d.Score = 35
		d.Hint = "检测到 USB 串口，需进一步确认是否为 ESP32-S3"
	}
	return d
}

func Sort(devices []Device) {
	sort.SliceStable(devices, func(i, j int) bool {
		return devices[i].Score > devices[j].Score
	})
}

func extractPort(text string) string {
	re := regexp.MustCompile(`(?i)\bCOM\d+\b`)
	return strings.ToUpper(re.FindString(text))
}

//go:build windows

package device

import (
	"encoding/json"
	"strings"

	"github.com/Textloding/skyloong-flasher/internal/processutil"
)

type pnpRow struct {
	Name        string
	PNPDeviceID string
	Status      string
}

type serialRow struct {
	DeviceID    string
	Name        string
	PNPDeviceID string
}

func Scan() ([]Device, error) {
	var devices []Device
	serialRows, _ := readSerialRows()
	for _, row := range serialRows {
		devices = append(devices, Score(Device{Name: row.Name, Port: row.DeviceID, PNPDeviceID: row.PNPDeviceID}))
	}

	pnpRows, err := readPNPRows()
	if err != nil {
		Sort(devices)
		return devices, err
	}
	for _, row := range pnpRows {
		devices = append(devices, Score(Device{Name: row.Name, PNPDeviceID: row.PNPDeviceID}))
	}
	Sort(devices)
	return dedupe(devices), nil
}

func readPNPRows() ([]pnpRow, error) {
	script := `Get-CimInstance Win32_PnPEntity | Where-Object { $_.PNPDeviceID -match 'VID_303A|VID_34BF|USB|COM' } | Select-Object Name,PNPDeviceID,Status | ConvertTo-Json -Compress`
	var rows []pnpRow
	err := runJSON(script, &rows)
	return rows, err
}

func readSerialRows() ([]serialRow, error) {
	script := `Get-CimInstance Win32_SerialPort | Select-Object DeviceID,Name,PNPDeviceID | ConvertTo-Json -Compress`
	var rows []serialRow
	err := runJSON(script, &rows)
	return rows, err
}

func runJSON(script string, out interface{}) error {
	cmd := processutil.Command("powershell", "-NoProfile", "-Command", script)
	raw, err := cmd.Output()
	if err != nil {
		return err
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "{") {
		text = "[" + text + "]"
	}
	return json.Unmarshal([]byte(text), out)
}

func dedupe(devices []Device) []Device {
	seen := map[string]bool{}
	out := make([]Device, 0, len(devices))
	for _, d := range devices {
		key := d.Port + "|" + d.PNPDeviceID + "|" + d.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	return out
}

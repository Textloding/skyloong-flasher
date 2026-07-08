//go:build windows

package device

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	cmd := processutil.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", wrapJSONScript(script))
	raw, err := cmd.Output()
	if err != nil {
		return err
	}
	return decodeJSONPayload(raw, out)
}

func wrapJSONScript(script string) string {
	return fmt.Sprintf(`$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[System.Text.UTF8Encoding]::new($false); $OutputEncoding=[Console]::OutputEncoding; $json = & { %s }; if ($null -eq $json) { $json = '' }; [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes([string]$json))`, script)
}

func decodeJSONPayload(raw []byte, out interface{}) error {
	encoded := strings.TrimSpace(string(raw))
	if encoded == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return decodeJSONText(encoded, out)
	}
	return decodeJSONText(string(decoded), out)
}

func decodeJSONText(text string, out interface{}) error {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
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

//go:build windows

package device

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeJSONPayloadPreservesChineseDeviceName(t *testing.T) {
	jsonText := `[{"DeviceID":"COM7","Name":"USB 串行设备 (COM7)","PNPDeviceID":"USB\\VID_303A&PID_1001\\1234"}]`
	encoded := base64.StdEncoding.EncodeToString([]byte(jsonText))

	var rows []serialRow
	if err := decodeJSONPayload([]byte(encoded+"\r\n"), &rows); err != nil {
		t.Fatalf("decode JSON payload: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].Name != "USB 串行设备 (COM7)" {
		t.Fatalf("device name should keep Chinese text, got %q", rows[0].Name)
	}
}

func TestRunJSONPreservesChinesePowerShellOutput(t *testing.T) {
	script := `[pscustomobject]@{DeviceID='COM7';Name='USB 串行设备 (COM7)';PNPDeviceID='USB\VID_303A&PID_1001\1234'} | ConvertTo-Json -Compress`

	var rows []serialRow
	if err := runJSON(script, &rows); err != nil {
		t.Fatalf("run JSON: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].Name != "USB 串行设备 (COM7)" {
		t.Fatalf("PowerShell output should keep Chinese text, got %q", rows[0].Name)
	}
}

func TestWrapJSONScriptUsesUtf8Base64Output(t *testing.T) {
	script := wrapJSONScript(`"测试" | ConvertTo-Json -Compress`)

	for _, want := range []string{"[Console]::OutputEncoding", "UTF8Encoding", "ToBase64String"} {
		if !strings.Contains(script, want) {
			t.Fatalf("wrapped script should contain %q, got %s", want, script)
		}
	}
}

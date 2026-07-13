# Device Preflight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add non-blocking firmware-package and ESP32-S3 device compatibility detection with clear Chinese explanations and executable recovery steps.

**Architecture:** A new `internal/preflight` package owns firmware inspection, read-only esptool probing, compatibility comparison, and Chinese guidance. `internal/packagekit` carries optional firmware hardware metadata, while a small shared `internal/esptoolcmd` package keeps EIM/Python/executable command construction identical between probing and flashing. The Wails app exposes one detection API and the React UI renders the report without changing the existing flash eligibility.

**Tech Stack:** Go 1.20+, Wails v2, React, TypeScript, Vite, ESP-IDF partition-table binary format, esptool.py.

---

### Task 1: Parse Firmware Hardware Metadata

**Files:**
- Modify: `internal/packagekit/packagekit.go`
- Modify: `internal/packagekit/packagekit_test.go`
- Create: `../esp32_screen_module/skyloong_firmware.json`

- [ ] **Step 1: Write failing package metadata tests**

Add tests that create source and prebuilt zip trees containing:

```json
{
  "schema_version": 1,
  "hardware_version": "SCM_V4.0",
  "display": "320x240-st7789-8bit-parallel",
  "minimum_flash_bytes": 16777216,
  "minimum_psram_bytes": 8388608,
  "psram_mode": "octal"
}
```

Assert that `AnalyzeDir` and `AnalyzeZip` expose `HardwareVersion`, `Display`, `MinimumFlashBytes`, `MinimumPSRAMBytes`, and `PSRAMMode`. Add a test proving a package without metadata remains valid with empty/zero metadata fields.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/packagekit -run FirmwareMetadata -v`

Expected: compilation fails because the new `Analysis` fields do not exist.

- [ ] **Step 3: Implement metadata parsing**

Extend `Analysis` with JSON-visible fields:

```go
HardwareVersion   string `json:"hardwareVersion"`
Display           string `json:"display"`
MinimumFlashBytes uint64 `json:"minimumFlashBytes"`
MinimumPSRAMBytes uint64 `json:"minimumPsramBytes"`
PSRAMMode         string `json:"psramMode"`
```

Add a private `firmwareMetadataJSON` type and `applyFirmwareMetadata(analysis, root)`. Search for `skyloong_firmware.json` under the extracted package root, validate `schema_version == 1`, and copy values into `Analysis`. Invalid metadata returns a package-analysis error that names the metadata file.

- [ ] **Step 4: Add the V4 firmware metadata file**

Create `../esp32_screen_module/skyloong_firmware.json` with the exact JSON shown in Step 1. This records the current firmware's fixed parallel display wiring, 16MB Flash requirement, and 8MB octal PSRAM requirement.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/packagekit -v`

Expected: PASS.

Commit flasher changes:

```powershell
git add internal/packagekit/packagekit.go internal/packagekit/packagekit_test.go
git commit -m "解析固件硬件兼容性元数据"
```

Commit firmware metadata separately in `../esp32_screen_module`:

```powershell
git add skyloong_firmware.json
git commit -m "声明V4固件硬件与内存要求"
```

### Task 2: Inspect Flash Regions and ESP-IDF Partition Tables

**Files:**
- Create: `internal/preflight/types.go`
- Create: `internal/preflight/partition.go`
- Create: `internal/preflight/partition_test.go`
- Create: `internal/preflight/firmware.go`
- Create: `internal/preflight/firmware_test.go`

- [ ] **Step 1: Write failing partition parser tests**

Create binary fixtures in memory using 32-byte ESP-IDF entries with little-endian magic `0x50AA`. Cover:

```go
entries := []partitionFixture{
    {Type: 0x01, Subtype: 0x02, Offset: 0x10000, Size: 0x2000, Label: "otadata"},
    {Type: 0x00, Subtype: 0x10, Offset: 0x20000, Size: 0x500000, Label: "ota_0"},
    {Type: 0x00, Subtype: 0x11, Offset: 0x520000, Size: 0x500000, Label: "ota_1"},
    {Type: 0x01, Subtype: 0x82, Offset: 0xA20000, Size: 0x5D0000, Label: "spiffs"},
}
```

Assert successful parsing, a highest end address of `0xFF0000`, and a rounded minimum Flash requirement of 16MB. Add malformed magic, overlapping partitions, and truncated-entry tests.

- [ ] **Step 2: Run partition tests and verify RED**

Run: `go test ./internal/preflight -run Partition -v`

Expected: compilation fails because `ParsePartitionTable` is missing.

- [ ] **Step 3: Implement partition parsing and validation**

Define `Partition`, `FlashRegion`, `FirmwareRequirements`, `DeviceCapabilities`, `DeviceProbe`, `CheckResult`, `FirmwareInspection`, `Report`, and status constants in `types.go`. `DeviceProbe` contains parsed capabilities, probe checks, and raw output. Implement:

```go
func ParsePartitionTable(raw []byte) ([]Partition, error)
func ValidatePartitions(parts []Partition) []CheckResult
func MinimumFlashSize(parts []Partition) uint64
```

Stop on erased entries (`0xFFFF`) or MD5 marker (`0xEBEB`). Reject truncated entries and ignore empty labels safely. Round the maximum partition end to one of 1, 2, 4, 8, 16, 32, 64, or 128MB.

- [ ] **Step 4: Write failing firmware inspection tests**

Construct `packagekit.Analysis` values for complete and incomplete packages. Verify checks for required offsets `0x0`, `0x8000`, `0x10000`, and the first app partition; overlapping write regions; app binary larger than its app partition; metadata requirements; and a missing/unparseable partition table.

- [ ] **Step 5: Implement firmware inspection**

Implement:

```go
func InspectFirmware(analysis *packagekit.Analysis) FirmwareInspection
```

The result includes requirements, checks, and raw technical details. Missing metadata produces `unknown`, not `warning`. Missing files and invalid layout produce Chinese `warning` checks but do not mutate `analysis.CanFlash`.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/preflight -v`

Expected: PASS.

```powershell
git add internal/preflight
git commit -m "检测固件文件与分区容量"
```

### Task 3: Share Esptool Command Construction and Probe Devices

**Files:**
- Create: `internal/esptoolcmd/command.go`
- Create: `internal/esptoolcmd/command_test.go`
- Modify: `internal/flasher/flasher.go`
- Modify: `internal/flasher/flasher_test.go`
- Create: `internal/preflight/probe.go`
- Create: `internal/preflight/probe_test.go`

- [ ] **Step 1: Write failing shared command tests**

For executable, Python-script, and EIM runtime statuses, assert that `esptoolcmd.Build` preserves every argument and correctly wraps paths containing spaces. Include `flash_id` and `get_security_info` examples.

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/esptoolcmd ./internal/flasher -v`

Expected: `internal/esptoolcmd` does not exist.

- [ ] **Step 3: Implement and adopt the shared command builder**

Implement:

```go
func Build(status runtimekit.Status, args ...string) (*exec.Cmd, error)
```

Move only runtime wrapping into this package. Keep flash argument ordering in `flasher.BuildCommand`, then delegate the final command creation to `esptoolcmd.Build`. Existing flasher command tests must remain unchanged and pass.

- [ ] **Step 4: Write failing esptool output parser tests**

Use representative output containing:

```text
Chip is ESP32-S3 (QFN56) (revision v0.2)
Features: WiFi, BLE, Embedded PSRAM 8MB (AP_3v3)
Detected flash size: 16MB
Secure Boot: Disabled
Flash Encryption: Disabled
```

Assert chip, revision, Flash bytes, PSRAM bytes/type, and tri-state security values. Add outputs with no PSRAM line, 8MB Flash, access denied, port missing, failed connection, wrong boot mode, and timeout.

- [ ] **Step 5: Implement read-only probing**

Implement pure parsers plus:

```go
func ProbeDevice(ctx context.Context, status runtimekit.Status, port string, baud int, log func(string)) DeviceProbe
```

Run `flash_id` followed by `get_security_info` with a 20-second timeout per command. Capture stdout and stderr, preserve raw lines, and never run erase or write commands. A failed security query must retain successful chip/Flash data.

- [ ] **Step 6: Run tests and commit**

Run: `go test ./internal/esptoolcmd ./internal/flasher ./internal/preflight -v`

Expected: PASS.

```powershell
git add internal/esptoolcmd internal/flasher internal/preflight/probe.go internal/preflight/probe_test.go
git commit -m "增加设备只读探测命令"
```

### Task 4: Compare Capabilities and Generate Chinese Recovery Steps

**Files:**
- Create: `internal/preflight/compare.go`
- Create: `internal/preflight/compare_test.go`
- Create: `internal/preflight/guidance.go`
- Create: `internal/preflight/guidance_test.go`

- [ ] **Step 1: Write failing comparison tests**

Cover chip match/mismatch, 8MB device versus 16MB requirement, sufficient/insufficient/unknown PSRAM, octal-mode mismatch, Secure Boot, Flash Encryption, unknown hardware version, and probe failure. Assert that no result changes a `CanFlash` value.

- [ ] **Step 2: Write failing Chinese guidance tests**

For each stable code assert a concrete action list. For example:

```go
want := []string{
    "断开屏幕的 USB 连接",
    "按住 BOOT/下载键并重新插入 USB",
    "看到新的 COM 串口后松开 BOOT/下载键",
    "返回工具点击重新检测",
}
```

Assert that Flash-capacity guidance includes actual and required sizes, V3/V4 mismatch says repeated flashing cannot fix wiring differences, and unknown errors retain technical output.

- [ ] **Step 3: Implement report comparison and overall state**

Implement:

```go
func Compare(firmware FirmwareInspection, device DeviceProbe) Report
```

Use stable codes such as `chip_mismatch`, `flash_too_small`, `psram_too_small`, `psram_unknown`, `secure_boot_enabled`, `flash_encryption_enabled`, `hardware_unknown`, `port_busy`, and `download_mode_required`. Overall priority is detection error, risk, unknown, compatible.

- [ ] **Step 4: Implement Chinese guidance mapping**

Use code-based templates rather than replacing entire English lines. Each non-pass item gets `Resolution` and ordered `Steps`. Include actual values using a shared byte formatter. Unknown low-level errors receive a generic Chinese summary plus original output in `Technical`.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/preflight -v`

Expected: PASS.

```powershell
git add internal/preflight/compare.go internal/preflight/compare_test.go internal/preflight/guidance.go internal/preflight/guidance_test.go
git commit -m "生成中文兼容性诊断与解决步骤"
```

### Task 5: Expose Detection Through the Wails App

**Files:**
- Modify: `app.go`
- Modify: `app_test.go`

- [ ] **Step 1: Write failing app tests**

Add tests for metadata preservation after source builds through a small merge helper, missing current analysis, missing port, report logging, and proof that detection does not mutate `a.current.CanFlash` or `FlashFiles`.

- [ ] **Step 2: Run and verify RED**

Run: `go test . -run Compatibility -v`

Expected: compilation fails because `DetectCompatibility` and the merge helper do not exist.

- [ ] **Step 3: Implement the Wails method**

Add:

```go
type CompatibilityRequest struct {
    Port string `json:"port"`
    Baud int    `json:"baud"`
}

func (a *App) DetectCompatibility(req CompatibilityRequest) (*preflight.Report, error)
```

Reuse or prepare the runtime, inspect the current package, run the read-only probe, compare results, emit detection progress, and append raw probe output to the existing log. Use a context timeout and return user-facing Chinese errors for missing package or port.

When `BuildSourcePackage` replaces the source analysis with the build analysis, copy hardware metadata fields from the original analysis if the build output has none.

- [ ] **Step 4: Run tests and commit**

Run: `go test . ./internal/...`

Expected: PASS.

```powershell
git add app.go app_test.go
git commit -m "接入固件与设备兼容性检测接口"
```

### Task 6: Add the Compatibility Panel to React

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/styles.css`

- [ ] **Step 1: Add frontend report types and state**

Define TypeScript equivalents of `Report`, `CheckResult`, firmware requirements, and device capabilities. Add `preflight`, `preflightBusy`, and `preflightError` state. Clear stale results when the package or selected port changes.

- [ ] **Step 2: Add detection behavior**

Implement `detectCompatibility()` calling:

```ts
await call<PreflightReport>("DetectCompatibility", {
  port: selectedPort,
  baud: 460800,
});
```

Start automatically when a flashable analysis and selected port are both available, while retaining a “重新检测” button. Prevent duplicate detection calls, but do not reuse results after either input changes.

- [ ] **Step 3: Render user-facing diagnostics**

Insert an unframed compatibility section between device selection and flashing. Show:

- Overall result and last detection time.
- Firmware versus device chip, Flash, PSRAM, security, and hardware version.
- Ordered checks with Chinese `Summary`, `Resolution`, and numbered `Steps`.
- Expandable raw technical details.
- “重新检测” action.

The flash button condition must remain exactly `busy || !analysis?.canFlash || !selectedPort`; preflight status must not be added to `disabled`.
换言之，界面不因检测结果而禁用刷机，只负责把风险和解决步骤讲清楚。

- [ ] **Step 4: Style responsive and accessible states**

Use the existing color tokens and card radius. Add stable grid tracks, wrapping for long paths/errors, visible focus states, status text in addition to color, and a single-column layout at the existing mobile breakpoint. Do not nest the compatibility section inside another card.

- [ ] **Step 5: Build frontend and commit**

Run: `npm run build --prefix frontend`

Expected: TypeScript and Vite build succeed.

```powershell
git add frontend/src/App.tsx frontend/src/styles.css
git commit -m "展示设备兼容性检测与解决指导"
```

### Task 7: Document, Integrate, and Verify End to End

**Files:**
- Modify: `README.md`
- Modify: `docs/releases/2026-07-08.md`
- Modify: generated Wails bindings only if `wails generate module` changes them

- [ ] **Step 1: Document supported checks and limitations**

Explain that detection is read-only and non-blocking; list chip, Flash, PSRAM, security, package, and partition checks; document V3/V4 auto-detection limitations; and tell users to follow numbered steps then click “重新检测”.

- [ ] **Step 2: Run all automated verification**

Run:

```powershell
gofmt -w app.go app_test.go internal/esptoolcmd internal/preflight internal/packagekit internal/flasher
go test ./...
npm run build --prefix frontend
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Build the Windows application**

Run: `wails build`

Expected: `build/bin/SKYLOONG-Flasher.exe` is produced successfully.

- [ ] **Step 4: Perform device smoke testing when COM3 is available**

Use the current V4 package and verify the report shows ESP32-S3, 16MB Flash, 8MB embedded PSRAM, security states, and compatible partition capacity. Disconnect or occupy COM3 once to verify Chinese recovery guidance. Confirm no erase/write command appears in detection logs.

- [ ] **Step 5: Final commit**

```powershell
git add README.md docs/releases/2026-07-08.md
git commit -m "完善刷机前检测文档与验证"
```

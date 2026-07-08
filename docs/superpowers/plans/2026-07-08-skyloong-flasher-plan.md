# SKYLOONG Flasher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an independent Windows desktop flashing tool for SKYLOONG/GK87 ESP32-S3 screen firmware packages.

**Architecture:** A Wails desktop app provides a polished React UI and a Go backend. The backend parses local/GitHub firmware packages, detects COM devices, prepares flashing arguments, and runs an external esptool-compatible runtime through a task pipeline.

**Tech Stack:** Go, Wails, React, TypeScript, Vite, CSS glass UI.

---

### Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `app.go`
- Create: `wails.json`
- Create: `frontend/package.json`
- Create: `frontend/index.html`
- Create: `frontend/src/main.tsx`
- Create: `frontend/src/App.tsx`
- Create: `frontend/src/styles.css`

- [ ] Create a Wails-compatible Go project with React frontend.
- [ ] Add backend API placeholders for package analysis, device scan, runtime check, and flashing.
- [ ] Add glassmorphism UI shell with five-step workflow.
- [ ] Verify `go test ./...` and `npm run build`.

### Task 2: Package Parsing

**Files:**
- Create: `internal/packagekit/packagekit.go`
- Create: `internal/packagekit/packagekit_test.go`

- [ ] Implement safe zip extraction.
- [ ] Recursively find `flasher_args.json` or `flash_args`.
- [ ] Parse ESP-IDF flasher arguments into structured flash files.
- [ ] Detect source-only ESP-IDF projects.
- [ ] Add tests for direct flash package, source package, and invalid package.

### Task 3: GitHub Source Handling

**Files:**
- Create: `internal/githubsource/githubsource.go`
- Create: `internal/githubsource/githubsource_test.go`

- [ ] Parse GitHub repository, branch, tag, archive, and codeload URLs.
- [ ] Download zip files with progress callbacks.
- [ ] Add tests for URL normalization.

### Task 4: Device Detection

**Files:**
- Create: `internal/device/device.go`
- Create: `internal/device/device_windows.go`
- Create: `internal/device/device_test.go`

- [ ] Scan Windows serial ports and PnP entities.
- [ ] Score Espressif flash-mode devices higher than keyboard HID runtime devices.
- [ ] Mark `COM1` as low-confidence unless it has Espressif hardware IDs.
- [ ] Add tests for device scoring.

### Task 5: Runtime and Flashing Pipeline

**Files:**
- Create: `internal/runtime/runtime.go`
- Create: `internal/flasher/flasher.go`
- Create: `internal/flasher/flasher_test.go`

- [ ] Detect local esptool from ESP-IDF or PATH.
- [ ] Build esptool command arguments from parsed package metadata.
- [ ] Stream command output to frontend logs.
- [ ] Add dry-run tests for command generation.

### Task 6: UI Integration and Build

**Files:**
- Modify: `app.go`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/styles.css`
- Create: `README.md`

- [ ] Wire frontend actions to backend APIs.
- [ ] Add Chinese guidance for BOOT/download mode, package selection, and logs.
- [ ] Add README with usage and release packaging instructions.
- [ ] Run `go test ./...`, `npm run build`, and a desktop build smoke test if Wails is available.


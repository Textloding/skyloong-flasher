package preflight

import (
	"fmt"
	"strings"
	"unicode"
)

func Compare(firmware FirmwareInspection, device DeviceProbe) Report {
	report := Report{
		Firmware: firmware.Requirements,
		Device:   device.Capabilities,
		Checks:   make([]CheckResult, 0, len(firmware.Checks)+len(device.Checks)+1),
		RawLog:   make([]string, 0, len(firmware.RawTechnical)+len(device.RawOutput)),
	}
	report.Checks = append(report.Checks, firmware.Checks...)
	report.Checks = append(report.Checks, device.Checks...)
	report.RawLog = append(report.RawLog, firmware.RawTechnical...)
	report.RawLog = append(report.RawLog, device.RawOutput...)

	if firmware.Requirements.Chip != "" {
		if device.Capabilities.Chip == "" {
			report.Checks = append(report.Checks, CheckResult{
				Code:      "chip_unknown",
				Status:    StatusUnknown,
				Title:     "无法确认设备芯片型号",
				Summary:   fmt.Sprintf("固件要求 %s，但设备探测没有返回芯片型号", firmware.Requirements.Chip),
				Technical: fmt.Sprintf("firmware chip=%q device chip is empty", firmware.Requirements.Chip),
			})
		} else if normalizeChip(firmware.Requirements.Chip) == normalizeChip(device.Capabilities.Chip) {
			report.Checks = append(report.Checks, CheckResult{
				Code:    "chip_compatible",
				Status:  StatusPass,
				Title:   "芯片型号匹配",
				Summary: fmt.Sprintf("固件与设备均为 %s", device.Capabilities.Chip),
			})
		} else {
			report.Checks = append(report.Checks, CheckResult{
				Code:      "chip_mismatch",
				Status:    StatusWarning,
				Title:     "芯片型号不匹配",
				Summary:   fmt.Sprintf("固件要求 %s，当前设备为 %s", firmware.Requirements.Chip, device.Capabilities.Chip),
				Technical: fmt.Sprintf("firmware chip=%q device chip=%q", firmware.Requirements.Chip, device.Capabilities.Chip),
			})
		}
	}
	appendFlashChecks(&report.Checks, firmware.Requirements, device.Capabilities)
	appendPSRAMChecks(&report.Checks, firmware.Requirements, device.Capabilities)
	appendSecurityChecks(&report.Checks, device.Capabilities)
	appendHardwareChecks(&report.Checks, firmware.Requirements)
	for index := range report.Checks {
		report.Checks[index] = addGuidance(report.Checks[index], firmware.Requirements, device.Capabilities)
	}

	report.Overall = overallFromChecks(report.Checks)
	return report
}

func appendSecurityChecks(checks *[]CheckResult, device DeviceCapabilities) {
	switch device.SecureBoot {
	case TriStateEnabled:
		*checks = append(*checks, CheckResult{
			Code:    "secure_boot_enabled",
			Status:  StatusWarning,
			Title:   "设备已启用安全启动",
			Summary: "普通未签名固件可能被设备拒绝启动",
		})
	case TriStateDisabled:
		*checks = append(*checks, CheckResult{
			Code:    "secure_boot_disabled",
			Status:  StatusPass,
			Title:   "安全启动未启用",
			Summary: "设备未启用 Secure Boot",
		})
	default:
		*checks = append(*checks, CheckResult{
			Code:    "secure_boot_unknown",
			Status:  StatusUnknown,
			Title:   "无法确认安全启动状态",
			Summary: "设备未返回可靠的 Secure Boot 状态",
		})
	}

	switch device.FlashEncryption {
	case TriStateEnabled:
		*checks = append(*checks, CheckResult{
			Code:    "flash_encryption_enabled",
			Status:  StatusWarning,
			Title:   "设备已启用 Flash 加密",
			Summary: "普通明文固件可能无法在该设备上启动",
		})
	case TriStateDisabled:
		*checks = append(*checks, CheckResult{
			Code:    "flash_encryption_disabled",
			Status:  StatusPass,
			Title:   "Flash 加密未启用",
			Summary: "设备未启用 Flash Encryption",
		})
	default:
		*checks = append(*checks, CheckResult{
			Code:    "flash_encryption_unknown",
			Status:  StatusUnknown,
			Title:   "无法确认 Flash 加密状态",
			Summary: "设备未返回可靠的 Flash Encryption 状态",
		})
	}
}

func appendHardwareChecks(checks *[]CheckResult, firmware FirmwareRequirements) {
	version := strings.TrimSpace(firmware.HardwareVersion)
	if version == "" {
		return
	}
	*checks = append(*checks, CheckResult{
		Code:      "hardware_unknown",
		Status:    StatusUnknown,
		Title:     "无法自动确认主板硬件版本",
		Summary:   fmt.Sprintf("固件面向 %s，但通用 USB 探测无法区分主板 V3/V4", version),
		Technical: fmt.Sprintf("required hardware=%q; generic USB probe has no board revision", version),
	})
}

func appendFlashChecks(checks *[]CheckResult, firmware FirmwareRequirements, device DeviceCapabilities) {
	if firmware.MinimumFlashBytes == 0 {
		return
	}
	if device.FlashBytes == 0 {
		*checks = append(*checks, CheckResult{
			Code:      "flash_unknown",
			Status:    StatusUnknown,
			Title:     "无法确认设备 Flash 容量",
			Summary:   fmt.Sprintf("固件至少需要 %s Flash，但设备探测没有返回可靠容量", formatBytes(firmware.MinimumFlashBytes)),
			Technical: fmt.Sprintf("required flash=%d device flash=0", firmware.MinimumFlashBytes),
		})
		return
	}
	if device.FlashBytes < firmware.MinimumFlashBytes {
		*checks = append(*checks, CheckResult{
			Code:    "flash_too_small",
			Status:  StatusWarning,
			Title:   "设备 Flash 容量不足",
			Summary: fmt.Sprintf("设备只有 %s Flash，固件至少需要 %s", formatBytes(device.FlashBytes), formatBytes(firmware.MinimumFlashBytes)),
			Technical: fmt.Sprintf("device flash=%d required flash=%d", device.FlashBytes,
				firmware.MinimumFlashBytes),
		})
		return
	}
	*checks = append(*checks, CheckResult{
		Code:    "flash_compatible",
		Status:  StatusPass,
		Title:   "Flash 容量满足要求",
		Summary: fmt.Sprintf("设备 Flash 为 %s，固件至少需要 %s", formatBytes(device.FlashBytes), formatBytes(firmware.MinimumFlashBytes)),
	})
}

func appendPSRAMChecks(checks *[]CheckResult, firmware FirmwareRequirements, device DeviceCapabilities) {
	if firmware.RequiredPSRAMBytes == 0 {
		return
	}
	if !device.PSRAMKnown {
		*checks = append(*checks, CheckResult{
			Code:      "psram_unknown",
			Status:    StatusUnknown,
			Title:     "无法确认 PSRAM",
			Summary:   fmt.Sprintf("固件需要 %s PSRAM，但设备未报告可靠容量", formatBytes(firmware.RequiredPSRAMBytes)),
			Technical: fmt.Sprintf("required psram=%d device psram known=false", firmware.RequiredPSRAMBytes),
		})
		return
	}
	if device.PSRAMBytes < firmware.RequiredPSRAMBytes {
		*checks = append(*checks, CheckResult{
			Code:    "psram_too_small",
			Status:  StatusWarning,
			Title:   "设备 PSRAM 容量不足",
			Summary: fmt.Sprintf("设备只有 %s PSRAM，固件至少需要 %s", formatBytes(device.PSRAMBytes), formatBytes(firmware.RequiredPSRAMBytes)),
			Technical: fmt.Sprintf("device psram=%d required psram=%d", device.PSRAMBytes,
				firmware.RequiredPSRAMBytes),
		})
	} else {
		*checks = append(*checks, CheckResult{
			Code:    "psram_compatible",
			Status:  StatusPass,
			Title:   "PSRAM 容量满足要求",
			Summary: fmt.Sprintf("设备 PSRAM 为 %s，固件至少需要 %s", formatBytes(device.PSRAMBytes), formatBytes(firmware.RequiredPSRAMBytes)),
		})
	}

	if firmware.RequiresOctalPSRAM {
		switch strings.ToLower(strings.TrimSpace(device.PSRAMMode)) {
		case "":
			*checks = append(*checks, CheckResult{
				Code:      "psram_mode_unknown",
				Status:    StatusUnknown,
				Title:     "无法确认 PSRAM 模式",
				Summary:   "固件需要 octal PSRAM，但设备没有报告可靠的总线模式",
				Technical: "required psram mode=octal device psram mode is empty",
			})
		case "octal":
			*checks = append(*checks, CheckResult{
				Code:    "psram_mode_compatible",
				Status:  StatusPass,
				Title:   "PSRAM 模式满足要求",
				Summary: "设备使用固件要求的 octal PSRAM",
			})
		default:
			*checks = append(*checks, CheckResult{
				Code:      "psram_mode_mismatch",
				Status:    StatusWarning,
				Title:     "PSRAM 模式不匹配",
				Summary:   fmt.Sprintf("固件需要 octal PSRAM，设备报告为 %s", device.PSRAMMode),
				Technical: fmt.Sprintf("required psram mode=octal device psram mode=%q", device.PSRAMMode),
			})
		}
	}
}

func formatBytes(size uint64) string {
	const (
		kilobyte = uint64(1024)
		megabyte = 1024 * kilobyte
		gigabyte = 1024 * megabyte
	)
	for _, unit := range []struct {
		size uint64
		name string
	}{
		{size: gigabyte, name: "GB"},
		{size: megabyte, name: "MB"},
		{size: kilobyte, name: "KB"},
	} {
		if size >= unit.size && size%unit.size == 0 {
			return fmt.Sprintf("%d %s", size/unit.size, unit.name)
		}
	}
	return fmt.Sprintf("%d bytes", size)
}

func normalizeChip(chip string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, chip)
}

func overallFromChecks(checks []CheckResult) string {
	hasWarning := false
	hasUnknown := false
	for _, check := range checks {
		switch check.Status {
		case StatusError:
			return "error"
		case StatusWarning:
			hasWarning = true
		case StatusUnknown:
			hasUnknown = true
		}
	}
	if hasWarning {
		return "risk"
	}
	if hasUnknown {
		return "unknown"
	}
	return "compatible"
}

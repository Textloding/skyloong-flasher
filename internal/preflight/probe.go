package preflight

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Textloding/skyloong-flasher/internal/esptoolcmd"
	"github.com/Textloding/skyloong-flasher/internal/processutil"
	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

const probeCommandTimeout = 20 * time.Second

type probeCommandOutput struct {
	stdout string
	stderr string
}

type probeCommandRunner func(context.Context, runtimekit.Status, []string) (probeCommandOutput, error)

var runProbeCommand probeCommandRunner = executeProbeCommand

var (
	flashSizePattern = regexp.MustCompile(`(?i)Detected flash size:\s*([0-9]+(?:\.[0-9]+)?)\s*(KB|MB|GB)`)
	psramPattern     = regexp.MustCompile(`(?i)(Embedded\s+PSRAM)\s+([0-9]+(?:\.[0-9]+)?)\s*(KB|MB|GB)(?:\s*\(([^)]*)\))?`)
)

func ProbeDevice(ctx context.Context, status runtimekit.Status, port string, baud int, log func(string)) DeviceProbe {
	probe := DeviceProbe{
		Capabilities: DeviceCapabilities{
			SecureBoot:      TriStateUnknown,
			FlashEncryption: TriStateUnknown,
		},
		Checks:    make([]CheckResult, 0, 2),
		RawOutput: make([]string, 0),
	}
	port = strings.TrimSpace(port)
	if port == "" {
		probe.Checks = append(probe.Checks, CheckResult{
			Code:      "port_missing",
			Status:    StatusError,
			Title:     "未选择串口",
			Summary:   "没有选择设备串口",
			Technical: "port is empty",
		})
		return probe
	}
	if !status.Available {
		probe.Checks = append(probe.Checks, CheckResult{
			Code:      "unknown_probe_error",
			Status:    StatusError,
			Title:     "探测运行时未准备",
			Summary:   "设备探测运行时尚未准备",
			Technical: status.Message,
		})
		return probe
	}
	if baud <= 0 {
		baud = 460800
	}

	commands := []struct {
		verb    string
		title   string
		summary string
	}{
		{verb: "flash_id", title: "Flash 信息读取完成", summary: "已读取芯片与 Flash 信息"},
		{verb: "get_security_info", title: "安全状态读取完成", summary: "已读取安全启动与 Flash 加密状态"},
	}
	for _, command := range commands {
		args := []string{"-p", port, "-b", strconv.Itoa(baud), command.verb}
		commandContext, cancel := context.WithTimeout(ctx, probeCommandTimeout)
		output, err := runProbeCommand(commandContext, status, args)
		timedOut := errors.Is(commandContext.Err(), context.DeadlineExceeded)
		cancel()

		combined := combineProbeOutput(output)
		lines := splitOutputLines(combined)
		probe.RawOutput = append(probe.RawOutput, lines...)
		for _, line := range lines {
			if log != nil {
				log(line)
			}
		}
		mergeDeviceCapabilities(&probe.Capabilities, parseEsptoolOutput(combined))

		if err != nil {
			if timedOut {
				err = context.DeadlineExceeded
			}
			probe.Checks = append(probe.Checks, classifyProbeError(combined, err))
			break
		}
		probe.Checks = append(probe.Checks, CheckResult{
			Code:    command.verb,
			Status:  StatusPass,
			Title:   command.title,
			Summary: command.summary,
		})
	}
	probe.Capabilities.RawOutput = append([]string(nil), probe.RawOutput...)
	return probe
}

func executeProbeCommand(ctx context.Context, status runtimekit.Status, args []string) (probeCommandOutput, error) {
	built, err := esptoolcmd.Build(status, args...)
	if err != nil {
		return probeCommandOutput{}, err
	}
	command := processutil.CommandContext(ctx, built.Path, built.Args[1:]...)
	command.Env = built.Env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	return probeCommandOutput{stdout: stdout.String(), stderr: stderr.String()}, err
}

func combineProbeOutput(output probeCommandOutput) string {
	stdout := strings.TrimRight(output.stdout, "\r\n")
	stderr := strings.TrimRight(output.stderr, "\r\n")
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	default:
		return stdout + "\n" + stderr
	}
}

func mergeDeviceCapabilities(target *DeviceCapabilities, parsed DeviceCapabilities) {
	if parsed.Chip != "" {
		target.Chip = parsed.Chip
	}
	if parsed.Revision != "" {
		target.Revision = parsed.Revision
	}
	if parsed.FlashBytes != 0 {
		target.FlashBytes = parsed.FlashBytes
		target.FlashDescription = parsed.FlashDescription
	}
	if parsed.PSRAMKnown {
		target.PSRAMKnown = true
		target.PSRAMBytes = parsed.PSRAMBytes
		target.PSRAMDescription = parsed.PSRAMDescription
		target.PSRAMMode = parsed.PSRAMMode
	}
	if parsed.SecureBoot != TriStateUnknown {
		target.SecureBoot = parsed.SecureBoot
	}
	if parsed.FlashEncryption != TriStateUnknown {
		target.FlashEncryption = parsed.FlashEncryption
	}
}

func parseEsptoolOutput(output string) DeviceCapabilities {
	capabilities := DeviceCapabilities{
		SecureBoot:      TriStateUnknown,
		FlashEncryption: TriStateUnknown,
		RawOutput:       splitOutputLines(output),
	}
	for _, rawLine := range capabilities.RawOutput {
		line := strings.TrimSpace(rawLine)
		parseChipLine(line, &capabilities)
		parseFlashLine(line, &capabilities)
		parsePSRAMLine(line, &capabilities)
		parseSecurityLine(line, &capabilities)
	}
	return capabilities
}

func parseChipLine(line string, capabilities *DeviceCapabilities) {
	const prefix = "Chip is "
	if !strings.HasPrefix(line, prefix) {
		return
	}
	chipAndPackage := strings.TrimPrefix(line, prefix)
	revisionIndex := strings.LastIndex(chipAndPackage, " (revision ")
	if revisionIndex < 0 || !strings.HasSuffix(chipAndPackage, ")") {
		return
	}
	capabilities.Revision = strings.TrimSuffix(chipAndPackage[revisionIndex+len(" (revision "):], ")")
	chip := strings.TrimSpace(chipAndPackage[:revisionIndex])
	if packageIndex := strings.Index(chip, " ("); packageIndex >= 0 {
		chip = chip[:packageIndex]
	}
	capabilities.Chip = strings.TrimSpace(chip)
}

func parseFlashLine(line string, capabilities *DeviceCapabilities) {
	match := flashSizePattern.FindStringSubmatch(line)
	if match == nil {
		return
	}
	if size, ok := parseCapacity(match[1], match[2]); ok {
		capabilities.FlashBytes = size
		capabilities.FlashDescription = match[1] + strings.ToUpper(match[2])
	}
}

func parsePSRAMLine(line string, capabilities *DeviceCapabilities) {
	match := psramPattern.FindStringSubmatch(line)
	if match == nil {
		return
	}
	size, ok := parseCapacity(match[2], match[3])
	if !ok {
		return
	}
	capabilities.PSRAMKnown = true
	capabilities.PSRAMBytes = size
	capabilities.PSRAMDescription = strings.Join(strings.Fields(match[1]), " ")
	capabilities.PSRAMMode = normalizePSRAMMode(match[4])
}

func parseSecurityLine(line string, capabilities *DeviceCapabilities) {
	name, value, found := strings.Cut(line, ":")
	if !found {
		return
	}
	state := parseTriState(value)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "secure boot":
		capabilities.SecureBoot = state
	case "flash encryption":
		capabilities.FlashEncryption = state
	}
}

func parseCapacity(value, unit string) (uint64, bool) {
	size, err := strconv.ParseFloat(value, 64)
	if err != nil || size < 0 {
		return 0, false
	}
	multiplier := float64(1)
	switch strings.ToUpper(unit) {
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0, false
	}
	return uint64(size * multiplier), true
}

func normalizePSRAMMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(normalized, "ap_"), strings.Contains(normalized, "octal"), strings.Contains(normalized, "opi"):
		return "octal"
	case strings.Contains(normalized, "quad"), strings.Contains(normalized, "qspi"):
		return "quad"
	default:
		return ""
	}
}

func parseTriState(value string) TriState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "enabled":
		return TriStateEnabled
	case "disabled":
		return TriStateDisabled
	default:
		return TriStateUnknown
	}
}

func classifyProbeError(output string, err error) CheckResult {
	technical := probeTechnical(output, err)
	normalized := strings.ToLower(technical)
	check := CheckResult{Status: StatusError, Technical: technical}

	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(normalized, "timed out"), strings.Contains(normalized, "timeout"):
		check.Code = "timeout"
		check.Title = "设备探测超时"
		check.Summary = "设备探测超时"
	case containsAny(normalized, "access is denied", "permissionerror", "resource busy", "port is busy", "exclusively lock port"):
		check.Code = "port_busy"
		check.Title = "串口被占用"
		check.Summary = "串口正被其他程序占用"
	case containsAny(normalized, "cannot find the file specified", "no such file", "file not found", "port doesn't exist", "port does not exist"):
		check.Code = "port_missing"
		check.Title = "串口不存在"
		check.Summary = "没有找到所选串口"
	case containsAny(normalized, "wrong boot mode", "needs to be in download mode", "not in download mode"):
		check.Code = "download_mode_required"
		check.Title = "需要下载模式"
		check.Summary = "设备未进入下载模式"
	case containsAny(normalized, "failed to connect", "no serial data received", "serial data stream stopped"):
		check.Code = "connection_failed"
		check.Title = "设备连接失败"
		check.Summary = "无法连接设备"
	default:
		check.Code = "unknown_probe_error"
		check.Title = "设备探测失败"
		check.Summary = "设备探测失败"
	}
	return check
}

func probeTechnical(output string, err error) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(output) != "" {
		parts = append(parts, strings.TrimRight(output, "\r\n"))
	}
	if err != nil {
		parts = append(parts, fmt.Sprintf("error: %v", err))
	}
	return strings.Join(parts, "\n")
}

func splitOutputLines(output string) []string {
	if output == "" {
		return nil
	}
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return []string{""}
	}
	return strings.Split(normalized, "\n")
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

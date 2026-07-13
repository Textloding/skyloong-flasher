package preflight

import (
	"fmt"
	"strings"
)

func addGuidance(check CheckResult, firmware FirmwareRequirements, device DeviceCapabilities) CheckResult {
	if check.Status == StatusPass {
		return check
	}

	switch check.Code {
	case "chip_mismatch":
		check.Resolution = fmt.Sprintf("固件目标芯片为 %s，当前设备为 %s；芯片型号不一致时固件不能可靠启动。", firmware.Chip, device.Chip)
		check.Steps = []string{
			"核对当前连接的设备是否为要刷写的屏幕",
			"重新选择与设备芯片型号一致的固件包",
			"返回工具点击“重新检测”",
		}
	case "chip_unknown":
		check.Resolution = fmt.Sprintf("设备探测没有返回芯片型号，当前无法确认是否满足固件要求的 %s。", firmware.Chip)
		check.Steps = []string{
			"查看原始检测日志是否包含 Chip is 信息",
			"按 BOOT/下载模式顺序重新连接屏幕",
			"返回工具点击“重新检测”",
		}
	case "flash_too_small":
		check.Resolution = fmt.Sprintf("设备 Flash 实际容量为 %s，固件要求至少 %s；降低波特率不能解决硬件容量不足。", formatBytes(device.FlashBytes), formatBytes(firmware.MinimumFlashBytes))
		check.Steps = []string{
			"选择最低 Flash 要求不超过设备实际容量的固件包",
			"或更换 Flash 容量满足固件要求的设备",
			"返回工具点击“重新检测”",
		}
	case "flash_unknown":
		check.Resolution = fmt.Sprintf("设备探测没有返回可靠的 Flash 容量，当前无法确认是否满足固件要求的 %s。", formatBytes(firmware.MinimumFlashBytes))
		check.Steps = []string{
			"查看原始检测日志是否包含 Detected flash size 信息",
			"按 BOOT/下载模式顺序重新连接屏幕",
			"返回工具点击“重新检测”",
		}
	case "psram_too_small":
		check.Resolution = fmt.Sprintf("设备 PSRAM 实际容量为 %s，固件要求至少 %s；这是硬件容量限制。", formatBytes(device.PSRAMBytes), formatBytes(firmware.RequiredPSRAMBytes))
		check.Steps = []string{
			"选择 PSRAM 要求不超过设备实际容量的固件包",
			"或更换 PSRAM 容量满足要求的主板",
			"返回工具点击“重新检测”",
		}
	case "psram_unknown":
		check.Resolution = fmt.Sprintf("当前探测没有返回可靠的 PSRAM 容量，无法确认是否满足固件要求的 %s；未知不等于设备没有 PSRAM。", formatBytes(firmware.RequiredPSRAMBytes))
		check.Steps = []string{
			"记录当前固件包名称和主板版本",
			"继续刷写后使用 115200 波特率收集完整启动日志",
			"将启动日志交给固件提供方确认 PSRAM 初始化结果",
		}
	case "psram_mode_mismatch":
		mode := strings.TrimSpace(device.PSRAMMode)
		if mode == "" {
			mode = "未知模式"
		}
		check.Resolution = fmt.Sprintf("固件要求 octal PSRAM，设备报告为 %s；PSRAM 总线模式不一致可能导致启动失败或反复重启。", mode)
		check.Steps = []string{
			"核对主板规格中的 PSRAM 总线模式",
			"选择明确支持该 PSRAM 模式的固件包",
			"返回工具点击“重新检测”",
		}
	case "psram_mode_unknown":
		check.Resolution = "固件要求 octal PSRAM，但当前探测无法确认设备的 PSRAM 总线模式；未知不能按不匹配处理。"
		check.Steps = []string{
			"核对主板规格或丝印标注的 PSRAM 类型",
			"继续刷写后使用 115200 波特率收集 PSRAM 初始化日志",
			"将日志交给固件提供方确认是否为 octal 模式",
		}
	case "secure_boot_enabled":
		check.Resolution = "设备已启用 Secure Boot，普通未签名固件可能被 bootloader 拒绝；刷机工具不能绕过签名策略。"
		check.Steps = []string{
			"确认设备来源和当前 Secure Boot 密钥策略",
			"联系固件提供方获取使用匹配密钥签名的固件",
			"使用匹配固件后返回工具点击“重新检测”",
		}
	case "flash_encryption_enabled":
		check.Resolution = "设备已启用 Flash Encryption，普通明文固件可能无法启动；刷机工具不能替代设备的加密密钥策略。"
		check.Steps = []string{
			"确认设备来源和当前 Flash Encryption 密钥策略",
			"联系固件提供方获取与设备加密配置匹配的固件",
			"使用匹配固件后返回工具点击“重新检测”",
		}
	case "secure_boot_unknown":
		check.Resolution = "设备没有返回可靠的 Secure Boot 状态，当前只能标记为无法确认，不能据此判断已启用或已关闭。"
		check.Steps = []string{
			"查看原始检测日志中的 get_security_info 输出",
			"重新进入 BOOT/下载模式后点击“重新检测”",
			"仍无法读取时联系固件提供方确认签名要求",
		}
	case "flash_encryption_unknown":
		check.Resolution = "设备没有返回可靠的 Flash Encryption 状态，当前只能标记为无法确认，不能据此判断已启用或已关闭。"
		check.Steps = []string{
			"查看原始检测日志中的 get_security_info 输出",
			"重新进入 BOOT/下载模式后点击“重新检测”",
			"仍无法读取时联系固件提供方确认加密要求",
		}
	case "hardware_unknown", "hardware_mismatch":
		version := strings.TrimSpace(firmware.HardwareVersion)
		if version == "" {
			version = "目标版本"
		}
		check.Resolution = fmt.Sprintf("固件面向 %s；主板 V3/V4 显示接线不同，通用 USB 探测不能自动区分硬件版本，反复刷机不能修复接线差异。", version)
		check.Steps = []string{
			"查看主板 PCB 上的主板丝印或版本标签，确认是 V3.0 还是 V4.0",
			fmt.Sprintf("选择与主板丝印 %s 对应的固件包", version),
			"返回工具点击“重新检测”",
		}
	case "port_busy":
		check.Resolution = "所选串口正在被其他程序占用，工具必须独占串口才能完成设备探测。"
		check.Steps = []string{
			"关闭串口助手、Arduino/PlatformIO监视器等占用串口的程序",
			"确认其他刷机工具没有继续使用所选 COM 串口",
			"返回工具点击“重新检测”",
		}
	case "port_missing":
		check.Resolution = "系统中没有找到所选 COM 串口，可能是设备已断开、串口号已变化或 USB 线不支持数据传输。"
		check.Steps = []string{
			"使用支持数据传输的 USB 线重新连接屏幕",
			"在串口列表中选择重新连接后新出现的 COM 串口",
			"返回工具点击“重新检测”",
		}
	case "connection_failed":
		check.Resolution = "设备没有完成 ROM 下载模式握手，请按顺序重新进入 BOOT/下载模式。"
		check.Steps = downloadModeSteps()
	case "download_mode_required":
		check.Resolution = "设备当前不在可刷写的下载模式，请按顺序使用 BOOT/下载键重新连接。"
		check.Steps = downloadModeSteps()
	case "timeout":
		check.Resolution = "设备在限定时间内没有完成探测，常见原因是未进入下载模式或串口连接不稳定。"
		check.Steps = downloadModeSteps()
	case "unknown_probe_error":
		check.Title = "设备探测失败"
		check.Summary = "设备探测失败，当前错误无法归入已知类型"
		check.Resolution = "底层探测返回了未识别错误；原始 esptool 输出已保留，可据此继续定位。"
		check.Steps = []string{
			"查看原始检测日志并记录完整 esptool 输出",
			"重新连接屏幕并选择新出现的 COM 串口",
			"返回工具点击“重新检测”",
		}
	case "firmware_analysis_missing":
		check.Resolution = "当前没有可用于比较的固件包分析结果，需要先选择并完成固件包解析。"
		check.Steps = []string{
			"选择要刷写的完整固件压缩包",
			"等待工具完成固件文件与分区解析",
			"返回工具点击“重新检测”",
		}
	case "flash_file_offset", "flash_file_size":
		check.Resolution = "固件包中的写入地址或文件大小无效，继续使用可能造成文件缺失或写入范围错误。"
		check.Steps = []string{
			"查看检测详情中标出的文件路径和无效数值",
			"从固件提供方重新下载未经修改的完整固件包",
			"重新选择固件包并点击“重新检测”",
		}
	case "partition_table":
		check.Resolution = "分区表缺失、不可读取或无法解析时，工具无法可靠确认固件布局和最低 Flash 容量。"
		check.Steps = []string{
			"确认固件包包含写入地址 0x8000 对应的分区表文件",
			"从固件提供方重新下载未经修改的完整固件包",
			"重新选择固件包并点击“重新检测”",
		}
	case "missing_flash_file":
		check.Resolution = "固件包缺少必需写入位置对应的文件，不能组成完整的启动镜像。"
		check.Steps = []string{
			"查看检测详情中的缺失写入地址",
			"重新下载完整固件包，确认包含 Bootloader、分区表、OTA 数据和应用镜像",
			"重新选择固件包并点击“重新检测”",
		}
	case "flash_region_overflow", "flash_region_overlap", "partition_overlap":
		check.Resolution = "固件文件或分区的地址范围无效，继续刷写可能覆盖其他区域或产生不可启动的布局。"
		check.Steps = []string{
			"查看检测详情中发生溢出或重叠的文件与地址",
			"停止使用经过混合、改名或手工修改的固件文件",
			"下载与当前设备型号匹配的完整固件包后重新检测",
		}
	case "app_partition", "app_image_too_large":
		check.Resolution = "应用镜像没有匹配的 app 分区或超过分区容量，当前分区布局无法完整容纳该固件。"
		check.Steps = []string{
			"查看检测详情中的应用镜像大小和 app 分区容量",
			"选择带有匹配分区表的固件包或容量更小的应用镜像",
			"重新选择固件包并点击“重新检测”",
		}
	case "firmware_metadata":
		check.Resolution = "固件包没有声明完整的硬件版本、Flash 或 PSRAM 要求，因此部分兼容性只能标记为未知。"
		check.Steps = []string{
			"核对固件包发布说明中的主板版本和内存要求",
			"优先选择包含完整兼容性元数据的官方固件包",
			"重新选择固件包并点击“重新检测”",
		}
	}
	fillFallbackGuidance(&check)

	return check
}

func fillFallbackGuidance(check *CheckResult) {
	if strings.TrimSpace(check.Resolution) == "" {
		switch check.Status {
		case StatusWarning:
			check.Resolution = "该检测项已确认存在兼容性风险，请先根据检测摘要核对固件包和设备。"
		case StatusUnknown:
			check.Resolution = "当前信息不足以确认该检测项，请结合原始日志和固件说明继续核对。"
		case StatusError:
			check.Resolution = "检测过程未完成，请根据检测摘要和原始日志排除问题后重试。"
		}
	}
	if len(check.Steps) != 0 {
		return
	}
	switch check.Status {
	case StatusWarning:
		check.Steps = []string{
			"查看该检测项的摘要和原始技术信息",
			"更换与设备要求匹配的固件包或硬件",
			"返回工具点击“重新检测”",
		}
	case StatusUnknown:
		check.Steps = []string{
			"查看原始检测日志并记录当前固件包与设备信息",
			"核对固件发布说明或联系固件提供方确认要求",
			"返回工具点击“重新检测”",
		}
	case StatusError:
		check.Steps = []string{
			"查看原始检测日志中的具体错误",
			"排除连接、串口或固件包问题",
			"返回工具点击“重新检测”",
		}
	}
}

func downloadModeSteps() []string {
	return []string{
		"断开屏幕的 USB 连接",
		"按住 BOOT/下载键并重新插入 USB",
		"看到新的 COM 串口后松开 BOOT/下载键",
		"返回工具点击“重新检测”",
	}
}

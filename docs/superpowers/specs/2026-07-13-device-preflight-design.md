# 固件包与设备兼容性检测设计

## 目标

在真正写入 Flash 之前，同时分析固件压缩包和已连接设备，给普通用户展示可理解的中文检测结果。检测功能只提供信息，不禁止刷机、不擦除设备数据，也不改变现有刷机参数。

检测需要回答三个问题：

1. 固件包本身是否完整、分区和文件尺寸是否合理。
2. 设备的芯片、Flash、PSRAM 和安全状态能否满足固件要求。
3. 无法确认的项目具体是什么，以及用户下一步应检查什么。

## 非目标

- 不根据检测结果禁用“开始刷机”。
- 不自动执行 `erase_flash` 或清除 NVS、LittleFS。
- 不把无法读取的信息误判为不兼容。
- 不声称仅凭 ESP32-S3 下载模式 USB ID 就能识别 SCM V3.0 或 V4.0 屏幕硬件。
- 本阶段不实现刷写后的串口启动日志诊断。

## 检测流程

### 1. 固件包静态检测

压缩包解析完成后立即生成固件需求：

- 目标芯片，优先读取 `flasher_args.json` 的 `extra_esptool_args.chip`。
- bootloader、partition table、OTA data 和应用固件是否存在。
- 每个写入文件的偏移、尺寸和结束地址。
- 写入区域是否互相重叠。
- ESP-IDF 分区表是否可解析，分区是否重叠或越界。
- 应用固件是否能放入目标 app 分区。
- 最高分区结束地址，由此计算最低 Flash 容量。
- 固件声明的 Flash 模式、频率和容量参数。
- 固件硬件版本。优先读取包内兼容性元数据；源码包可从 `FIRMWARE_VERSION` 辅助识别 SCM V3.0/V4.0；无法识别时返回“未知”。

分区表按照 ESP-IDF 32 字节条目格式解析。解析失败不阻止刷机，但必须显示原始文件和失败原因。

### 2. 设备实时检测

用户选择串口后自动检测，也保留“重新检测”按钮。检测过程只运行只读 esptool 命令：

- ROM 握手确认实际芯片型号和修订版本。
- `flash_id` 读取 Flash 厂商、设备 ID 和容量。
- 从 esptool 芯片特征输出读取可识别的嵌入式 PSRAM 类型和容量。
- `get_security_info` 读取 Secure Boot、Flash Encryption 和下载模式限制。

部分 esptool 或芯片批次无法可靠报告 PSRAM。此时结果必须为“无法确认”，不能显示“没有 PSRAM”。检测失败后保留串口和固件包信息，允许用户直接刷机。

### 3. 兼容性对比

每个检测项返回以下状态之一：

- `pass`：已确认满足要求。
- `warning`：已确认存在风险，但仍允许刷机。
- `unknown`：当前检测方法无法确认。
- `error`：检测过程自身失败，例如串口被占用。

综合状态只用于展示：

- `兼容`：关键项目均通过。
- `存在风险`：至少一个项目为 warning。
- `部分信息无法确认`：没有 warning，但存在 unknown。
- `检测失败`：设备检测命令无法完成。

无论综合状态如何，现有“开始刷机”按钮都不因检测结果而禁用。

## 数据模型

Go 后端新增独立的 `internal/preflight` 包，避免把检测逻辑混入刷写执行器。

```go
type FirmwareRequirements struct {
    Chip               string
    HardwareVersion    string
    MinimumFlashBytes  uint64
    RequiredPSRAMBytes uint64
    RequiresOctalPSRAM bool
    FlashFiles         []FlashRegion
    Partitions         []Partition
}

type DeviceCapabilities struct {
    Chip              string
    Revision          string
    FlashBytes        uint64
    FlashDescription  string
    PSRAMBytes        uint64
    PSRAMDescription  string
    PSRAMKnown        bool
    SecureBoot        TriState
    FlashEncryption   TriState
    RawOutput         []string
}

type CheckResult struct {
    Code       string
    Status     string
    Title      string
    Summary    string
    Resolution string
    Steps      []string
    Technical  string
}

type Report struct {
    Overall     string
    Firmware    FirmwareRequirements
    Device      DeviceCapabilities
    Checks      []CheckResult
    RawLog      []string
}
```

`app.go` 暴露 `DetectCompatibility`，使用当前已解析固件和用户选择的串口生成报告。切换固件包或串口时废弃旧报告，避免把上一台设备的结果带到当前任务。

## 中文错误映射

底层保留 esptool 原文，但主界面只展示中文原因和操作建议。至少覆盖：

| 原始情况 | 中文结果 | 建议 |
| --- | --- | --- |
| 串口不存在 | 没有找到所选串口 | 重新扫描设备并确认 USB 线支持数据传输 |
| access denied / port busy | 串口正在被其他程序占用 | 关闭串口助手、监视器或其他刷机软件后重试 |
| failed to connect / no serial data | 设备没有进入下载模式 | 按住 BOOT 后重新插入，再点击重新检测 |
| wrong boot mode | 当前不是可刷机模式 | 按设备说明进入 BOOT/下载模式 |
| chip mismatch | 固件芯片与设备不一致 | 检查是否选择了错误设备或固件包 |
| detected flash 小于最低需求 | 设备 Flash 容量不足 | 当前固件分区需要更大 Flash，继续刷写可能无法启动 |
| PSRAM 缺失或容量不足 | 设备 PSRAM 不满足固件要求 | 该固件可能在启动阶段停止或反复重启 |
| PSRAM 无法读取 | 无法确认 PSRAM | 可继续刷机，但黑屏时需要查看 115200 波特率启动日志 |
| Flash Encryption 开启 | 设备已启用 Flash 加密 | 普通明文固件可能无法启动，请确认设备来源和密钥策略 |
| Secure Boot 开启 | 设备已启用安全启动 | 未签名固件可能被 bootloader 拒绝 |
| 缺少 bin | 固件包不完整 | 重新下载完整固件包 |
| app 超过分区 | 应用固件放不进目标分区 | 使用匹配的分区表或更小的应用固件 |
| 分区重叠/越界 | 固件分区布局无效 | 不建议继续，检查固件包是否损坏或混入其他型号文件 |
| V3/V4 无法识别 | 无法自动确认屏幕硬件版本 | 对照刷机前固件版本或硬件标签确认 |

错误匹配使用稳定的错误码和关键字组合，不直接把整段英文替换成一句模糊提示。无法识别的错误显示“设备检测失败”，并附上原始 esptool 输出。

## 解决指导

检测不能只说明“哪里不对”，还必须尽量把用户带回可刷机状态。每个 `warning`、`unknown` 和 `error` 项都提供一个简短结论和按顺序执行的步骤：

- 串口问题：指出需要关闭哪些常见串口程序、如何重新扫描，以及重新插拔后应选择哪个 COM 口。
- 下载模式问题：明确写出“断开 USB、按住 BOOT、重新插入、松开 BOOT、点击重新检测”的顺序。
- 固件包问题：列出缺失或尺寸异常的具体文件，并给出完整包应包含的文件清单。
- 芯片或硬件版本问题：说明应换哪个固件版本，V3.0 与 V4.0 不兼容时明确提示重复刷机不能解决。
- Flash 容量不足：显示设备容量、固件最低需求，并说明只能更换匹配固件或容量足够的设备，不能通过降低波特率解决。
- PSRAM 问题：区分“确认不足”和“无法读取”；确认不足时说明属于硬件限制，未知时指导用户继续刷写后收集 115200 波特率启动日志。
- 安全启动或 Flash 加密：说明普通固件不能直接解决，并指导用户联系固件提供方获取签名/加密匹配的固件。

步骤必须是用户可以直接执行的动作，不能只写“检查配置”“确认环境”这类空泛提示。完成操作后，用户可以在同一区域点击“重新检测”验证问题是否消失。硬件限制无法修复时也要给出可行选择，而不是承诺软件能够修复。

## 界面设计

在“连接屏幕”和“开始刷机”之间增加紧凑的“兼容性检测”区域：

- 顶部显示综合状态和最后检测时间。
- 固件包与设备信息采用两列对照，窄窗口改为单列。
- 下方按严重程度排列检测项，每项包含中文标题、原因和建议。
- 有解决步骤的项目显示编号操作列表，完成后可直接点击“重新检测”。
- `warning` 使用醒目的风险色，`unknown` 使用中性色，不使用只有颜色才能区分的表达。
- 提供“重新检测”和“查看原始检测日志”。
- 检测期间显示当前阶段：连接设备、读取 Flash、读取安全状态、对比固件。
- 检测结果不改变“开始刷机”按钮可用性。

## 错误与并发处理

- 同一时间只运行一个设备检测任务。
- 用户切换串口、重新解析固件或开始刷机时取消旧检测。
- 检测命令设置超时，超时后终止子进程并返回中文提示。
- 原始日志继续写入现有日志文件，前端报告只保留必要摘要。
- 检测不会修改 `a.current` 的刷机文件列表和参数。

## 测试

Go 单元测试覆盖：

- 完整与缺失固件文件。
- 分区表正常、损坏、重叠、越界。
- 应用固件超过 app 分区。
- 8MB 设备对比要求约 16MB 的固件。
- 芯片不匹配。
- PSRAM 满足、不足和未知。
- Secure Boot/Flash Encryption 三态处理。
- 常见 esptool 英文错误到中文提示的映射。
- 检测失败不修改 `CanFlash`，也不改变刷机命令。

前端测试与构建验证：

- 不同综合状态正确展示。
- 长错误原因在窄窗口不溢出。
- 切换串口后旧报告被清除。
- 检测失败时“开始刷机”仍可点击。
- `go test ./...`、`npm run build --prefix frontend` 和 `wails build` 通过。

## 已知限制

ESP32-S3 ROM 下载模式使用通用 Espressif USB 标识，无法仅凭 `VID_303A&PID_1001` 判断屏幕是 SCM V3.0 还是 V4.0。设备检测可以可靠确认芯片、Flash 和安全状态，但屏幕总线和引脚版本只有在固件或硬件提供额外标识时才能确认。界面必须诚实显示这一限制。

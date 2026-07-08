# SKYLOONG 刷机工具设计

## 目标

构建一个独立 Windows 桌面刷机工具，面向 SKYLOONG/GK87 类 ESP32-S3 彩屏模块。用户可以选择本地 zip、输入 GitHub 仓库链接或分支 zip 链接，工具自动解析固件包、检测环境、识别屏幕刷机串口，并用傻瓜式步骤引导完成刷机。

## 范围

首版支持 Windows 10/11 x64。工具不混入固件仓库，独立维护、独立发布。用户不需要手动安装 Go、Node 或 ESP-IDF。首版先实现桌面 UI、包解析、GitHub 下载、设备识别、刷机命令管线和日志展示；源码构建和完整运行时下载以任务管线方式接入，便于后续把 ESP-IDF runtime 放到 Releases 或官方下载源。

## 设备识别

工具识别两种状态：

- 运行态：常见 USB HID `VID_34BF&PID_FF0E`，名称可能是 `STC USB Keyboard` 或 HID Keyboard。这说明键盘/屏幕在线，但不能直接刷机。
- 刷机态：常见 Espressif USB Serial/JTAG `VID_303A&PID_1001`，名称可能是 `USB JTAG/serial debug unit`、`USB Composite Device` 或 USB 串行设备。工具还会通过 esptool/ROM handshake 确认目标芯片为 `esp32s3`。

如果只检测到 `COM1`，默认不推荐选择，并提示这通常不是屏幕。

## 固件来源

工具支持：

- 本地 zip：从用户选择的 zip 中递归查找 `flasher_args.json`、`flash_args` 和 bin 文件。
- GitHub 仓库链接：解析 owner/repo，读取默认分支或用户指定分支，下载仓库 zip。
- GitHub zip 链接：直接下载并解压。

解析优先级：

1. 找到 `flasher_args.json` 且所有 bin 存在，进入“可直接刷机”状态。
2. 找到 `flash_args` 且所有 bin 存在，进入“可直接刷机”状态。
3. 找到 ESP-IDF 源码结构，进入“需要构建”状态。
4. 无法识别时展示中文错误和建议的包结构。

## 刷写参数

对 SKYLOONG 固件包，常见参数为：

- `--chip esp32s3`
- `--flash_mode dio`
- `--flash_freq 80m`
- `--flash_size detect`
- `0x0 bootloader/bootloader.bin`
- `0x8000 partition_table/partition-table.bin`
- `0x10000 ota_data_initial.bin`
- `0x20000 GK87-Screen.bin`

工具优先读取包内 `flasher_args.json`，不硬编码单一固件名。

## UI 设计

使用 Wails + React + TypeScript 做桌面 UI。界面是深色现代玻璃质感：半透明面板、细边框、柔和阴影、清晰进度条和可展开高级日志。主流程是 5 步：

1. 选择固件来源。
2. 解析固件包。
3. 环境检查。
4. 连接屏幕。
5. 开始刷机。

普通用户只看到当前步骤、下一步按钮和明确提示；高级用户可以展开端口列表、刷机参数、命令日志。

## 后端架构

Go 后端拆为这些包：

- `internal/githubsource`：解析 GitHub 链接和下载 zip。
- `internal/packagekit`：解压、扫描、识别固件包。
- `internal/device`：扫描 Windows PnP/COM 设备，给出置信度。
- `internal/runtime`：检测 esptool/ESP-IDF runtime，后续负责下载和准备。
- `internal/flasher`：根据包内参数组装刷机任务，执行进程并推送日志。
- `app.go`：Wails 暴露给前端的 API。

## 错误处理

所有错误都转为中文用户提示，并附带可复制的技术日志。常见错误包括：

- 包内没有刷机产物。
- 源码包需要构建但 runtime 未准备。
- 未检测到刷机串口。
- 检测到运行态键盘但未进入下载模式。
- 下载失败或网络超时。
- 刷写过程中连接断开。

## 验证

首版验证包括：

- Go 单元测试覆盖 GitHub URL 解析、flasher_args 解析、包扫描、设备置信度。
- `go test ./...` 通过。
- 前端 `npm run build` 通过。
- Wails 构建或 Vite 构建通过。


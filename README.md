# SKYLOONG Flasher

SKYLOONG Flasher 是一个独立的 Windows 桌面刷机工具，面向 SKYLOONG/GK87 类 ESP32-S3 彩屏模块。它和固件仓库分开维护，目标是让普通用户通过本地 zip、GitHub 仓库链接或分支链接完成傻瓜式刷机。

截图补充计划见 [应用截图占位说明](docs/images/app-preview-note.md)。

## 当前版本

- 当前版本说明见 [2026-07-08 版本说明](docs/releases/2026-07-08.md)。
- 首版目标平台：Windows 10/11 x64。
- 生成文件：`build/bin/SKYLOONG-Flasher.exe`。

## 当前能力

- 选择本地固件 zip。
- 输入 GitHub 仓库、分支或 zip 链接并下载。
- 自动解析 `flasher_args.json` 或 `flash_args`。
- 识别 ESP-IDF 源码包；检测到本机 ESP-IDF 时可一键构建。
- 扫描 Windows COM/PnP 设备。
- 区分 SKYLOONG 键盘运行态 `VID_34BF&PID_FF0E` 和 ESP32-S3 下载态 `VID_303A&PID_1001`。
- 检测本机 esptool/ESP-IDF runtime。
- 生成并执行 esptool 刷机命令。
- 现代毛玻璃中文向导界面。
- 下载、解析、扫描和刷机时显示实时阶段、进度条和高级日志。

## 普通用户怎么用

1. 打开 `SKYLOONG-Flasher.exe`。
2. 在“选择固件来源”中选择：
   - 本地 zip；
   - GitHub 仓库链接；
   - GitHub 分支链接；
   - GitHub zip 链接。
3. 点击“解析固件”。
4. 如果工具提示“可直接刷”，按界面提示连接屏幕并进入下载模式。
5. 如果工具提示“需要构建”，工具会检测本机 ESP-IDF；检测到后可点击“构建固件”。
6. 工具检测到 ESP32-S3 刷机串口后，点击“开始刷机”。
7. 刷机完成后等待设备自动重启。

整个过程中，界面会显示当前阶段、百分比、提示文字和高级日志。用户不需要打开命令行。

## 支持的固件来源

### 本地 zip

推荐 zip 内包含 ESP-IDF 构建产物：

```text
build/flasher_args.json
build/bootloader/bootloader.bin
build/partition_table/partition-table.bin
build/ota_data_initial.bin
build/GK87-Screen.bin
```

也支持 `flash_args`。工具会递归查找这些文件，不要求它们一定在 zip 根目录。

### GitHub 仓库链接

支持类似：

```text
https://github.com/Textloding/SKYLOONG
https://github.com/Textloding/SKYLOONG/tree/idf-v5.1.4
https://github.com/Textloding/SKYLOONG/archive/refs/heads/main.zip
```

如果下载到的是源码包，工具会先识别为“需要构建”。检测到本机 ESP-IDF 时，可以在工具里一键构建。

## 设备识别说明

工具会区分两种状态：

```text
运行态：VID_34BF&PID_FF0E，可能显示为 STC USB Keyboard
下载态：VID_303A&PID_1001，可能显示为 USB JTAG/serial debug unit 或 USB Composite Device
```

运行态说明键盘/屏幕插着，但还不能刷机。需要按设备方式进入 BOOT/下载模式后重新扫描。

如果只看到 `COM1`，通常不是屏幕刷机串口。

## 进入下载模式提示

不同批次键盘进入下载模式的方式可能不同。常见方式：

- 按住 BOOT/下载键后插入 USB。
- 按住 BOOT 后点击工具里的“重新扫描设备”。
- 如果一直连接失败，换一根支持数据传输的 USB 线。

## 实时进度

工具不会弹出 PowerShell 或命令行窗口。所有等待过程都在应用内显示：

- GitHub 下载进度。
- zip 解析状态。
- ESP-IDF 构建日志。
- 设备扫描结果。
- esptool 刷机日志。

## 开发运行

```powershell
cd C:\path\to\skyloong-flasher
npm install --prefix frontend
npm run build --prefix frontend
go test ./...
```

如果已安装 Wails CLI：

```powershell
wails dev
wails build
```

## 发布建议

发布前建议执行：

```powershell
npm run build --prefix frontend
go test ./...
wails build
```

构建成功后，Windows 可执行文件位于：

```text
build/bin/SKYLOONG-Flasher.exe
```

## 开源协议

本项目以 0BSD 协议发布，详见 [LICENSE](LICENSE)。第三方工具链、Wails、React、ESP-IDF、esptool 等仍遵循各自原始许可证。

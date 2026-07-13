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
- 对固件包和下载模式设备执行只读、非阻塞的兼容性检测，并给出中文风险提示与处理步骤。
- 识别 ESP-IDF 源码包；没有本机 Git、Python、CMake、Ninja、ESP-IDF、交叉编译器、esptool 等基础环境时也会自动准备。
- 扫描 Windows COM/PnP 设备。
- 区分 SKYLOONG 键盘运行态 `VID_34BF&PID_FF0E` 和 ESP32-S3 下载态 `VID_303A&PID_1001`。
- 检测本机、缓存和离线包内置的 esptool/ESP-IDF runtime。
- 生成并执行 esptool 刷机命令。
- 现代毛玻璃中文向导界面。
- 下载、解析、扫描、构建和刷机时显示实时阶段、进度条和完整高级日志。

## 普通用户怎么用

1. 打开 `SKYLOONG-Flasher.exe`。
2. 在“选择固件来源”中选择：
   - 本地 zip；
   - GitHub 仓库链接；
   - GitHub 分支链接；
   - GitHub zip 链接。
3. 点击“解析固件”。
4. 如果工具提示“可直接刷”，按界面提示连接屏幕并进入下载模式。
5. 如果工具提示“需要构建”，直接点击“准备环境并构建”。工具会自动准备便携 Git、EIM CLI、ESP-IDF v5.1.4、Python、CMake、Ninja、交叉编译器、esptool 和构建所需组件依赖，并在界面里显示当前下载源、进度和日志。
6. 工具检测到 ESP32-S3 刷机串口后，点击“开始刷机”。
7. 刷机完成后等待设备自动重启。

整个过程中，界面会显示当前阶段、百分比、提示文字和高级日志。用户不需要打开命令行，也不需要理解 Git、Python、ESP-IDF 等技术环境。

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

如果下载到的是源码包，工具会先识别为“需要构建”。用户不需要手动安装 Git、Python、CMake、Ninja、ESP-IDF、编译器或 esptool，也不需要打开命令行。点击“准备环境并构建”后，工具会自动准备构建环境并继续构建。

首次自动准备环境会下载较多文件，耗时取决于网络。工具会先准备便携 Git，然后通过 EIM CLI 安装 ESP-IDF v5.1.4。Git 会优先使用华为云镜像，其次 npmmirror，最后才尝试 GitHub；EIM CLI 和 ESP-IDF 资源会优先使用乐鑫国内镜像 `dl.espressif.cn`，失败后自动切换备用源。准备完成后会缓存在本机，后续构建会直接复用。

源码组件下载会优先使用 `https://components-file.espressif.cn`，再回退到 `https://components-file.espressif.com`。

网络较差的用户，后续可以下载离线完整版压缩包。离线包建议把以下目录放在 exe 同级，工具启动后会优先识别这些内置运行时：

```text
SKYLOONG-Flasher.exe
runtime/
  tools/
    git/
    eim.exe
  eim-config/
  esp-idf/
```

普通在线版也会把下载好的运行时缓存到用户目录，例如 `%LOCALAPPDATA%\SkyloongFlasher\tools\git` 和 `%LOCALAPPDATA%\SkyloongFlasher\runtime`。用户不需要修改系统 PATH，也不会弹出 PowerShell。

工具启动时会主动创建这些工作文件夹：

```text
%LOCALAPPDATA%\SkyloongFlasher\downloads
%LOCALAPPDATA%\SkyloongFlasher\packages
%LOCALAPPDATA%\SkyloongFlasher\runtime
%LOCALAPPDATA%\SkyloongFlasher\tools
%LOCALAPPDATA%\SkyloongFlasher\logs
C:\SLCM
C:\P
```

其中 `C:\SLCM` 用于 ESP-IDF 组件缓存，`C:\P` 用于源码包解压和构建工作区，目录名故意很短，用来避开 Windows 路径长度限制。如果根目录不可写，工具会自动尝试备用目录，并在高级日志里记录实际使用的目录。部分 ESP-IDF 组件包会把不参与固件构建的测试 build 产物也打进包内，工具会在 Python 文件写入阶段启用 Windows 长路径前缀，确保组件包完整解压并通过校验。

## 设备识别说明

工具会区分两种状态：

```text
运行态：VID_34BF&PID_FF0E，可能显示为 STC USB Keyboard
下载态：VID_303A&PID_1001，可能显示为 USB JTAG/serial debug unit 或 USB Composite Device
```

运行态说明键盘/屏幕插着，但还不能刷机。需要按设备方式进入 BOOT/下载模式后重新扫描。

如果只看到 `COM1`，通常不是屏幕刷机串口。

## 刷机前兼容性检测

解析固件包并选择下载模式串口后，工具会执行“固件包 + 设备兼容性检测”。检测全程只读、非阻塞，只提供风险提示和解决步骤；即使结果为警告、无法确认或检测失败，也不会因此禁用“开始刷机”。用户应先核对风险，再自行决定是否刷机。

检测内容包括：

- 固件包完整性：Bootloader、分区表、OTA 数据和应用镜像是否齐全。
- 固件写入偏移与范围：偏移是否有效、文件写入范围是否重叠或越界。
- 分区表：能否读取、分区是否重叠，以及所需 Flash 总容量。
- APP 容量：主应用镜像能否放入对应 APP 分区。
- 芯片型号：固件要求与设备芯片是否匹配。
- Flash：设备容量是否满足固件和分区布局要求。
- PSRAM：容量是否满足要求；固件需要 octal PSRAM 时，同时检查设备是否报告 octal 模式。
- 安全状态：Secure Boot（安全启动）和 Flash Encryption（Flash 加密）是否启用。
- 连接状态：串口是否可访问、设备是否处于 BOOT/下载模式，以及探测是否超时。

设备探测只会运行 esptool 的 `flash_id` 和 `get_security_info`。兼容性检测绝不会自动运行 `erase_flash` 或 `write_flash`；只有用户主动点击“开始刷机”后，工具才会进入实际刷写流程。esptool 的原始输出会保留在检测结果的“技术详情/原始检测日志”和应用的“高级日志”中，便于排查。

### SCM V3/V4 不能通过 USB 自动区分

通用 ESP32-S3 下载模式 USB ID `VID_303A&PID_1001` 只能说明检测到了 ESP32-S3，无法区分 SCM V3 和 SCM V4。V3 使用 SPI 显示接线，V4 使用 8 位并口显示接线，两者不能靠通用 USB 探测确认。

刷错 V3/V4 固件时，芯片可能可以正常启动，但屏幕仍然黑屏。反复刷机不能修复显示接线差异。刷机前请查看显示板或主板上的 PCB 丝印，确认实际是 V3 还是 V4，并选择明确标注对应硬件版本的固件包。

### 怎么恢复到可刷机

1. 重新选择完整固件包，确认其中包含 Bootloader、分区表、OTA 数据和应用镜像；不要使用缺文件、偏移异常或 APP 镜像超过分区容量的包。
2. 换用确认支持数据传输的 USB 数据线，并尽量直接连接电脑 USB 口。
3. 关闭串口监视器、串口调试工具、其他刷机工具以及任何可能占用目标 COM 口的程序。
4. 断开设备 USB。
5. 按住 BOOT/下载键并重新插入 USB；看到 Windows 新增目标 COM 口后松开 BOOT 键。
6. 回到工具点击“重新扫描设备”，选择新出现的下载模式 COM 口，再点击“重新检测”。
7. 如果提示 Flash 或 PSRAM 容量不足、PSRAM 不是所需的 octal 模式，请改用要求更低且与硬件匹配的固件包，或更换满足要求的设备；反复检测或刷机不会增加硬件容量。
8. 如果提示 Secure Boot 或 Flash Encryption 已启用，请停止尝试普通未签名或明文固件，联系设备或固件提供方获取匹配的已签名/加密固件及恢复方案；不要自行修改安全熔丝。处理后重新进入下载模式并再次检测。

## 进入下载模式提示

不同批次键盘进入下载模式的方式可能不同。常见方式：

- 按住 BOOT/下载键后插入 USB。
- 按住 BOOT 后点击工具里的“重新扫描设备”。
- 如果一直连接失败，换一根支持数据传输的 USB 线。

## 实时进度

工具不会弹出 PowerShell 或命令行窗口。所有等待过程都在应用内显示：

- GitHub 下载进度。
- zip 解析状态。
- 便携 Git 自动下载、解压和缓存进度。
- ESP-IDF 自动下载、安装和缓存进度，包含当前下载源和失败切换提示。
- ESP-IDF 构建日志。
- 设备扫描结果。
- esptool 刷机日志。
- 日志从任务开始保留到最后，不再只显示末尾部分。
- 每次启动会生成本地日志文件，界面会显示日志文件路径，并提供“复制日志”按钮。

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

## 常见问题

### 提示构建环境下载失败怎么办？

先让用户直接在工具里重试一次。工具会自动切换华为云、npmmirror、乐鑫镜像和 GitHub 等多个来源，不需要用户复制命令。

如果多次失败，通常是网络无法访问镜像源或公司/校园网络拦截下载。建议改用离线完整版，或者换一个网络后重新点击“准备环境并构建”。

### 提示缺少 Git 怎么办？

新版会自动下载便携 Git，不要求用户安装系统 Git。如果高级日志中仍出现 `Git was not found` 或 `git not found`，请确认正在使用新版 exe，并重新点击“准备环境并构建”。离线完整版需要包含 `runtime/tools/git/cmd/git.exe`。

### 提示组件缓存路径过长或 FileNotFoundError 怎么办？

Windows 下 ESP-IDF 组件有些测试文件路径非常深，默认组件缓存目录可能触发路径过长。新版会把 `IDF_COMPONENT_CACHE_PATH` 优先指向 `C:\SLCM`，如果该目录不可写，会自动尝试 `ProgramData`、系统临时目录和工具本地缓存目录。高级日志里会出现“ESP-IDF 组件缓存目录：...”用于确认实际路径。

如果遇到 `espressif/esp-serial-flasher` 这类组件包内自带的 `test/target-example-src/**/build-*` 超长路径，工具会通过内置 Python 补丁给 `open`、`io.open`、`os.open`、`os.makedirs`、`os.mkdir`、`os.stat`、`os.scandir`、`os.listdir`、`shutil.copytree`、`shutil.copy2` 等文件操作加上 Windows 长路径前缀，覆盖组件下载、缓存校验、复制到 `managed_components` 和构建产物解析流程。旧版本如果留下过缺文件的损坏缓存，新版会按 `CHECKSUMS.json` 检测并删除该组件缓存，让 ESP-IDF 重新完整下载和解压。

构建进程会强制设置 `PYTHONUTF8=1` 和 `PYTHONIOENCODING=utf-8`，避免 ESP-IDF 在中文 Windows 环境下用 GBK 读取 CMake 日志时出现 `UnicodeDecodeError`。

新版还会把源码包解压到 `C:\P\<短ID>`，并在解压写入时直接剥离 zip 里常见的公共根目录，例如 `SKYLOONG-main`。工具不会再先完整解压再移动目录，因此遇到 `tools/web`、`web_new` 这类深层目录时也不会把源码根目录搬到一半；同时 Go 侧的 GitHub 下载、固件 zip 解压、运行时工具解压、缓存创建、日志写入、目录扫描和构建产物检查都会使用 Windows 长路径前缀。特殊环境下可设置 `SKYLOONG_PACKAGE_WORKSPACE_PATH` 指向一个更短且可写的目录。

如果旧版本已经失败过，直接用新版重新点击“准备环境并构建”即可。特殊环境下也可以在启动前设置 `SKYLOONG_COMPONENT_CACHE_PATH` 指向一个更短且可写的目录，例如 `D:\SLCM`。

如果仍然失败，复制完整高级日志，优先查看最早出现的 `CMake Error`、`FileNotFoundError` 或 `cmake failed with exit code`。

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

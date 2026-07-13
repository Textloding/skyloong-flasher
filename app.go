package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/Textloding/skyloong-flasher/internal/builder"
	"github.com/Textloding/skyloong-flasher/internal/device"
	"github.com/Textloding/skyloong-flasher/internal/flasher"
	"github.com/Textloding/skyloong-flasher/internal/githubsource"
	"github.com/Textloding/skyloong-flasher/internal/packagekit"
	"github.com/Textloding/skyloong-flasher/internal/preflight"
	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const packageWorkspaceOverrideEnv = "SKYLOONG_PACKAGE_WORKSPACE_PATH"

const compatibilityDetectionTimeout = 50 * time.Second

type probeDeviceFunc func(context.Context, runtimekit.Status, string, int, func(string)) preflight.DeviceProbe

type App struct {
	ctx             context.Context
	mu              sync.Mutex
	current         *packagekit.Analysis
	probeDevice     probeDeviceFunc
	preflightMu     sync.Mutex
	preflightCancel context.CancelFunc
	preflightToken  uint64
	cacheDir        string
	logMu           sync.Mutex
	logHistory      []string
	logFile         string
}

type AnalyzeResponse struct {
	Analysis *packagekit.Analysis `json:"analysis"`
	Runtime  runtimekit.Status    `json:"runtime"`
	Devices  []device.Device      `json:"devices"`
}

type GitHubRequest struct {
	URL string `json:"url"`
}

type FlashRequest struct {
	Port string `json:"port"`
	Baud int    `json:"baud"`
}

type CompatibilityRequest struct {
	Port string `json:"port"`
	Baud int    `json:"baud"`
}

func NewApp() *App {
	return &App{probeDevice: preflight.ProbeDevice}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.cacheDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "SkyloongFlasher")
	if a.cacheDir == "" || a.cacheDir == "SkyloongFlasher" {
		a.cacheDir = filepath.Join(os.TempDir(), "SkyloongFlasher")
	}
	if err := a.prepareCacheDirs(); err != nil {
		a.cacheDir = filepath.Join(os.TempDir(), "SkyloongFlasher")
		_ = a.prepareCacheDirs()
	}
	_ = a.prepareLogFile()
	a.logLine("SKYLOONG Flasher 已启动")
	a.logLine("缓存目录：" + a.cacheDir)
	if a.logFile != "" {
		a.logLine("日志文件：" + a.logFile)
	}
}

func (a *App) OpenFirmwareZip() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择固件 zip 包",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "ZIP 固件包", Pattern: "*.zip"},
		},
	})
}

func (a *App) AnalyzeLocalZip(path string) (*AnalyzeResponse, error) {
	if err := a.prepareCacheDirs(); err != nil {
		return nil, friendlyError("缓存文件夹准备失败", err)
	}
	a.logLine("开始解析本地固件包：" + path)
	a.progress("解析固件包", 8, "正在读取 zip 文件")
	workspace, err := a.preparePackageWorkspace()
	if err != nil {
		return nil, friendlyError("固件工作目录准备失败", err)
	}
	a.logLine("固件工作目录：" + workspace)
	analysis, err := packagekit.AnalyzeZip(path, workspace)
	if err != nil {
		a.logLine("固件包解析失败：" + err.Error())
		return nil, friendlyError("固件包解析失败", err)
	}
	a.logLine("固件包解析完成：" + analysis.ProjectName)
	a.progress("解析固件包", 100, "固件包解析完成")
	return a.withState(analysis)
}

func (a *App) DownloadAndAnalyzeGithub(req GitHubRequest) (*AnalyzeResponse, error) {
	if err := a.prepareCacheDirs(); err != nil {
		return nil, friendlyError("缓存文件夹准备失败", err)
	}
	spec, err := githubsource.Parse(req.URL)
	if err != nil {
		a.logLine("GitHub 链接解析失败：" + err.Error())
		return nil, friendlyError("GitHub 链接解析失败", err)
	}
	archiveURL := spec.ArchiveURL()
	dest := filepath.Join(a.cacheDir, "downloads", fmt.Sprintf("%s-%s-%d.zip", spec.Repo, safeRef(spec.Ref), time.Now().Unix()))
	a.logLine("开始下载 GitHub 压缩包：" + archiveURL)
	a.logLine("下载保存位置：" + dest)
	a.progress("下载 GitHub 压缩包", 1, "正在连接 GitHub")
	a.emit("download:status", map[string]interface{}{"message": "开始下载 GitHub 压缩包", "url": archiveURL})
	err = githubsource.Download(a.ctx, archiveURL, dest, func(downloaded int64, total int64) {
		percent := 0
		if total > 0 {
			percent = int(float64(downloaded) / float64(total) * 100)
		}
		a.progress("下载 GitHub 压缩包", percent, fmt.Sprintf("已下载 %s", formatBytes(downloaded)))
		a.emit("download:progress", map[string]interface{}{"downloaded": downloaded, "total": total})
	})
	if err != nil {
		a.logLine("GitHub 下载失败：" + err.Error())
		return nil, friendlyError("GitHub 下载失败", err)
	}
	a.logLine("GitHub 压缩包下载完成：" + dest)
	a.progress("下载 GitHub 压缩包", 100, "下载完成，开始解析")
	return a.AnalyzeLocalZip(dest)
}

func (a *App) ScanDevices() ([]device.Device, error) {
	a.progress("扫描设备", 20, "正在读取 Windows 设备列表")
	devices, err := device.Scan()
	if err != nil {
		return devices, friendlyError("设备扫描失败", err)
	}
	a.progress("扫描设备", 100, fmt.Sprintf("扫描完成，发现 %d 个候选设备", len(devices)))
	return devices, nil
}

func (a *App) CheckRuntime() runtimekit.Status {
	return runtimekit.DetectIn(a.cacheDir)
}

func (a *App) DetectCompatibility(req CompatibilityRequest) (*preflight.Report, error) {
	ctx, cancel, token := a.beginCompatibilityDetection()
	defer a.finishCompatibilityDetection(cancel, token)

	a.mu.Lock()
	analysis := a.current
	a.mu.Unlock()
	if analysis == nil {
		return nil, fmt.Errorf("无法检测兼容性：请先选择并解析固件包")
	}
	req.Port = strings.TrimSpace(req.Port)
	if req.Port == "" {
		return nil, fmt.Errorf("无法检测兼容性：请选择设备串口")
	}

	a.logLine(fmt.Sprintf("开始兼容性检测：端口=%s，波特率=%d", req.Port, req.Baud))
	a.progress("兼容性检测", 5, "正在检查设备探测运行时")
	status := runtimekit.DetectIn(a.cacheDir)
	if !status.Available {
		var err error
		status, err = a.ensureRuntimeWithContext(ctx)
		if err != nil {
			a.logLine("兼容性检测运行时准备失败：" + err.Error())
			return nil, friendlyError("兼容性检测运行时准备失败", err)
		}
	}

	a.progress("兼容性检测", 20, "正在检查固件要求")
	firmware := preflight.InspectFirmware(analysis)
	a.progress("兼容性检测", 40, "正在读取设备信息")
	probeDevice := a.probeDevice
	if probeDevice == nil {
		probeDevice = preflight.ProbeDevice
	}
	deviceProbe := probeDevice(ctx, status, req.Port, req.Baud, a.logLine)
	a.progress("兼容性检测", 90, "正在比较固件与设备")
	report := preflight.Compare(firmware, deviceProbe)
	a.logLine("兼容性检测结果：overall=" + report.Overall)
	a.progress("兼容性检测", 100, "兼容性检测完成")
	a.logLine("兼容性检测结束")
	return &report, nil
}

func (a *App) CancelCompatibilityDetection() {
	a.preflightMu.Lock()
	cancel := a.preflightCancel
	if cancel != nil {
		a.preflightCancel = nil
		a.preflightToken++
	}
	a.preflightMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (a *App) beginCompatibilityDetection() (context.Context, context.CancelFunc, uint64) {
	baseContext := a.ctx
	if baseContext == nil {
		baseContext = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseContext, compatibilityDetectionTimeout)

	a.preflightMu.Lock()
	if a.preflightCancel != nil {
		a.preflightCancel()
	}
	a.preflightToken++
	token := a.preflightToken
	a.preflightCancel = cancel
	a.preflightMu.Unlock()

	return ctx, cancel, token
}

func (a *App) finishCompatibilityDetection(cancel context.CancelFunc, token uint64) {
	cancel()
	a.preflightMu.Lock()
	if a.preflightToken == token {
		a.preflightCancel = nil
	}
	a.preflightMu.Unlock()
}

func (a *App) BuildSourcePackage() (*AnalyzeResponse, error) {
	if err := a.prepareCacheDirs(); err != nil {
		return nil, friendlyError("缓存文件夹准备失败", err)
	}
	a.mu.Lock()
	analysis := a.current
	a.mu.Unlock()
	if analysis == nil || !analysis.NeedsBuild {
		return nil, friendlyError("无法构建", fmt.Errorf("当前包不是源码包"))
	}
	status, err := a.ensureRuntime()
	if err != nil {
		a.logLine("构建环境准备失败：" + err.Error())
		return nil, friendlyError("构建环境准备失败", err)
	}
	a.logLine("开始构建源码包：" + analysis.Root)
	a.progress("构建固件", 5, "正在启动 ESP-IDF 构建")
	err = builder.Run(a.ctx, status, analysis.Root, func(line string) {
		a.progress("构建固件", estimateBuildPercent(line), line)
		a.logLine(line)
	})
	if err != nil {
		a.logLine("源码构建失败：" + err.Error())
		return nil, friendlyError("源码构建失败", err)
	}
	a.progress("构建固件", 92, "构建完成，正在重新解析刷机产物")
	buildDir := filepath.Join(analysis.Root, "build")
	if info, err := os.Stat(windowsFilesystemPath(buildDir)); err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("路径不是文件夹：%s", buildDir)
		}
		a.logLine("构建后没有找到 build 文件夹：" + err.Error())
		return nil, friendlyError("构建产物解析失败", fmt.Errorf("构建命令结束了，但没有生成 build 文件夹，请在高级日志中查看前面的构建错误：%w", err))
	}
	built, err := packagekit.AnalyzeDir(buildDir)
	if err != nil {
		a.logLine("构建产物解析失败：" + err.Error())
		return nil, friendlyError("构建产物解析失败", err)
	}
	mergeBuildAnalysisMetadata(built, analysis)
	a.logLine("源码构建完成，刷机产物已准备好")
	a.progress("构建固件", 100, "构建产物已准备好，可以刷机")
	return a.withState(built)
}

func (a *App) StartFlash(req FlashRequest) error {
	a.CancelCompatibilityDetection()

	a.mu.Lock()
	analysis := a.current
	a.mu.Unlock()
	if analysis == nil {
		return friendlyError("无法刷机", fmt.Errorf("请先选择并解析固件包"))
	}
	status := runtimekit.DetectIn(a.cacheDir)
	if !status.Available || (status.Kind == runtimekit.KindEIM && status.GitPath == "") {
		var err error
		status, err = a.ensureRuntime()
		if err != nil {
			a.logLine("刷机运行时准备失败：" + err.Error())
			return friendlyError("刷机运行时准备失败", err)
		}
	}
	a.logLine(fmt.Sprintf("开始刷机：端口=%s，波特率=%d", req.Port, req.Baud))
	a.progress("刷机", 5, "正在启动刷机进程")
	return flasher.Run(a.ctx, status, analysis, req.Port, req.Baud, func(line string) {
		a.progress("刷机", estimateFlashPercent(line), line)
		a.logLine(line)
	})
}

func (a *App) PreviewFlashCommand(req FlashRequest) (string, error) {
	a.mu.Lock()
	analysis := a.current
	a.mu.Unlock()
	if analysis == nil {
		return "", fmt.Errorf("请先选择并解析固件包")
	}
	cmd, err := flasher.BuildCommand(runtimekit.DetectIn(a.cacheDir), analysis, req.Port, req.Baud)
	if err != nil {
		return "", friendlyError("无法生成刷机命令", err)
	}
	return fmt.Sprintf("%q", cmd.Args), nil
}

func (a *App) withState(analysis *packagekit.Analysis) (*AnalyzeResponse, error) {
	a.mu.Lock()
	a.current = analysis
	a.mu.Unlock()
	devices, _ := device.Scan()
	return &AnalyzeResponse{
		Analysis: analysis,
		Runtime:  runtimekit.DetectIn(a.cacheDir),
		Devices:  devices,
	}, nil
}

func (a *App) ensureRuntime() (runtimekit.Status, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.ensureRuntimeWithContext(ctx)
}

func (a *App) ensureRuntimeWithContext(ctx context.Context) (runtimekit.Status, error) {
	if err := a.prepareCacheDirs(); err != nil {
		return runtimekit.Status{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.logLine("开始准备构建/刷机运行时")
	a.logLine("工具会自动准备：便携 Git、EIM CLI、ESP-IDF、Python、CMake、Ninja、交叉编译器、esptool 和组件依赖")
	return runtimekit.Ensure(ctx, a.cacheDir, func(stage string, percent int, message string) {
		a.progress(stage, percent, message)
	}, func(line string) {
		a.logLine(line)
	})
}

func mergeBuildAnalysisMetadata(built, source *packagekit.Analysis) {
	if built == nil || source == nil {
		return
	}
	if strings.TrimSpace(built.HardwareVersion) == "" {
		built.HardwareVersion = source.HardwareVersion
	}
	if strings.TrimSpace(built.Display) == "" {
		built.Display = source.Display
	}
	if built.MinimumFlashBytes == 0 {
		built.MinimumFlashBytes = source.MinimumFlashBytes
	}
	if built.MinimumPSRAMBytes == 0 {
		built.MinimumPSRAMBytes = source.MinimumPSRAMBytes
	}
	if strings.TrimSpace(built.PSRAMMode) == "" {
		built.PSRAMMode = source.PSRAMMode
	}
}

func (a *App) GetLogHistory() []string {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	return append([]string{}, a.logHistory...)
}

func (a *App) GetLogFilePath() string {
	return a.logFile
}

func (a *App) prepareCacheDirs() error {
	if a.cacheDir == "" {
		a.cacheDir = filepath.Join(os.TempDir(), "SkyloongFlasher")
	}
	for _, dir := range []string{
		a.cacheDir,
		filepath.Join(a.cacheDir, "downloads"),
		filepath.Join(a.cacheDir, "packages"),
		filepath.Join(a.cacheDir, "runtime"),
		filepath.Join(a.cacheDir, "tools"),
		filepath.Join(a.cacheDir, "logs"),
	} {
		if err := os.MkdirAll(windowsFilesystemPath(dir), 0o755); err != nil {
			return fmt.Errorf("无法创建文件夹 %s：%w", dir, err)
		}
	}
	if _, err := runtimekit.PrepareComponentCacheDir(a.cacheDir); err != nil {
		return fmt.Errorf("无法创建 ESP-IDF 组件缓存目录：%w", err)
	}
	return nil
}

func (a *App) preparePackageWorkspace() (string, error) {
	var lastErr error
	for _, dir := range packageWorkspaceCandidates(a.cacheDir) {
		if err := os.MkdirAll(windowsFilesystemPath(dir), 0o755); err != nil {
			lastErr = fmt.Errorf("无法创建固件工作目录 %s：%w", dir, err)
			continue
		}
		return dir, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("没有可用的固件工作目录")
}

func packageWorkspaceCandidates(cacheDir string) []string {
	candidates := []string{}
	if override := strings.TrimSpace(os.Getenv(packageWorkspaceOverrideEnv)); override != "" {
		candidates = appendUniquePath(candidates, override)
	}
	if shortRoot := shortPackageWorkspacePath(cacheDir); shortRoot != "" {
		candidates = appendUniquePath(candidates, shortRoot)
	}
	if cacheDir != "" {
		candidates = appendUniquePath(candidates, filepath.Join(cacheDir, "packages"))
	}
	return candidates
}

func shortPackageWorkspacePath(cacheDir string) string {
	if cacheDir == "" {
		return ""
	}
	volume := filepath.VolumeName(cacheDir)
	if volume == "" {
		if abs, err := filepath.Abs(cacheDir); err == nil {
			volume = filepath.VolumeName(abs)
		}
	}
	if volume == "" {
		return ""
	}
	return filepath.Join(volume+string(os.PathSeparator), "P")
}

func appendUniquePath(paths []string, path string) []string {
	if strings.TrimSpace(path) == "" {
		return paths
	}
	clean := filepath.Clean(path)
	for _, existing := range paths {
		if strings.EqualFold(existing, clean) {
			return paths
		}
	}
	return append(paths, clean)
}

func (a *App) prepareLogFile() error {
	if err := os.MkdirAll(windowsFilesystemPath(filepath.Join(a.cacheDir, "logs")), 0o755); err != nil {
		return fmt.Errorf("无法创建日志文件夹：%w", err)
	}
	a.logFile = filepath.Join(a.cacheDir, "logs", time.Now().Format("20060102-150405")+".log")
	file, err := os.OpenFile(windowsFilesystemPath(a.logFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("无法创建日志文件：%w", err)
	}
	return file.Close()
}

func (a *App) logLine(line string) {
	if line == "" {
		return
	}
	a.logMu.Lock()
	a.logHistory = append(a.logHistory, line)
	logFile := a.logFile
	a.logMu.Unlock()

	if logFile != "" {
		if file, err := os.OpenFile(windowsFilesystemPath(logFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			_, _ = file.WriteString(time.Now().Format("15:04:05 ") + line + "\n")
			_ = file.Close()
		}
	}
	a.emit("flash:log", map[string]interface{}{"line": line})
}

func (a *App) emit(name string, payload interface{}) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, name, payload)
	}
}

func (a *App) progress(stage string, percent int, message string) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	a.emit("task:progress", map[string]interface{}{
		"stage":   stage,
		"percent": percent,
		"message": message,
	})
}

func friendlyError(prefix string, err error) error {
	return fmt.Errorf("%s：%w", prefix, err)
}

func safeRef(ref string) string {
	if ref == "" {
		return "main"
	}
	out := make([]rune, 0, len(ref))
	for _, r := range ref {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			out = append(out, r)
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func estimateFlashPercent(line string) int {
	switch {
	case line == "":
		return 5
	case containsAny(line, "Connecting", "Chip is", "Stub running"):
		return 12
	case containsAny(line, "Erasing"):
		return 24
	case containsAny(line, "Writing at"):
		return 45
	case containsAny(line, "Hash of data verified", "Leaving"):
		return 88
	case containsAny(line, "Hard resetting", "Done"):
		return 100
	default:
		return 30
	}
}

func estimateBuildPercent(line string) int {
	switch {
	case line == "":
		return 5
	case containsAny(line, "Executing action", "Running ninja"):
		return 18
	case containsAny(line, "Building C", "Building CXX", "Generating"):
		return 48
	case containsAny(line, "Linking", "Creating esp32s3 image"):
		return 76
	case containsAny(line, "Project build complete", "Successfully created"):
		return 90
	default:
		return 35
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && len(text) >= len(needle) {
			for i := 0; i <= len(text)-len(needle); i++ {
				if text[i:i+len(needle)] == needle {
					return true
				}
			}
		}
	}
	return false
}

func windowsFilesystemPath(path string) string {
	if goruntime.GOOS != "windows" || path == "" {
		return path
	}
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, `\\?\`) {
		return clean
	}
	if strings.HasPrefix(clean, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(clean, `\\`)
	}
	if filepath.VolumeName(clean) != "" {
		return `\\?\` + clean
	}
	return path
}

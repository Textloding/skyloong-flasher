package runtimekit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Textloding/skyloong-flasher/internal/processutil"
)

const (
	KindMissing      = "missing"
	KindExecutable   = "executable"
	KindPythonScript = "python-script"
	KindEIM          = "eim"

	DefaultIDFVersion = "v5.1.4"
	EIMDownloadURL    = "https://github.com/espressif/idf-im-ui/releases/download/v0.17.0/eim-cli-windows-x64.exe"
)

type Status struct {
	Available    bool   `json:"available"`
	CanBuild     bool   `json:"canBuild"`
	Kind         string `json:"kind"`
	ToolPath     string `json:"toolPath"`
	PythonPath   string `json:"pythonPath"`
	IDFPyPath    string `json:"idfPyPath"`
	ExportScript string `json:"exportScript"`
	EIMPath      string `json:"eimPath"`
	EIMJsonPath  string `json:"eimJsonPath"`
	IDFVersion   string `json:"idfVersion"`
	Message      string `json:"message"`
}

type ProgressFunc func(stage string, percent int, message string)
type LogFunc func(line string)

func Detect() Status {
	status := Status{Kind: KindMissing, Message: "未检测到刷机运行时，后续可由工具自动下载或使用已安装 ESP-IDF"}
	if path, err := exec.LookPath("esptool.exe"); err == nil {
		status.Available = true
		status.Kind = KindExecutable
		status.ToolPath = path
		status.Message = "已检测到 esptool.exe"
	}
	if path, err := exec.LookPath("esptool.py"); err == nil {
		if python, ok := findPython(); ok {
			status.Available = true
			status.Kind = KindPythonScript
			status.ToolPath = path
			status.PythonPath = python
			status.Message = "已检测到 esptool.py"
		}
	}
	idf := detectIDF()
	if idf.CanBuild {
		status.CanBuild = true
		status.IDFPyPath = idf.IDFPyPath
		status.ExportScript = idf.ExportScript
		if !status.Available && idf.Available {
			status.Available = true
			status.Kind = idf.Kind
			status.ToolPath = idf.ToolPath
			status.PythonPath = idf.PythonPath
			status.Message = idf.Message
		}
	}
	return status
}

func DetectIn(cacheDir string) Status {
	status := Detect()
	if status.CanBuild && status.Available {
		return status
	}
	if cached, ok := detectCachedEIM(cacheDir); ok {
		return cached
	}
	return status
}

func Ensure(ctx context.Context, cacheDir string, progress ProgressFunc, log LogFunc) (Status, error) {
	status := DetectIn(cacheDir)
	if status.CanBuild && status.Available {
		return status, nil
	}
	if cacheDir == "" {
		return Status{}, errors.New("运行时缓存目录为空")
	}

	if progress != nil {
		progress("准备构建环境", 5, "正在准备 ESP-IDF 安装管理器")
	}
	eimPath, err := ensureEIM(ctx, cacheDir, progress, log)
	if err != nil {
		return Status{}, err
	}

	configDir := filepath.Join(cacheDir, "runtime", "eim-config")
	installDir := filepath.Join(cacheDir, "runtime", "esp-idf")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return Status{}, err
	}

	if progress != nil {
		progress("安装 ESP-IDF", 35, "正在下载并安装 ESP-IDF v5.1.4，首次执行会比较久")
	}
	cmd := EIMInstallCommand(eimPath, configDir, installDir, DefaultIDFVersion)
	installPercent := 35
	if err := runLogged(ctx, cmd, func(line string) {
		if log != nil {
			log(line)
		}
		if progress != nil && line != "" {
			if installPercent < 88 {
				installPercent += 1
			}
			progress("安装 ESP-IDF", installPercent, line)
		}
	}); err != nil {
		return Status{}, fmt.Errorf("ESP-IDF 自动安装失败：%w", err)
	}
	if progress != nil {
		progress("安装 ESP-IDF", 100, "ESP-IDF 构建环境已准备好")
	}
	return eimStatus(eimPath, configDir), nil
}

func EIMInstallCommand(eimPath string, configDir string, installDir string, version string) *exec.Cmd {
	return processutil.Command(
		eimPath,
		"install",
		"--path", installDir,
		"--idf-versions", version,
		"--target", "esp32s3",
		"--non-interactive", "true",
		"--install-all-prerequisites", "true",
		"--do-not-track", "true",
		"--cleanup", "false",
		"--locale", "cn",
		"--esp-idf-json-path", configDir,
	)
}

func EIMRunCommand(status Status, args ...string) *exec.Cmd {
	cmdArgs := []string{"run"}
	if status.EIMJsonPath != "" {
		cmdArgs = append(cmdArgs, "--esp-idf-json-path", status.EIMJsonPath)
	}
	cmdArgs = append(cmdArgs, commandLine(args...))
	if status.IDFVersion != "" {
		cmdArgs = append(cmdArgs, status.IDFVersion)
	}
	return processutil.Command(status.EIMPath, cmdArgs...)
}

func detectCachedEIM(cacheDir string) (Status, bool) {
	if cacheDir == "" {
		return Status{}, false
	}
	for _, root := range runtimeRoots(cacheDir) {
		for _, layout := range []struct {
			eimPath   string
			configDir string
		}{
			{
				eimPath:   filepath.Join(root, "tools", "eim.exe"),
				configDir: filepath.Join(root, "runtime", "eim-config"),
			},
			{
				eimPath:   filepath.Join(root, "runtime", "tools", "eim.exe"),
				configDir: filepath.Join(root, "runtime", "eim-config"),
			},
		} {
			if _, err := os.Stat(layout.eimPath); err != nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(layout.configDir, "eim_idf.json")); err != nil {
				continue
			}
			return eimStatus(layout.eimPath, layout.configDir), true
		}
	}
	return Status{}, false
}

func runtimeRoots(cacheDir string) []string {
	roots := []string{cacheDir}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		roots = append(roots, exeDir, filepath.Join(exeDir, "runtime"))
	}
	return roots
}

func eimStatus(eimPath string, configDir string) Status {
	return Status{
		Available:   true,
		CanBuild:    true,
		Kind:        KindEIM,
		EIMPath:     eimPath,
		EIMJsonPath: configDir,
		IDFVersion:  DefaultIDFVersion,
		Message:     "已准备 ESP-IDF v5.1.4 构建/刷机运行时",
	}
}

func ensureEIM(ctx context.Context, cacheDir string, progress ProgressFunc, log LogFunc) (string, error) {
	if path, err := exec.LookPath("eim.exe"); err == nil {
		return path, nil
	}
	toolDir := filepath.Join(cacheDir, "tools")
	eimPath := filepath.Join(toolDir, "eim.exe")
	if _, err := os.Stat(eimPath); err == nil {
		return eimPath, nil
	}
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		return "", err
	}
	if log != nil {
		log("正在下载 Espressif EIM CLI，用于自动准备 ESP-IDF。")
	}
	tmp := eimPath + ".download"
	if err := downloadFile(ctx, EIMDownloadURL, tmp, func(downloaded int64, total int64) {
		if progress == nil {
			return
		}
		if total > 0 {
			percent := 5 + int(float64(downloaded)/float64(total)*25)
			progress("下载构建环境", percent, fmt.Sprintf("正在下载 EIM CLI：%s / %s", formatBytes(downloaded), formatBytes(total)))
			return
		}
		progress("下载构建环境", 12, fmt.Sprintf("正在下载 EIM CLI：%s", formatBytes(downloaded)))
	}); err != nil {
		return "", fmt.Errorf("EIM CLI 下载失败：%w", err)
	}
	if err := os.Rename(tmp, eimPath); err != nil {
		return "", err
	}
	return eimPath, nil
}

func downloadFile(ctx context.Context, url string, dest string, progress func(downloaded int64, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	var downloaded int64
	buf := make([]byte, 256*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(written)
			if progress != nil {
				progress(downloaded, resp.ContentLength)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func runLogged(ctx context.Context, cmd *exec.Cmd, log LogFunc) error {
	command := processutil.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if log != nil {
		log(strings.Join(cmd.Args, " "))
	}
	if err := command.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	pipe := func(scanner *bufio.Scanner) {
		defer wg.Done()
		for scanner.Scan() {
			if log != nil {
				log(scanner.Text())
			}
		}
	}
	wg.Add(2)
	go pipe(bufio.NewScanner(stdout))
	go pipe(bufio.NewScanner(stderr))
	wg.Wait()
	return command.Wait()
}

func commandLine(args ...string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, quoteArg(arg))
	}
	return strings.Join(quoted, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
}

func detectIDF() Status {
	candidates := []string{}
	if idf := os.Getenv("IDF_PATH"); idf != "" {
		candidates = append(candidates, filepath.Join(idf, "components", "esptool_py", "esptool", "esptool.py"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "esp", "esp-idf", "components", "esptool_py", "esptool", "esptool.py"))
	}
	status := Status{}
	if idfpy, err := exec.LookPath("idf.py"); err == nil {
		status.CanBuild = true
		status.IDFPyPath = idfpy
	}
	for _, export := range exportCandidates() {
		if _, err := os.Stat(export); err == nil {
			status.CanBuild = true
			status.ExportScript = export
			break
		}
	}
	for _, tool := range candidates {
		if _, err := os.Stat(tool); err == nil {
			if python, ok := findPython(); ok {
				status.Available = true
				status.Kind = KindPythonScript
				status.ToolPath = tool
				status.PythonPath = python
				status.Message = "已检测到本机 ESP-IDF esptool"
				return status
			}
		}
	}
	return status
}

func findPython() (string, bool) {
	for _, name := range []string{"python.exe", "python3.exe", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}
	return "", false
}

func exportCandidates() []string {
	candidates := []string{}
	if idf := os.Getenv("IDF_PATH"); idf != "" {
		candidates = append(candidates, filepath.Join(idf, "export.ps1"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "esp", "esp-idf", "export.ps1"))
	}
	return candidates
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

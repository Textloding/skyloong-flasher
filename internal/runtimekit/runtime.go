package runtimekit

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
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
	eimAssetPath      = "espressif/idf-im-ui/releases/download/v0.17.0/eim-cli-windows-x64.exe"
	gitVersion        = "2.53.0"
	gitWindowsVersion = "v2.53.0.windows.1"
	gitAssetName      = "MinGit-2.53.0-64-bit.zip"

	componentCacheOverrideEnv = "SKYLOONG_COMPONENT_CACHE_PATH"
	componentCacheDirName     = "SLCM"
	pythonPatchDirName        = "_python_patch"
	siteCustomizeName         = "sitecustomize.py"
)

const siteCustomizeZipPatch = `import os
import builtins
import io
import shutil

_ORIGINAL_OPEN = builtins.open
_ORIGINAL_IO_OPEN = io.open
_ORIGINAL_OS_OPEN = os.open
_ORIGINAL_MAKEDIRS = os.makedirs
_ORIGINAL_MKDIR = os.mkdir
_ORIGINAL_STAT = os.stat
_ORIGINAL_LSTAT = getattr(os, "lstat", None)
_ORIGINAL_SCANDIR = os.scandir
_ORIGINAL_LISTDIR = os.listdir
_ORIGINAL_RENAME = os.rename
_ORIGINAL_REPLACE = os.replace
_ORIGINAL_REMOVE = os.remove
_ORIGINAL_UNLINK = os.unlink
_ORIGINAL_RMDIR = os.rmdir
_ORIGINAL_COPYTREE = shutil.copytree
_ORIGINAL_COPY2 = shutil.copy2
_ORIGINAL_COPY = shutil.copy
_ORIGINAL_COPYFILE = shutil.copyfile
_ORIGINAL_COPYSTAT = shutil.copystat
_ORIGINAL_COPYMODE = shutil.copymode
_ORIGINAL_RMTREE = shutil.rmtree

def _skyloong_long_path(path):
    if os.name != "nt":
        return path
    try:
        raw = os.fspath(path)
    except TypeError:
        return path
    if not isinstance(raw, str):
        return path
    if raw.startswith("\\\\?\\"):
        return raw
    absolute = os.path.normpath(os.path.abspath(raw))
    if len(absolute) < 240:
        return path
    if absolute.startswith("\\\\"):
        return "\\\\?\\UNC\\" + absolute[2:]
    if len(absolute) > 2 and absolute[1] == ":":
        return "\\\\?\\" + absolute
    return path

def _skyloong_unbind_path(path, args):
    if isinstance(path, int):
        return path, args
    try:
        os.fspath(path)
        return path, args
    except TypeError:
        if args:
            return args[0], args[1:]
        return path, args

def _skyloong_one_path(func):
    def wrapper(path, *args, **kwargs):
        path, args = _skyloong_unbind_path(path, args)
        return func(_skyloong_long_path(path), *args, **kwargs)
    return wrapper

def _skyloong_two_paths(func):
    def wrapper(src, dst, *args, **kwargs):
        src, remaining = _skyloong_unbind_path(src, (dst,) + args)
        if not remaining:
            return func(_skyloong_long_path(src), *args, **kwargs)
        dst = remaining[0]
        args = remaining[1:]
        return func(_skyloong_long_path(src), _skyloong_long_path(dst), *args, **kwargs)
    return wrapper

def _skyloong_open(file, *args, **kwargs):
    file, args = _skyloong_unbind_path(file, args)
    if args and isinstance(args[0], int):
        return _ORIGINAL_OS_OPEN(_skyloong_long_path(file), *args, **kwargs)
    return _ORIGINAL_OPEN(_skyloong_long_path(file), *args, **kwargs)

def _skyloong_io_open(file, *args, **kwargs):
    file, args = _skyloong_unbind_path(file, args)
    if args and isinstance(args[0], int):
        return _ORIGINAL_OS_OPEN(_skyloong_long_path(file), *args, **kwargs)
    return _ORIGINAL_IO_OPEN(_skyloong_long_path(file), *args, **kwargs)

def _skyloong_os_open(path, *args, **kwargs):
    path, args = _skyloong_unbind_path(path, args)
    return _ORIGINAL_OS_OPEN(_skyloong_long_path(path), *args, **kwargs)

def _skyloong_makedirs(name, *args, **kwargs):
    name, args = _skyloong_unbind_path(name, args)
    return _ORIGINAL_MAKEDIRS(_skyloong_long_path(name), *args, **kwargs)

def _skyloong_mkdir(path, *args, **kwargs):
    path, args = _skyloong_unbind_path(path, args)
    if kwargs.get("dir_fd") is not None:
        return _ORIGINAL_MKDIR(path, *args, **kwargs)
    return _ORIGINAL_MKDIR(_skyloong_long_path(path), *args, **kwargs)

def _skyloong_stat(path, *args, **kwargs):
    path, args = _skyloong_unbind_path(path, args)
    return _ORIGINAL_STAT(_skyloong_long_path(path), *args, **kwargs)

def _skyloong_lstat(path, *args, **kwargs):
    path, args = _skyloong_unbind_path(path, args)
    return _ORIGINAL_LSTAT(_skyloong_long_path(path), *args, **kwargs)

def _skyloong_copytree(src, dst, *args, **kwargs):
    src, remaining = _skyloong_unbind_path(src, (dst,) + args)
    if remaining:
        dst = remaining[0]
        args = remaining[1:]
    if "copy_function" not in kwargs and len(args) < 3:
        kwargs["copy_function"] = shutil.copy2
    return _ORIGINAL_COPYTREE(_skyloong_long_path(src), _skyloong_long_path(dst), *args, **kwargs)

if not getattr(os, "_skyloong_long_path_patch_installed", False):
    builtins.open = _skyloong_open
    io.open = _skyloong_io_open
    os.open = _skyloong_os_open
    os.makedirs = _skyloong_makedirs
    os.mkdir = _skyloong_mkdir
    os.stat = _skyloong_stat
    if _ORIGINAL_LSTAT is not None:
        os.lstat = _skyloong_lstat
    os.scandir = _skyloong_one_path(_ORIGINAL_SCANDIR)
    os.listdir = _skyloong_one_path(_ORIGINAL_LISTDIR)
    os.rename = _skyloong_two_paths(_ORIGINAL_RENAME)
    os.replace = _skyloong_two_paths(_ORIGINAL_REPLACE)
    os.remove = _skyloong_one_path(_ORIGINAL_REMOVE)
    os.unlink = _skyloong_one_path(_ORIGINAL_UNLINK)
    os.rmdir = _skyloong_one_path(_ORIGINAL_RMDIR)
    shutil.copytree = _skyloong_copytree
    shutil.copy2 = _skyloong_two_paths(_ORIGINAL_COPY2)
    shutil.copy = _skyloong_two_paths(_ORIGINAL_COPY)
    shutil.copyfile = _skyloong_two_paths(_ORIGINAL_COPYFILE)
    shutil.copystat = _skyloong_two_paths(_ORIGINAL_COPYSTAT)
    shutil.copymode = _skyloong_two_paths(_ORIGINAL_COPYMODE)
    shutil.rmtree = _skyloong_one_path(_ORIGINAL_RMTREE)
    os._skyloong_long_path_patch_installed = True
`

type Status struct {
	Available          bool   `json:"available"`
	CanBuild           bool   `json:"canBuild"`
	Kind               string `json:"kind"`
	ToolPath           string `json:"toolPath"`
	PythonPath         string `json:"pythonPath"`
	IDFPyPath          string `json:"idfPyPath"`
	ExportScript       string `json:"exportScript"`
	EIMPath            string `json:"eimPath"`
	EIMJsonPath        string `json:"eimJsonPath"`
	IDFVersion         string `json:"idfVersion"`
	GitPath            string `json:"gitPath"`
	ComponentCachePath string `json:"componentCachePath"`
	Message            string `json:"message"`
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
		return withRuntimePaths(status, cacheDir)
	}
	if cached, ok := detectCachedEIM(cacheDir); ok {
		return withRuntimePaths(cached, cacheDir)
	}
	return withRuntimePaths(status, cacheDir)
}

func Ensure(ctx context.Context, cacheDir string, progress ProgressFunc, log LogFunc) (Status, error) {
	if cacheDir == "" {
		return Status{}, errors.New("运行时缓存目录为空")
	}

	status := DetectIn(cacheDir)
	componentCache, err := PrepareComponentCacheDir(cacheDir)
	if err != nil {
		return Status{}, fmt.Errorf("ESP-IDF 组件缓存目录准备失败：%w", err)
	}
	status.ComponentCachePath = componentCache
	if log != nil && componentCache != "" {
		log("ESP-IDF 组件缓存目录：" + componentCache)
	}
	gitPath, err := ensureGit(ctx, cacheDir, progress, log)
	if err != nil {
		return Status{}, err
	}
	status.GitPath = gitPath
	if status.CanBuild && status.Available {
		return status, nil
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
	if err := os.MkdirAll(windowsFilesystemPath(configDir), 0o755); err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(windowsFilesystemPath(installDir), 0o755); err != nil {
		return Status{}, err
	}

	if progress != nil {
		progress("安装 ESP-IDF", 35, "正在下载并安装 ESP-IDF v5.1.4，首次执行会比较久")
	}
	cmd := EIMInstallCommand(eimPath, configDir, installDir, DefaultIDFVersion, gitPath)
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
	status = eimStatus(eimPath, configDir)
	status.GitPath = gitPath
	status.ComponentCachePath = componentCache
	return status, nil
}

func EIMInstallCommand(eimPath string, configDir string, installDir string, version string, gitPaths ...string) *exec.Cmd {
	cmd := processutil.Command(
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
		"--pypi-mirror", "https://pypi.tuna.tsinghua.edu.cn/simple",
		"--esp-idf-json-path", configDir,
	)
	cmd.Env = mirrorEnv(os.Environ(), gitPaths...)
	return cmd
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
	cmd := processutil.Command(status.EIMPath, cmdArgs...)
	cmd.Env = runtimeEnv(os.Environ(), status.ComponentCachePath, status.GitPath)
	return cmd
}

func CommandEnv(status Status) []string {
	return runtimeEnv(os.Environ(), status.ComponentCachePath, status.GitPath)
}

func EIMDownloadSources() []string {
	return []string{
		"https://dl.espressif.cn/github_assets/" + eimAssetPath,
		"https://dl.espressif.com/github_assets/" + eimAssetPath,
		"https://github.com/" + eimAssetPath,
	}
}

func GitDownloadSources() []string {
	assetPath := gitWindowsVersion + "/" + gitAssetName
	return []string{
		"https://mirrors.huaweicloud.com/git-for-windows/" + assetPath,
		"https://registry.npmmirror.com/-/binary/git-for-windows/" + assetPath,
		"https://github.com/git-for-windows/git/releases/download/" + assetPath,
	}
}

func withRuntimePaths(status Status, cacheDir string) Status {
	if path, ok := findGit(cacheDir); ok {
		status.GitPath = path
	}
	status.ComponentCachePath = componentCachePath(cacheDir)
	return status
}

func ComponentCachePath(cacheDir string) string {
	return componentCachePath(cacheDir)
}

func PrepareComponentCacheDir(cacheDir string) (string, error) {
	var lastErr error
	for _, dir := range componentCachePathCandidates(cacheDir) {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(windowsFilesystemPath(dir), 0o755); err != nil {
			lastErr = fmt.Errorf("%s: %w", dir, err)
			continue
		}
		if err := preparePythonZipPatch(dir); err != nil {
			lastErr = fmt.Errorf("%s: %w", dir, err)
			continue
		}
		if err := repairCorruptSerialFlasherCaches(dir); err != nil {
			lastErr = fmt.Errorf("%s: %w", dir, err)
			continue
		}
		return dir, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("没有可用的 ESP-IDF 组件缓存目录")
}

func componentCachePath(cacheDir string) string {
	candidates := componentCachePathCandidates(cacheDir)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func componentCachePathCandidates(cacheDir string) []string {
	paths := []string{}
	if override := strings.TrimSpace(os.Getenv(componentCacheOverrideEnv)); override != "" {
		paths = appendUniquePath(paths, override)
	}
	if shortRoot := shortRootComponentCachePath(cacheDir); shortRoot != "" {
		paths = appendUniquePath(paths, shortRoot)
	}
	if programData := strings.TrimSpace(os.Getenv("PROGRAMDATA")); programData != "" {
		paths = appendUniquePath(paths, filepath.Join(programData, componentCacheDirName))
	}
	if tempDir := strings.TrimSpace(os.TempDir()); tempDir != "" {
		paths = appendUniquePath(paths, filepath.Join(tempDir, componentCacheDirName))
	}
	if cacheDir != "" {
		paths = appendUniquePath(paths, filepath.Join(cacheDir, "cm"))
	}
	return paths
}

func shortRootComponentCachePath(cacheDir string) string {
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
	return filepath.Join(volume+string(os.PathSeparator), componentCacheDirName)
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

func pythonPatchPath(componentCachePath string) string {
	if componentCachePath == "" {
		return ""
	}
	return filepath.Join(componentCachePath, pythonPatchDirName)
}

func preparePythonZipPatch(componentCachePath string) error {
	patchDir := pythonPatchPath(componentCachePath)
	if patchDir == "" {
		return nil
	}
	if err := os.MkdirAll(windowsFilesystemPath(patchDir), 0o755); err != nil {
		return err
	}
	return os.WriteFile(windowsFilesystemPath(filepath.Join(patchDir, siteCustomizeName)), []byte(siteCustomizeZipPatch), 0o644)
}

func repairCorruptSerialFlasherCaches(componentCachePath string) error {
	if componentCachePath == "" {
		return nil
	}
	pattern := filepath.Join(
		componentCachePath,
		"service_*",
		"espressif__esp-serial-flasher_*",
	)
	matches, err := filepath.Glob(windowsFilesystemPath(pattern))
	if err != nil {
		return err
	}
	for _, dir := range matches {
		dir = normalWindowsFilesystemPath(dir)
		corrupt, err := componentCacheCorrupt(dir)
		if err != nil {
			return err
		}
		if !corrupt {
			continue
		}
		if err := os.RemoveAll(windowsFilesystemPath(dir)); err != nil {
			return err
		}
	}
	return nil
}

type componentChecksums struct {
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

func componentCacheCorrupt(componentDir string) (bool, error) {
	raw, err := os.ReadFile(windowsFilesystemPath(filepath.Join(componentDir, "CHECKSUMS.json")))
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var checksums componentChecksums
	if err := json.Unmarshal(raw, &checksums); err != nil {
		return true, nil
	}
	componentAbs, err := filepath.Abs(componentDir)
	if err != nil {
		return false, err
	}
	for _, file := range checksums.Files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		target := filepath.Join(componentDir, filepath.FromSlash(file.Path))
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return false, err
		}
		if targetAbs != componentAbs && !strings.HasPrefix(targetAbs, componentAbs+string(os.PathSeparator)) {
			return true, nil
		}
		if _, err := os.Stat(windowsFilesystemPath(target)); err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, err
		}
	}
	return false, nil
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

func normalWindowsFilesystemPath(path string) string {
	if goruntime.GOOS != "windows" || path == "" {
		return path
	}
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	}
	if strings.HasPrefix(path, `\\?\`) {
		return strings.TrimPrefix(path, `\\?\`)
	}
	return path
}

func findGit(cacheDir string) (string, bool) {
	if path, err := exec.LookPath("git.exe"); err == nil {
		return path, true
	}
	if path, ok := detectCachedGit(cacheDir); ok {
		return path, true
	}
	return "", false
}

func detectCachedGit(cacheDir string) (string, bool) {
	if cacheDir == "" {
		return "", false
	}
	for _, root := range runtimeRoots(cacheDir) {
		for _, candidate := range []string{
			filepath.Join(root, "tools", "git", "cmd", "git.exe"),
			filepath.Join(root, "tools", "git", "mingw64", "bin", "git.exe"),
			filepath.Join(root, "runtime", "tools", "git", "cmd", "git.exe"),
			filepath.Join(root, "runtime", "tools", "git", "mingw64", "bin", "git.exe"),
		} {
			if _, err := os.Stat(windowsFilesystemPath(candidate)); err == nil {
				return candidate, true
			}
		}
	}
	return "", false
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
			if _, err := os.Stat(windowsFilesystemPath(layout.eimPath)); err != nil {
				continue
			}
			if _, err := os.Stat(windowsFilesystemPath(filepath.Join(layout.configDir, "eim_idf.json"))); err != nil {
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

func ensureGit(ctx context.Context, cacheDir string, progress ProgressFunc, log LogFunc) (string, error) {
	if path, ok := findGit(cacheDir); ok {
		if log != nil {
			log("已检测到 Git 运行时：" + path)
		}
		return path, nil
	}

	toolDir := filepath.Join(cacheDir, "tools")
	gitDir := filepath.Join(toolDir, "git")
	gitPath := filepath.Join(gitDir, "cmd", "git.exe")
	if err := os.MkdirAll(windowsFilesystemPath(toolDir), 0o755); err != nil {
		return "", err
	}

	if log != nil {
		log("正在准备便携 Git 运行时，用于 ESP-IDF 自动安装和源码构建。")
	}
	if progress != nil {
		progress("准备构建环境", 3, "正在检查 Git 运行时")
	}

	tmpZip := filepath.Join(toolDir, gitAssetName+".download")
	tmpDir := filepath.Join(toolDir, "git.download")
	var lastErr error
	for index, source := range GitDownloadSources() {
		sourceName := downloadSourceName(source)
		if log != nil {
			log(fmt.Sprintf("正在尝试下载便携 Git %s：%s", gitVersion, sourceName))
		}
		if progress != nil {
			progress("下载构建环境", 4+index*5, fmt.Sprintf("正在连接 %s 下载便携 Git", sourceName))
		}
		err := downloadFile(ctx, source, tmpZip, func(downloaded int64, total int64) {
			if progress == nil {
				return
			}
			if total > 0 {
				percent := 5 + index*5 + int(float64(downloaded)/float64(total)*14)
				progress("下载构建环境", percent, fmt.Sprintf("正在从 %s 下载便携 Git：%s / %s", sourceName, formatBytes(downloaded), formatBytes(total)))
				return
			}
			progress("下载构建环境", 8+index*5, fmt.Sprintf("正在从 %s 下载便携 Git：%s", sourceName, formatBytes(downloaded)))
		})
		if err != nil {
			lastErr = err
			_ = os.Remove(windowsFilesystemPath(tmpZip))
			if log != nil {
				log(fmt.Sprintf("%s 下载便携 Git 失败：%v，准备切换备用源。", sourceName, err))
			}
			if progress != nil {
				progress("下载构建环境", 10+index*5, fmt.Sprintf("%s 下载失败，正在切换备用源", sourceName))
			}
			continue
		}

		_ = os.RemoveAll(windowsFilesystemPath(tmpDir))
		if progress != nil {
			progress("解压构建环境", 25, "正在解压便携 Git")
		}
		if err := unzip(tmpZip, tmpDir, func(done int, total int) {
			if progress == nil || total == 0 {
				return
			}
			progress("解压构建环境", 25+int(float64(done)/float64(total)*8), fmt.Sprintf("正在解压便携 Git：%d / %d", done, total))
		}); err != nil {
			lastErr = err
			_ = os.Remove(windowsFilesystemPath(tmpZip))
			_ = os.RemoveAll(windowsFilesystemPath(tmpDir))
			if log != nil {
				log(fmt.Sprintf("便携 Git 解压失败：%v，准备切换备用源。", err))
			}
			continue
		}
		_ = os.Remove(windowsFilesystemPath(tmpZip))
		_ = os.RemoveAll(windowsFilesystemPath(gitDir))
		if err := os.Rename(windowsFilesystemPath(tmpDir), windowsFilesystemPath(gitDir)); err != nil {
			lastErr = err
			_ = os.RemoveAll(windowsFilesystemPath(tmpDir))
			if log != nil {
				log(fmt.Sprintf("便携 Git 缓存写入失败：%v，准备切换备用源。", err))
			}
			continue
		}
		if _, err := os.Stat(windowsFilesystemPath(gitPath)); err != nil {
			if fallback, ok := detectCachedGit(cacheDir); ok {
				gitPath = fallback
			} else {
				lastErr = fmt.Errorf("便携 Git 解压完成但未找到 git.exe")
				continue
			}
		}
		if progress != nil {
			progress("准备构建环境", 34, "便携 Git 已准备好")
		}
		if log != nil {
			log("便携 Git 已准备好：" + gitPath)
		}
		return gitPath, nil
	}
	return "", fmt.Errorf("便携 Git 下载或解压失败，已尝试华为云镜像、npmmirror 和 GitHub：%w", lastErr)
}

func ensureEIM(ctx context.Context, cacheDir string, progress ProgressFunc, log LogFunc) (string, error) {
	if path, err := exec.LookPath("eim.exe"); err == nil {
		return path, nil
	}
	toolDir := filepath.Join(cacheDir, "tools")
	eimPath := filepath.Join(toolDir, "eim.exe")
	if _, err := os.Stat(windowsFilesystemPath(eimPath)); err == nil {
		return eimPath, nil
	}
	if err := os.MkdirAll(windowsFilesystemPath(toolDir), 0o755); err != nil {
		return "", err
	}
	if log != nil {
		log("正在下载 Espressif EIM CLI，用于自动准备 ESP-IDF。")
	}
	tmp := eimPath + ".download"
	var lastErr error
	for index, source := range EIMDownloadSources() {
		sourceName := downloadSourceName(source)
		if log != nil {
			log(fmt.Sprintf("正在尝试下载 EIM CLI：%s", sourceName))
		}
		if progress != nil {
			progress("下载构建环境", 6+index*4, fmt.Sprintf("正在连接 %s", sourceName))
		}
		if err := downloadFile(ctx, source, tmp, func(downloaded int64, total int64) {
			if progress == nil {
				return
			}
			if total > 0 {
				percent := 8 + index*4 + int(float64(downloaded)/float64(total)*18)
				progress("下载构建环境", percent, fmt.Sprintf("正在从 %s 下载 EIM CLI：%s / %s", sourceName, formatBytes(downloaded), formatBytes(total)))
				return
			}
			progress("下载构建环境", 12+index*4, fmt.Sprintf("正在从 %s 下载 EIM CLI：%s", sourceName, formatBytes(downloaded)))
		}); err == nil {
			if log != nil {
				log(fmt.Sprintf("EIM CLI 下载完成：%s", sourceName))
			}
			if err := os.Rename(windowsFilesystemPath(tmp), windowsFilesystemPath(eimPath)); err != nil {
				return "", err
			}
			return eimPath, nil
		} else {
			lastErr = err
			_ = os.Remove(windowsFilesystemPath(tmp))
			if log != nil {
				log(fmt.Sprintf("%s 下载失败：%v，准备切换备用源。", sourceName, err))
			}
			if progress != nil {
				progress("下载构建环境", 12+index*5, fmt.Sprintf("%s 连接失败，正在切换备用源", sourceName))
			}
		}
	}
	return "", fmt.Errorf("EIM CLI 下载失败，已尝试乐鑫国内镜像、乐鑫国际镜像和 GitHub：%w", lastErr)
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
	out, err := os.Create(windowsFilesystemPath(dest))
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

func unzip(zipPath string, destDir string, progress func(done int, total int)) error {
	reader, err := zip.OpenReader(windowsFilesystemPath(zipPath))
	if err != nil {
		return err
	}
	defer reader.Close()
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	for index, file := range reader.File {
		target := filepath.Join(destDir, file.Name)
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if targetAbs != destAbs && !strings.HasPrefix(targetAbs, destAbs+string(os.PathSeparator)) {
			return fmt.Errorf("zip 内包含非法路径：%s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(windowsFilesystemPath(targetAbs), 0o755); err != nil {
				return err
			}
			if progress != nil {
				progress(index+1, len(reader.File))
			}
			continue
		}
		if err := os.MkdirAll(windowsFilesystemPath(filepath.Dir(targetAbs)), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(windowsFilesystemPath(targetAbs), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if progress != nil {
			progress(index+1, len(reader.File))
		}
	}
	return nil
}

func runLogged(ctx context.Context, cmd *exec.Cmd, log LogFunc) error {
	command := processutil.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	command.Env = cmd.Env
	command.Dir = cmd.Dir
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
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			if log != nil {
				log(scanner.Text())
			}
		}
		if err := scanner.Err(); err != nil && log != nil {
			log("日志读取失败：" + err.Error())
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

func mirrorEnv(base []string, toolPaths ...string) []string {
	return runtimeEnv(base, "", toolPaths...)
}

func runtimeEnv(base []string, componentCachePath string, toolPaths ...string) []string {
	values := map[string]string{
		"IDF_GITHUB_ASSETS":         "dl.espressif.cn/github_assets",
		"IDF_COMPONENT_STORAGE_URL": "https://components-file.espressif.cn;https://components-file.espressif.com",
		"PIP_INDEX_URL":             "https://pypi.tuna.tsinghua.edu.cn/simple",
		"PIP_TRUSTED_HOST":          "pypi.tuna.tsinghua.edu.cn",
		"PYTHONUTF8":                "1",
		"PYTHONIOENCODING":          "utf-8",
	}
	if componentCachePath != "" {
		values["IDF_COMPONENT_CACHE_PATH"] = componentCachePath
	}
	env := upsertEnv(base, values)
	if patchPath := pythonPatchPath(componentCachePath); patchPath != "" {
		env = prependEnvPath(env, "PYTHONPATH", patchPath)
	}
	return prependPath(env, gitEnvDirs(toolPaths...)...)
}

func upsertEnv(base []string, values map[string]string) []string {
	out := append([]string{}, base...)
	seen := map[string]bool{}
	for i, item := range out {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if value, exists := values[key]; exists {
			out[i] = key + "=" + value
			seen[key] = true
		}
	}
	for key, value := range values {
		if !seen[key] {
			out = append(out, key+"="+value)
		}
	}
	return out
}

func prependPath(base []string, dirs ...string) []string {
	return prependEnvPath(base, "PATH", dirs...)
}

func prependEnvPath(base []string, envKey string, dirs ...string) []string {
	cleanDirs := make([]string, 0, len(dirs))
	seenDir := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		clean := filepath.Clean(dir)
		key := strings.ToLower(clean)
		if seenDir[key] {
			continue
		}
		seenDir[key] = true
		cleanDirs = append(cleanDirs, clean)
	}
	if len(cleanDirs) == 0 {
		return base
	}

	out := append([]string{}, base...)
	prefix := strings.Join(cleanDirs, string(os.PathListSeparator))
	for i, item := range out {
		key, value, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(key, envKey) {
			if value != "" {
				out[i] = key + "=" + prefix + string(os.PathListSeparator) + value
			} else {
				out[i] = key + "=" + prefix
			}
			return out
		}
	}
	return append(out, envKey+"="+prefix)
}

func gitEnvDirs(paths ...string) []string {
	dirs := []string{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		gitDir := filepath.Dir(path)
		dirs = append(dirs, gitDir)
		root := portableGitRoot(path)
		if root == "" {
			continue
		}
		for _, candidate := range []string{
			filepath.Join(root, "cmd"),
			filepath.Join(root, "mingw64", "bin"),
			filepath.Join(root, "usr", "bin"),
		} {
			if candidate != gitDir {
				dirs = append(dirs, candidate)
			}
		}
	}
	return dirs
}

func portableGitRoot(gitPath string) string {
	clean := filepath.Clean(gitPath)
	dir := filepath.Dir(clean)
	parent := filepath.Base(dir)
	switch strings.ToLower(parent) {
	case "cmd":
		return filepath.Dir(dir)
	case "bin":
		up := filepath.Dir(dir)
		if strings.EqualFold(filepath.Base(up), "mingw64") || strings.EqualFold(filepath.Base(up), "usr") {
			return filepath.Dir(up)
		}
	}
	return ""
}

func downloadSourceName(url string) string {
	switch {
	case strings.Contains(url, "mirrors.huaweicloud.com"):
		return "华为云镜像"
	case strings.Contains(url, "registry.npmmirror.com"):
		return "npmmirror 镜像"
	case strings.Contains(url, "dl.espressif.cn"):
		return "乐鑫国内镜像"
	case strings.Contains(url, "dl.espressif.com"):
		return "乐鑫国际镜像"
	case strings.Contains(url, "git-for-windows"):
		return "Git for Windows"
	case strings.Contains(url, "github.com"):
		return "GitHub"
	default:
		return url
	}
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
		if _, err := os.Stat(windowsFilesystemPath(export)); err == nil {
			status.CanBuild = true
			status.ExportScript = export
			break
		}
	}
	for _, tool := range candidates {
		if _, err := os.Stat(windowsFilesystemPath(tool)); err == nil {
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

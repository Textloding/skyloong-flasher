package packagekit

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	KindUnknown = "unknown"
	KindFlash   = "flash"
	KindSource  = "source"
)

type Analysis struct {
	SourcePath     string      `json:"sourcePath"`
	Root           string      `json:"root"`
	Kind           string      `json:"kind"`
	ProjectName    string      `json:"projectName"`
	CanFlash       bool        `json:"canFlash"`
	NeedsBuild     bool        `json:"needsBuild"`
	Chip           string      `json:"chip"`
	WriteFlashArgs []string    `json:"writeFlashArgs"`
	Before         string      `json:"before"`
	After          string      `json:"after"`
	FlashFiles     []FlashFile `json:"flashFiles"`
	Messages       []string    `json:"messages"`
}

type FlashFile struct {
	Offset string `json:"offset"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
}

type flasherArgsJSON struct {
	WriteFlashArgs []string          `json:"write_flash_args"`
	FlashFiles     map[string]string `json:"flash_files"`
	ExtraEsptool   struct {
		Chip   string `json:"chip"`
		Before string `json:"before"`
		After  string `json:"after"`
	} `json:"extra_esptool_args"`
}

func AnalyzeZip(zipPath string, workspace string) (*Analysis, error) {
	if zipPath == "" {
		return nil, errors.New("请选择固件 zip 文件")
	}
	if info, err := os.Stat(zipPath); err != nil {
		return nil, fmt.Errorf("找不到固件 zip 文件：%s", zipPath)
	} else if info.IsDir() {
		return nil, fmt.Errorf("选择的是文件夹，不是固件 zip 文件：%s", zipPath)
	}
	if workspace == "" {
		workspace = os.TempDir()
	}
	root, err := createPackageRoot(workspace, shortPackageID)
	if err != nil {
		return nil, fmt.Errorf("无法创建固件解压文件夹：%w", err)
	}
	extracted, err := ExtractZip(zipPath, root)
	if err != nil {
		return nil, err
	}
	analysis, err := AnalyzeDir(extracted)
	if err != nil {
		return nil, fmt.Errorf("%w; %s", err, unrecognizedPackageSummary(zipPath, extracted))
	}
	analysis.SourcePath = zipPath
	return analysis, nil
}

func ExtractZip(zipPath string, dest string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("无法打开 zip：%w", err)
	}
	defer reader.Close()

	cleanDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cleanDest, 0o755); err != nil {
		return "", err
	}

	for _, file := range reader.File {
		target := filepath.Join(cleanDest, filepath.Clean(file.Name))
		if !isInside(cleanDest, target) {
			return "", fmt.Errorf("zip 内包含不安全路径：%s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		src, err := file.Open()
		if err != nil {
			return "", err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			src.Close()
			return "", err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return collapseSingleRoot(cleanDest), nil
}

func AnalyzeDir(root string) (*Analysis, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	analysis := &Analysis{
		Root:        absRoot,
		Kind:        KindUnknown,
		ProjectName: filepath.Base(absRoot),
		Chip:        "esp32s3",
		Before:      "default_reset",
		After:       "hard_reset",
	}

	flasherPath := findFirst(absRoot, "flasher_args.json")
	if flasherPath != "" {
		return analyzeFlasherJSON(analysis, flasherPath)
	}

	flashArgsPath := findFirst(absRoot, "flash_args")
	if flashArgsPath != "" {
		return analyzeFlashArgs(analysis, flashArgsPath)
	}

	if sourceRoot := findIDFSourceRoot(absRoot); sourceRoot != "" {
		analysis.Root = sourceRoot
		analysis.ProjectName = filepath.Base(sourceRoot)
		analysis.Kind = KindSource
		analysis.NeedsBuild = true
		analysis.Messages = append(analysis.Messages, "Detected nested ESP-IDF source package; build is required before flashing.")
		return analysis, nil
	}

	if isIDFSource(absRoot) {
		analysis.Kind = KindSource
		analysis.NeedsBuild = true
		analysis.Messages = append(analysis.Messages, "检测到 ESP-IDF 源码包，需要先构建再刷机。")
		return analysis, nil
	}

	return analysis, errors.New("没有找到 flasher_args.json、flash_args 或 ESP-IDF 源码结构")
}

func analyzeFlasherJSON(analysis *Analysis, path string) (*Analysis, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data flasherArgsJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("flasher_args.json 解析失败：%w", err)
	}

	base := filepath.Dir(path)
	analysis.Root = base
	analysis.Kind = KindFlash
	analysis.CanFlash = true
	analysis.WriteFlashArgs = append([]string{}, data.WriteFlashArgs...)
	if data.ExtraEsptool.Chip != "" {
		analysis.Chip = data.ExtraEsptool.Chip
	}
	if data.ExtraEsptool.Before != "" {
		analysis.Before = data.ExtraEsptool.Before
	}
	if data.ExtraEsptool.After != "" {
		analysis.After = data.ExtraEsptool.After
	}

	files, err := resolveFlashFiles(base, data.FlashFiles)
	if err != nil {
		return nil, err
	}
	analysis.FlashFiles = files
	analysis.Messages = append(analysis.Messages, "检测到可直接刷机的 ESP-IDF 构建产物。")
	return analysis, nil
}

func analyzeFlashArgs(analysis *Analysis, path string) (*Analysis, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	lines := strings.Split(string(raw), "\n")
	files := map[string]string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if strings.HasPrefix(parts[0], "--") {
			analysis.WriteFlashArgs = append(analysis.WriteFlashArgs, parts...)
			continue
		}
		if len(parts) >= 2 && strings.HasPrefix(parts[0], "0x") {
			files[parts[0]] = parts[1]
		}
	}
	resolved, err := resolveFlashFiles(base, files)
	if err != nil {
		return nil, err
	}
	analysis.Root = base
	analysis.Kind = KindFlash
	analysis.CanFlash = true
	analysis.FlashFiles = resolved
	analysis.Messages = append(analysis.Messages, "检测到 flash_args，可直接刷机。")
	return analysis, nil
}

func resolveFlashFiles(base string, files map[string]string) ([]FlashFile, error) {
	offsets := make([]string, 0, len(files))
	for offset := range files {
		offsets = append(offsets, offset)
	}
	sort.Slice(offsets, func(i, j int) bool {
		return parseOffset(offsets[i]) < parseOffset(offsets[j])
	})

	out := make([]FlashFile, 0, len(offsets))
	for _, offset := range offsets {
		fullPath := filepath.Join(base, filepath.FromSlash(files[offset]))
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("缺少刷机文件 %s：%w", files[offset], err)
		}
		out = append(out, FlashFile{Offset: offset, Path: fullPath, Size: info.Size()})
	}
	return out, nil
}

func findFirst(root string, name string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), name) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func isIDFSource(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "CMakeLists.txt")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(root, "main")); err != nil {
		return false
	}
	return true
}

func findIDFSourceRoot(root string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" || !d.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		switch strings.ToLower(d.Name()) {
		case ".git", ".github", "__macosx", "build", "managed_components":
			return filepath.SkipDir
		}
		if isIDFSource(path) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func collapseSingleRoot(root string) string {
	for {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 || !entries[0].IsDir() {
			return root
		}
		child := filepath.Join(root, entries[0].Name())
		childEntries, err := os.ReadDir(child)
		if err != nil {
			return child
		}
		for _, entry := range childEntries {
			if err := os.Rename(filepath.Join(child, entry.Name()), filepath.Join(root, entry.Name())); err != nil {
				return child
			}
		}
		if err := os.Remove(child); err != nil {
			return child
		}
	}
}

func shortPackageID() string {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "p" + hex.EncodeToString(bytes[:])
	}
	return "p" + strconv.FormatInt(time.Now().UnixNano()%2176782336, 36)
}

func createPackageRoot(workspace string, nextID func() string) (string, error) {
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", err
	}
	for attempt := 0; attempt < 64; attempt++ {
		id := strings.TrimSpace(nextID())
		if id == "" {
			continue
		}
		root := filepath.Join(workspace, id)
		if err := os.Mkdir(root, 0o755); err == nil {
			return root, nil
		} else if os.IsExist(err) {
			continue
		} else {
			return "", err
		}
	}
	return "", errors.New("无法分配新的短固件工作目录")
}

func unrecognizedPackageSummary(zipPath string, extractedRoot string) string {
	parts := []string{}
	if zipSummary := summarizeZipEntries(zipPath, 16); zipSummary != "" {
		parts = append(parts, "zip entries: "+zipSummary)
	}
	if dirSummary := summarizeDirEntries(extractedRoot, 16); dirSummary != "" {
		parts = append(parts, "extracted entries: "+dirSummary)
	}
	if extractedRoot != "" {
		parts = append(parts, "extracted root: "+extractedRoot)
	}
	return strings.Join(parts, "; ")
}

func summarizeZipEntries(zipPath string, limit int) string {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "cannot reopen zip: " + err.Error()
	}
	defer reader.Close()

	names := make([]string, 0, limit+1)
	for _, file := range reader.File {
		name := strings.TrimRight(file.Name, "/\\")
		if name == "" {
			continue
		}
		names = append(names, name)
		if len(names) >= limit {
			break
		}
	}
	if len(reader.File) > limit {
		names = append(names, "...")
	}
	if len(names) == 0 {
		return "empty zip"
	}
	return strings.Join(names, ", ")
}

func summarizeDirEntries(root string, limit int) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "cannot read extracted root: " + err.Error()
	}
	names := make([]string, 0, limit+1)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
		if len(names) >= limit {
			break
		}
	}
	if len(entries) > limit {
		names = append(names, "...")
	}
	if len(names) == 0 {
		return "empty extracted root"
	}
	return strings.Join(names, ", ")
}

func isInside(root string, target string) bool {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, absTarget)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func parseOffset(offset string) int64 {
	v, err := strconv.ParseInt(strings.TrimPrefix(offset, "0x"), 16, 64)
	if err != nil {
		return 0
	}
	return v
}

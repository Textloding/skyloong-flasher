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
	"path"
	"path/filepath"
	goruntime "runtime"
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
		return nil, errors.New("please choose firmware zip file")
	}
	if info, err := os.Stat(windowsFilesystemPath(zipPath)); err != nil {
		return nil, fmt.Errorf("firmware zip not found: %s", zipPath)
	} else if info.IsDir() {
		return nil, fmt.Errorf("selected path is a directory, not a firmware zip: %s", zipPath)
	}
	if workspace == "" {
		workspace = os.TempDir()
	}
	root, err := createPackageRoot(workspace, shortPackageID)
	if err != nil {
		return nil, fmt.Errorf("cannot create firmware extraction dir: %w", err)
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
	reader, err := zip.OpenReader(windowsFilesystemPath(zipPath))
	if err != nil {
		return "", fmt.Errorf("cannot open zip: %w", err)
	}
	defer reader.Close()

	cleanDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(windowsFilesystemPath(cleanDest), 0o755); err != nil {
		return "", err
	}

	stripPrefix := zipSingleRootPrefix(reader.File)
	for _, file := range reader.File {
		name, skip, err := stripZipRootPrefix(file.Name, stripPrefix)
		if err != nil {
			return "", err
		}
		if skip {
			continue
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(name))
		if !isInside(cleanDest, target) {
			return "", fmt.Errorf("unsafe zip path: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(windowsFilesystemPath(target), 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(windowsFilesystemPath(filepath.Dir(target)), 0o755); err != nil {
			return "", err
		}
		src, err := file.Open()
		if err != nil {
			return "", err
		}
		dst, err := os.OpenFile(windowsFilesystemPath(target), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.FileInfo().Mode())
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

type zipNameParts struct {
	parts []string
	isDir bool
}

func zipSingleRootPrefix(files []*zip.File) []string {
	items := make([]zipNameParts, 0, len(files))
	for _, file := range files {
		name, skip, err := cleanZipName(file.Name)
		if err != nil || skip {
			continue
		}
		items = append(items, zipNameParts{
			parts: strings.Split(name, "/"),
			isDir: file.FileInfo().IsDir(),
		})
	}

	prefix := []string{}
	for {
		first := ""
		hasChild := false
		active := 0
		for _, item := range items {
			if len(item.parts) == 0 {
				continue
			}
			active++
			if first == "" {
				first = item.parts[0]
			} else if item.parts[0] != first {
				return prefix
			}
			if len(item.parts) > 1 || item.isDir {
				hasChild = true
			}
		}
		if active == 0 || first == "" || !hasChild {
			return prefix
		}
		prefix = append(prefix, first)
		for i := range items {
			if len(items[i].parts) > 0 {
				items[i].parts = items[i].parts[1:]
			}
		}
	}
}

func stripZipRootPrefix(rawName string, prefix []string) (string, bool, error) {
	name, skip, err := cleanZipName(rawName)
	if err != nil || skip {
		return "", skip, err
	}
	parts := strings.Split(name, "/")
	if len(parts) >= len(prefix) {
		matches := true
		for i, segment := range prefix {
			if parts[i] != segment {
				matches = false
				break
			}
		}
		if matches {
			parts = parts[len(prefix):]
		}
	}
	if len(parts) == 0 {
		return "", true, nil
	}
	return strings.Join(parts, "/"), false, nil
}

func cleanZipName(rawName string) (string, bool, error) {
	name := strings.ReplaceAll(rawName, "\\", "/")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", true, nil
	}
	if path.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", false, fmt.Errorf("unsafe zip path: %s", rawName)
	}
	firstSegment := strings.Split(name, "/")[0]
	if strings.Contains(firstSegment, ":") {
		return "", false, fmt.Errorf("unsafe zip path: %s", rawName)
	}
	clean := path.Clean(name)
	if clean == "." {
		return "", true, nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("unsafe zip path: %s", rawName)
	}
	return clean, false, nil
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

	if isIDFSource(absRoot) {
		return analyzeSourceRoot(analysis, absRoot)
	}
	if sourceRoot := findIDFSourceRoot(absRoot); sourceRoot != "" {
		return analyzeSourceRoot(analysis, sourceRoot)
	}

	flasherPath := findFirst(absRoot, "flasher_args.json")
	if flasherPath != "" {
		return analyzeFlasherJSON(analysis, flasherPath)
	}

	flashArgsPath := findFirst(absRoot, "flash_args")
	if flashArgsPath != "" {
		return analyzeFlashArgs(analysis, flashArgsPath)
	}

	return analysis, errors.New("missing flasher_args.json, flash_args or ESP-IDF source structure")
}

func analyzeSourceRoot(analysis *Analysis, sourceRoot string) (*Analysis, error) {
	buildDir := filepath.Join(sourceRoot, "build")
	if flasherPath := findFirst(buildDir, "flasher_args.json"); flasherPath != "" {
		return analyzeFlasherJSON(analysis, flasherPath)
	}
	if flashArgsPath := findFirst(buildDir, "flash_args"); flashArgsPath != "" {
		return analyzeFlashArgs(analysis, flashArgsPath)
	}
	analysis.Root = sourceRoot
	analysis.ProjectName = filepath.Base(sourceRoot)
	analysis.Kind = KindSource
	analysis.NeedsBuild = true
	analysis.Messages = append(analysis.Messages, "Detected flashable ESP-IDF build output.")
	return analysis, nil
}

func analyzeFlasherJSON(analysis *Analysis, path string) (*Analysis, error) {
	raw, err := os.ReadFile(windowsFilesystemPath(path))
	if err != nil {
		return nil, err
	}
	var data flasherArgsJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse flasher_args.json failed: %w", err)
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
	analysis.Messages = append(analysis.Messages, "Detected ESP-IDF source package; build is required before flashing.")
	return analysis, nil
}

func analyzeFlashArgs(analysis *Analysis, path string) (*Analysis, error) {
	raw, err := os.ReadFile(windowsFilesystemPath(path))
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
	analysis.Messages = append(analysis.Messages, "Detected flash_args; package can be flashed directly.")
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
		info, err := os.Stat(windowsFilesystemPath(fullPath))
		if err != nil {
			return nil, fmt.Errorf("missing flash file %s: %w", files[offset], err)
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
	if _, err := os.Stat(windowsFilesystemPath(filepath.Join(root, "CMakeLists.txt"))); err != nil {
		return false
	}
	if _, err := os.Stat(windowsFilesystemPath(filepath.Join(root, "main"))); err != nil {
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
		entries, err := os.ReadDir(windowsFilesystemPath(root))
		if err != nil || len(entries) != 1 || !entries[0].IsDir() {
			return root
		}
		child := filepath.Join(root, entries[0].Name())
		root = child
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
	if err := os.MkdirAll(windowsFilesystemPath(workspace), 0o755); err != nil {
		return "", err
	}
	for attempt := 0; attempt < 64; attempt++ {
		id := strings.TrimSpace(nextID())
		if id == "" {
			continue
		}
		root := filepath.Join(workspace, id)
		if err := os.Mkdir(windowsFilesystemPath(root), 0o755); err == nil {
			return root, nil
		} else if os.IsExist(err) {
			continue
		} else {
			return "", err
		}
	}
	return "", errors.New("cannot allocate short firmware work dir")
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
	reader, err := zip.OpenReader(windowsFilesystemPath(zipPath))
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
	entries, err := os.ReadDir(windowsFilesystemPath(root))
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

package packagekit

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeZipWithFlasherArgs(t *testing.T) {
	zipPath := makeZip(t, map[string]string{
		"firmware/build/flasher_args.json": `{
			"write_flash_args":["--flash_mode","dio","--flash_size","detect","--flash_freq","80m"],
			"flash_files":{
				"0x0":"bootloader/bootloader.bin",
				"0x8000":"partition_table/partition-table.bin",
				"0x10000":"ota_data_initial.bin",
				"0x20000":"GK87-Screen.bin"
			},
			"extra_esptool_args":{"chip":"esp32s3","before":"default_reset","after":"hard_reset"}
		}`,
		"firmware/build/bootloader/bootloader.bin":           "boot",
		"firmware/build/partition_table/partition-table.bin": "part",
		"firmware/build/ota_data_initial.bin":                "ota",
		"firmware/build/GK87-Screen.bin":                     "app",
	})

	analysis, err := AnalyzeZip(zipPath, t.TempDir())
	if err != nil {
		t.Fatalf("AnalyzeZip() error = %v", err)
	}

	if !analysis.CanFlash {
		t.Fatalf("expected CanFlash")
	}
	if analysis.NeedsBuild {
		t.Fatalf("did not expect NeedsBuild")
	}
	if analysis.Chip != "esp32s3" {
		t.Fatalf("chip = %q", analysis.Chip)
	}
	if len(analysis.FlashFiles) != 4 {
		t.Fatalf("flash files = %d", len(analysis.FlashFiles))
	}
	if analysis.FlashFiles[0].Offset != "0x0" || analysis.FlashFiles[3].Offset != "0x20000" {
		t.Fatalf("unexpected flash file ordering: %#v", analysis.FlashFiles)
	}
}

func TestAnalyzeZipWithSourceOnlyProject(t *testing.T) {
	workspace := t.TempDir()
	zipPath := makeZip(t, map[string]string{
		"SKYLOONG-main/CMakeLists.txt": "idf_component_register()",
		"SKYLOONG-main/main/main.cpp":  "void app_main(){}",
		"SKYLOONG-main/sdkconfig":      "CONFIG_IDF_TARGET=\"esp32s3\"",
	})

	analysis, err := AnalyzeZip(zipPath, workspace)
	if err != nil {
		t.Fatalf("AnalyzeZip() error = %v", err)
	}
	if analysis.CanFlash {
		t.Fatalf("source-only project should not be directly flashable")
	}
	if !analysis.NeedsBuild {
		t.Fatalf("source-only project should need build")
	}
	if analysis.Kind != KindSource {
		t.Fatalf("kind = %q", analysis.Kind)
	}
	if filepath.Base(analysis.Root) == "SKYLOONG-main" {
		t.Fatalf("single zip root should be flattened to keep build paths short, got %q", analysis.Root)
	}
	if !strings.HasPrefix(analysis.Root, workspace) {
		t.Fatalf("analysis root %q should stay under workspace %q", analysis.Root, workspace)
	}
	if len(filepath.Base(analysis.Root)) > 10 {
		t.Fatalf("package work dir base should be short, got %q", filepath.Base(analysis.Root))
	}
}

func TestZipSingleRootPrefixStripsNestedArchiveRoots(t *testing.T) {
	zipPath := makeZip(t, map[string]string{
		"archive/SKYLOONG-main/CMakeLists.txt":           "idf_component_register()",
		"archive/SKYLOONG-main/main/main.cpp":            "void app_main(){}",
		"archive/SKYLOONG-main/tools/web_new/index.html": "<html></html>",
	})
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	prefix := zipSingleRootPrefix(reader.File)
	if got, want := strings.Join(prefix, "/"), "archive/SKYLOONG-main"; got != want {
		t.Fatalf("prefix = %q, want %q", got, want)
	}
	name, skip, err := stripZipRootPrefix("archive/SKYLOONG-main/CMakeLists.txt", prefix)
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatalf("CMakeLists.txt should not be skipped")
	}
	if name != "CMakeLists.txt" {
		t.Fatalf("stripped name = %q", name)
	}
}

func TestAnalyzeZipFindsNestedSourceProject(t *testing.T) {
	workspace := t.TempDir()
	zipPath := makeZip(t, map[string]string{
		"archive/SKYLOONG-main/CMakeLists.txt": "idf_component_register()",
		"archive/SKYLOONG-main/main/main.cpp":  "void app_main(){}",
		"archive/SKYLOONG-main/sdkconfig":      "CONFIG_IDF_TARGET=\"esp32s3\"",
	})

	analysis, err := AnalyzeZip(zipPath, workspace)
	if err != nil {
		t.Fatalf("AnalyzeZip() error = %v", err)
	}
	if analysis.Kind != KindSource {
		t.Fatalf("kind = %q", analysis.Kind)
	}
	if !analysis.NeedsBuild {
		t.Fatalf("nested source project should need build")
	}
	if filepath.Base(analysis.Root) == "SKYLOONG-main" {
		t.Fatalf("nested single zip roots should be flattened to keep build paths short, got %q", analysis.Root)
	}
	if !strings.HasPrefix(analysis.Root, workspace) {
		t.Fatalf("analysis root %q should stay under workspace %q", analysis.Root, workspace)
	}
}

func TestAnalyzeZipUnrecognizedPackageIncludesZipSummary(t *testing.T) {
	zipPath := makeZip(t, map[string]string{
		"not-firmware/README.md": "hello",
	})

	_, err := AnalyzeZip(zipPath, t.TempDir())
	if err == nil {
		t.Fatalf("expected unrecognized package error")
	}
	if !strings.Contains(err.Error(), "README.md") {
		t.Fatalf("error should include zip content summary, got: %v", err)
	}
}

func TestCreatePackageRootSkipsExistingShortDirectory(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "pabc"), 0o755); err != nil {
		t.Fatal(err)
	}
	ids := []string{"pabc", "pdef"}
	nextID := func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}

	root, err := createPackageRoot(workspace, nextID)
	if err != nil {
		t.Fatalf("createPackageRoot() error = %v", err)
	}
	if filepath.Base(root) != "pdef" {
		t.Fatalf("root = %q, want pdef after collision", root)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("new package root should exist: %v", err)
	}
}

func TestAnalyzeZipRejectsPathTraversal(t *testing.T) {
	zipPath := makeZip(t, map[string]string{
		"../evil.txt": "owned",
	})
	_, err := AnalyzeZip(zipPath, t.TempDir())
	if err == nil {
		t.Fatalf("expected traversal zip to be rejected")
	}
}

func TestAnalyzeZipMissingFileHasFriendlyMessage(t *testing.T) {
	_, err := AnalyzeZip(filepath.Join(t.TempDir(), "missing.zip"), t.TempDir())
	if err == nil {
		t.Fatalf("expected missing zip error")
	}
	if !strings.Contains(err.Error(), "firmware zip not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAnalyzeDirReadsFirmwareMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.16)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := `{
		"schema_version": 1,
		"hardware_version": "SCM_V4.0",
		"display": "320x240-st7789-8bit-parallel",
		"minimum_flash_bytes": 16777216,
		"minimum_psram_bytes": 8388608,
		"psram_mode": "octal"
	}`
	if err := os.WriteFile(filepath.Join(root, "skyloong_firmware.json"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeDir(root)
	if err != nil {
		t.Fatalf("AnalyzeDir() error = %v", err)
	}
	assertFirmwareMetadata(t, analysis)
}

func TestAnalyzeZipReadsFirmwareMetadata(t *testing.T) {
	zipPath := makeZip(t, map[string]string{
		"SKYLOONG-main/CMakeLists.txt": "cmake_minimum_required(VERSION 3.16)",
		"SKYLOONG-main/main/main.cpp":  "void app_main(){}",
		"SKYLOONG-main/skyloong_firmware.json": `{
			"schema_version": 1,
			"hardware_version": "SCM_V4.0",
			"display": "320x240-st7789-8bit-parallel",
			"minimum_flash_bytes": 16777216,
			"minimum_psram_bytes": 8388608,
			"psram_mode": "octal"
		}`,
	})

	analysis, err := AnalyzeZip(zipPath, t.TempDir())
	if err != nil {
		t.Fatalf("AnalyzeZip() error = %v", err)
	}
	assertFirmwareMetadata(t, analysis)
}

func TestAnalyzeDirWithoutFirmwareMetadataStaysCompatible(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.16)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "main"), 0o755); err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeDir(root)
	if err != nil {
		t.Fatalf("AnalyzeDir() error = %v", err)
	}
	if analysis.HardwareVersion != "" || analysis.MinimumFlashBytes != 0 || analysis.MinimumPSRAMBytes != 0 {
		t.Fatalf("unexpected metadata for legacy package: %#v", analysis)
	}
}

func assertFirmwareMetadata(t *testing.T, analysis *Analysis) {
	t.Helper()
	if analysis.HardwareVersion != "SCM_V4.0" {
		t.Fatalf("hardware version = %q", analysis.HardwareVersion)
	}
	if analysis.Display != "320x240-st7789-8bit-parallel" {
		t.Fatalf("display = %q", analysis.Display)
	}
	if analysis.MinimumFlashBytes != 16*1024*1024 {
		t.Fatalf("minimum flash = %d", analysis.MinimumFlashBytes)
	}
	if analysis.MinimumPSRAMBytes != 8*1024*1024 {
		t.Fatalf("minimum PSRAM = %d", analysis.MinimumPSRAMBytes)
	}
	if analysis.PSRAMMode != "octal" {
		t.Fatalf("PSRAM mode = %q", analysis.PSRAMMode)
	}
}

func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "pkg.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

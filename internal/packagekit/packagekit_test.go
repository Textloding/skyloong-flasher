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
	if !strings.Contains(err.Error(), "找不到固件 zip 文件") {
		t.Fatalf("unexpected error: %v", err)
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

package runtimekit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEIMInstallCommandUsesNonInteractiveCacheInstall(t *testing.T) {
	cmd := EIMInstallCommand(`C:\tools\eim.exe`, `C:\cache\eim`, `C:\cache\runtime`, "v5.1.4")
	got := strings.Join(cmd.Args, " ")

	for _, want := range []string{
		`C:\tools\eim.exe`,
		"install",
		"--path C:\\cache\\runtime",
		"--idf-versions v5.1.4",
		"--target esp32s3",
		"--non-interactive true",
		"--install-all-prerequisites true",
		"--esp-idf-json-path C:\\cache\\eim",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install command %q missing %q", got, want)
		}
	}
}

func TestEIMRunCommandWrapsCommandAndVersion(t *testing.T) {
	status := Status{
		Kind:        KindEIM,
		EIMPath:     `C:\tools\eim.exe`,
		EIMJsonPath: `C:\cache\eim`,
		IDFVersion:  "v5.1.4",
	}

	cmd := EIMRunCommand(status, "idf.py", "build")
	got := strings.Join(cmd.Args, " ")

	for _, want := range []string{
		`C:\tools\eim.exe`,
		"run",
		"--esp-idf-json-path C:\\cache\\eim",
		"idf.py build",
		"v5.1.4",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run command %q missing %q", got, want)
		}
	}
}

func TestDetectCachedEIMUsesToolCacheLayout(t *testing.T) {
	root := t.TempDir()
	eimPath := filepath.Join(root, "tools", "eim.exe")
	configPath := filepath.Join(root, "runtime", "eim-config", "eim_idf.json")
	if err := os.MkdirAll(filepath.Dir(eimPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eimPath, []byte("probe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, ok := detectCachedEIM(root)
	if !ok {
		t.Fatalf("expected cached EIM to be detected")
	}
	if !status.Available || !status.CanBuild || status.Kind != KindEIM {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestDetectCachedEIMUsesPortableRuntimeLayout(t *testing.T) {
	root := t.TempDir()
	eimPath := filepath.Join(root, "runtime", "tools", "eim.exe")
	configPath := filepath.Join(root, "runtime", "eim-config", "eim_idf.json")
	if err := os.MkdirAll(filepath.Dir(eimPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eimPath, []byte("probe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, ok := detectCachedEIM(root)
	if !ok {
		t.Fatalf("expected portable EIM runtime to be detected")
	}
	if status.EIMPath != eimPath {
		t.Fatalf("EIMPath = %q, want %q", status.EIMPath, eimPath)
	}
}

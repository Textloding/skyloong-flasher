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

func TestEIMDownloadSourcesPreferEspressifMirrors(t *testing.T) {
	sources := EIMDownloadSources()
	if len(sources) < 3 {
		t.Fatalf("expected mirror fallback sources, got %#v", sources)
	}
	if !strings.Contains(sources[0], "dl.espressif.cn/github_assets") {
		t.Fatalf("first source should be China mirror, got %q", sources[0])
	}
	if !strings.Contains(sources[1], "dl.espressif.com/github_assets") {
		t.Fatalf("second source should be international Espressif mirror, got %q", sources[1])
	}
	if !strings.Contains(sources[len(sources)-1], "github.com/espressif/idf-im-ui") {
		t.Fatalf("last source should be GitHub fallback, got %q", sources[len(sources)-1])
	}
}

func TestEIMInstallCommandUsesEspressifMirrorEnvironment(t *testing.T) {
	cmd := EIMInstallCommand(`C:\tools\eim.exe`, `C:\cache\eim`, `C:\cache\runtime`, "v5.1.4")
	env := strings.Join(cmd.Env, "\n")

	for _, want := range []string{
		"IDF_GITHUB_ASSETS=dl.espressif.cn/github_assets",
		"PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple",
		"PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("install env missing %q in:\n%s", want, env)
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

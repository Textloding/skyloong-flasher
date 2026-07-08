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

func TestGitDownloadSourcesPreferDomesticMirrors(t *testing.T) {
	sources := GitDownloadSources()
	if len(sources) < 3 {
		t.Fatalf("expected Git mirror fallback sources, got %#v", sources)
	}
	if !strings.Contains(sources[0], "mirrors.huaweicloud.com/git-for-windows") {
		t.Fatalf("first Git source should be Huawei Cloud mirror, got %q", sources[0])
	}
	if !strings.Contains(sources[1], "registry.npmmirror.com") {
		t.Fatalf("second Git source should be npmmirror, got %q", sources[1])
	}
	if !strings.Contains(sources[len(sources)-1], "github.com/git-for-windows/git") {
		t.Fatalf("last Git source should be GitHub fallback, got %q", sources[len(sources)-1])
	}
}

func TestEIMInstallCommandUsesEspressifMirrorEnvironment(t *testing.T) {
	cmd := EIMInstallCommand(`C:\tools\eim.exe`, `C:\cache\eim`, `C:\cache\runtime`, "v5.1.4")
	env := strings.Join(cmd.Env, "\n")

	for _, want := range []string{
		"IDF_GITHUB_ASSETS=dl.espressif.cn/github_assets",
		"IDF_COMPONENT_STORAGE_URL=https://components-file.espressif.cn;https://components-file.espressif.com",
		"PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple",
		"PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("install env missing %q in:\n%s", want, env)
		}
	}
}

func TestEIMInstallCommandPrefixesPortableGitPath(t *testing.T) {
	cmd := EIMInstallCommand(`C:\tools\eim.exe`, `C:\cache\eim`, `C:\cache\runtime`, "v5.1.4", `C:\cache\tools\git\cmd\git.exe`)
	pathValue := envValue(cmd.Env, "PATH")
	if pathValue == "" {
		t.Fatalf("expected PATH in env: %#v", cmd.Env)
	}
	for _, want := range []string{
		`C:\cache\tools\git\cmd`,
		`C:\cache\tools\git\mingw64\bin`,
		`C:\cache\tools\git\usr\bin`,
	} {
		if !strings.Contains(pathValue, want) {
			t.Fatalf("PATH %q missing %q", pathValue, want)
		}
	}
	if !strings.HasPrefix(pathValue, `C:\cache\tools\git\cmd;`) {
		t.Fatalf("portable Git cmd dir should be first in PATH, got %q", pathValue)
	}
}

func TestEIMRunCommandWrapsCommandAndVersion(t *testing.T) {
	status := Status{
		Kind:               KindEIM,
		EIMPath:            `C:\tools\eim.exe`,
		EIMJsonPath:        `C:\cache\eim`,
		IDFVersion:         "v5.1.4",
		GitPath:            `C:\cache\tools\git\cmd\git.exe`,
		ComponentCachePath: `C:\cache\cm`,
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
	if pathValue := envValue(cmd.Env, "PATH"); !strings.HasPrefix(pathValue, `C:\cache\tools\git\cmd;`) {
		t.Fatalf("portable Git cmd dir should be first in PATH, got %q", pathValue)
	}
	if got := envValue(cmd.Env, "IDF_COMPONENT_CACHE_PATH"); got != `C:\cache\cm` {
		t.Fatalf("IDF_COMPONENT_CACHE_PATH = %q", got)
	}
}

func TestDetectInSetsComponentCachePath(t *testing.T) {
	t.Setenv("SKYLOONG_COMPONENT_CACHE_PATH", "")
	root := `C:\Users\Administrator\AppData\Local\SkyloongFlasher`
	status := DetectIn(root)
	want := filepath.Join(`C:\`, "SLCM")
	if status.ComponentCachePath != want {
		t.Fatalf("ComponentCachePath = %q, want %q", status.ComponentCachePath, want)
	}
	if strings.Contains(strings.ToLower(status.ComponentCachePath), strings.ToLower(root)) {
		t.Fatalf("component cache path should not stay under long cache dir: %q", status.ComponentCachePath)
	}
}

func TestComponentCachePathRespectsOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "idf-components")
	t.Setenv("SKYLOONG_COMPONENT_CACHE_PATH", override)

	status := DetectIn(`C:\Users\Administrator\AppData\Local\SkyloongFlasher`)
	if status.ComponentCachePath != override {
		t.Fatalf("ComponentCachePath = %q, want override %q", status.ComponentCachePath, override)
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

func TestDetectCachedGitUsesToolCacheLayout(t *testing.T) {
	root := t.TempDir()
	gitPath := filepath.Join(root, "tools", "git", "cmd", "git.exe")
	if err := os.MkdirAll(filepath.Dir(gitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitPath, []byte("probe"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := detectCachedGit(root)
	if !ok {
		t.Fatalf("expected cached Git to be detected")
	}
	if got != gitPath {
		t.Fatalf("git path = %q, want %q", got, gitPath)
	}
}

func TestDetectCachedGitUsesPortableRuntimeLayout(t *testing.T) {
	root := t.TempDir()
	gitPath := filepath.Join(root, "runtime", "tools", "git", "cmd", "git.exe")
	if err := os.MkdirAll(filepath.Dir(gitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitPath, []byte("probe"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := detectCachedGit(root)
	if !ok {
		t.Fatalf("expected portable runtime Git to be detected")
	}
	if got != gitPath {
		t.Fatalf("git path = %q, want %q", got, gitPath)
	}
}

func envValue(env []string, key string) string {
	for _, item := range env {
		gotKey, value, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(gotKey, key) {
			return value
		}
	}
	return ""
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogHistoryKeepsFirstAndLastLine(t *testing.T) {
	app := NewApp()
	app.cacheDir = t.TempDir()
	if err := app.prepareLogFile(); err != nil {
		t.Fatalf("prepareLogFile() error = %v", err)
	}

	for i := 0; i < 220; i++ {
		app.logLine("line " + formatTestNumber(i))
	}

	history := app.GetLogHistory()
	if len(history) != 220 {
		t.Fatalf("history len = %d, want 220", len(history))
	}
	if history[0] != "line 000" {
		t.Fatalf("first log = %q", history[0])
	}
	if history[len(history)-1] != "line 219" {
		t.Fatalf("last log = %q", history[len(history)-1])
	}

	raw, err := os.ReadFile(app.logFile)
	if err != nil {
		t.Fatalf("ReadFile(logFile) error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "line 000") || !strings.Contains(text, "line 219") {
		t.Fatalf("log file should contain first and last line, got:\n%s", text)
	}
}

func TestPrepareCacheDirsCreatesExpectedFolders(t *testing.T) {
	app := NewApp()
	app.cacheDir = t.TempDir()

	if err := app.prepareCacheDirs(); err != nil {
		t.Fatalf("prepareCacheDirs() error = %v", err)
	}

	for _, dir := range []string{"downloads", "packages", "runtime", "tools", "logs"} {
		if info, err := os.Stat(filepath.Join(app.cacheDir, dir)); err != nil || !info.IsDir() {
			t.Fatalf("expected %s directory to exist, info=%v err=%v", dir, info, err)
		}
	}
	if info, err := os.Stat(filepath.Join(app.cacheDir, "cm")); err != nil || !info.IsDir() {
		t.Fatalf("expected component cache directory to exist, info=%v err=%v", info, err)
	}
}

func formatTestNumber(n int) string {
	return string(rune('0'+n/100)) + string(rune('0'+n/10%10)) + string(rune('0'+n%10))
}

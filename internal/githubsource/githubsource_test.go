package githubsource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseRepositoryURL(t *testing.T) {
	spec, err := Parse("https://github.com/Textloding/SKYLOONG.git")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if spec.Owner != "Textloding" || spec.Repo != "SKYLOONG" {
		t.Fatalf("unexpected repo: %#v", spec)
	}
	if spec.RefType != RefDefault {
		t.Fatalf("ref type = %q", spec.RefType)
	}
}

func TestParseBranchURL(t *testing.T) {
	spec, err := Parse("https://github.com/Textloding/SKYLOONG/tree/idf-v5.1.4")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if spec.RefType != RefBranch || spec.Ref != "idf-v5.1.4" {
		t.Fatalf("unexpected branch spec: %#v", spec)
	}
	if got := spec.ArchiveURL(); got != "https://github.com/Textloding/SKYLOONG/archive/refs/heads/idf-v5.1.4.zip" {
		t.Fatalf("ArchiveURL() = %q", got)
	}
}

func TestParseArchiveURL(t *testing.T) {
	spec, err := Parse("https://github.com/Textloding/SKYLOONG/archive/refs/heads/main.zip")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if spec.RefType != RefBranch || spec.Ref != "main" {
		t.Fatalf("unexpected archive spec: %#v", spec)
	}
}

func TestParseCodeloadURL(t *testing.T) {
	spec, err := Parse("https://codeload.github.com/Textloding/SKYLOONG/zip/refs/tags/v1.0.0")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if spec.RefType != RefTag || spec.Ref != "v1.0.0" {
		t.Fatalf("unexpected codeload spec: %#v", spec)
	}
}

func TestDownloadCreatesDestinationFolder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("zip-body"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "nested", "firmware.zip")
	if err := Download(context.Background(), server.URL, dest, nil); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile(dest) error = %v", err)
	}
	if string(raw) != "zip-body" {
		t.Fatalf("downloaded body = %q", string(raw))
	}
}

package githubsource

import "testing"

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

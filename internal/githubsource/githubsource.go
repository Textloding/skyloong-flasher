package githubsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

const (
	RefDefault = "default"
	RefBranch  = "branch"
	RefTag     = "tag"
)

type Spec struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	RefType string `json:"refType"`
	Ref     string `json:"ref"`
	RawURL  string `json:"rawUrl"`
}

type Progress func(downloaded int64, total int64)

func Parse(raw string) (Spec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Spec{}, errors.New("请输入 GitHub 链接")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Spec{}, err
	}
	host := strings.ToLower(u.Host)
	parts := splitPath(u.Path)
	if host == "github.com" {
		return parseGithub(raw, parts)
	}
	if host == "codeload.github.com" {
		return parseCodeload(raw, parts)
	}
	return Spec{}, errors.New("目前只支持 github.com 或 codeload.github.com 链接")
}

func (s Spec) ArchiveURL() string {
	if s.RefType == RefTag {
		return fmt.Sprintf("https://github.com/%s/%s/archive/refs/tags/%s.zip", s.Owner, s.Repo, s.Ref)
	}
	if s.RefType == RefBranch && s.Ref != "" {
		return fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", s.Owner, s.Repo, s.Ref)
	}
	return fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/main.zip", s.Owner, s.Repo)
}

func Download(ctx context.Context, archiveURL string, dest string, progress Progress) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(windowsFilesystemPath(filepath.Dir(dest)), 0o755); err != nil {
		return fmt.Errorf("无法创建下载文件夹：%w", err)
	}
	out, err := os.Create(windowsFilesystemPath(dest))
	if err != nil {
		return fmt.Errorf("无法创建下载文件：%w", err)
	}
	defer out.Close()

	var downloaded int64
	buf := make([]byte, 128*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(written)
			if progress != nil {
				progress(downloaded, resp.ContentLength)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func parseGithub(raw string, parts []string) (Spec, error) {
	if len(parts) < 2 {
		return Spec{}, errors.New("GitHub 链接缺少 owner/repo")
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	spec := Spec{Owner: parts[0], Repo: repo, RefType: RefDefault, RawURL: raw}
	if len(parts) >= 4 && parts[2] == "tree" {
		spec.RefType = RefBranch
		spec.Ref = strings.Join(parts[3:], "/")
	}
	if len(parts) >= 6 && parts[2] == "archive" && parts[3] == "refs" {
		switch parts[4] {
		case "heads":
			spec.RefType = RefBranch
		case "tags":
			spec.RefType = RefTag
		}
		spec.Ref = strings.TrimSuffix(strings.Join(parts[5:], "/"), ".zip")
	}
	return spec, nil
}

func parseCodeload(raw string, parts []string) (Spec, error) {
	if len(parts) < 6 {
		return Spec{}, errors.New("codeload 链接格式不完整")
	}
	spec := Spec{Owner: parts[0], Repo: parts[1], RawURL: raw}
	if parts[3] == "refs" && parts[4] == "heads" {
		spec.RefType = RefBranch
		spec.Ref = strings.Join(parts[5:], "/")
		return spec, nil
	}
	if parts[3] == "refs" && parts[4] == "tags" {
		spec.RefType = RefTag
		spec.Ref = strings.Join(parts[5:], "/")
		return spec, nil
	}
	return Spec{}, errors.New("不支持的 codeload 引用类型")
}

func splitPath(p string) []string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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

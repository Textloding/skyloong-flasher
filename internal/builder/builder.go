package builder

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/Textloding/skyloong-flasher/internal/processutil"
	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

type LogFunc func(line string)

func BuildCommand(status runtimekit.Status, sourceRoot string) (*exec.Cmd, error) {
	if sourceRoot == "" {
		return nil, errors.New("源码目录为空")
	}
	if !status.CanBuild {
		return nil, errors.New("未检测到 ESP-IDF 构建环境")
	}
	if status.Kind == runtimekit.KindEIM {
		return runtimekit.EIMRunCommand(status, "idf.py", "build"), nil
	}
	if status.ExportScript != "" {
		script := ". '" + strings.ReplaceAll(status.ExportScript, "'", "''") + "'; idf.py build"
		cmd := processutil.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
		cmd.Env = runtimekit.CommandEnv(status)
		return cmd, nil
	}
	if status.IDFPyPath != "" {
		cmd := processutil.Command(status.IDFPyPath, "build")
		cmd.Env = runtimekit.CommandEnv(status)
		return cmd, nil
	}
	return nil, errors.New("未找到 idf.py 或 export.ps1")
}

func Run(ctx context.Context, status runtimekit.Status, sourceRoot string, log LogFunc) error {
	cmd, err := BuildCommand(status, sourceRoot)
	if err != nil {
		return err
	}
	command := processutil.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	command.Dir = sourceRoot
	command.Env = cmd.Env
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if log != nil {
		log(strings.Join(cmd.Args, " "))
	}
	if err := command.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	state := newBuildLogState()
	pipe := func(scanner *bufio.Scanner) {
		defer wg.Done()
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			state.Observe(line)
			if log != nil {
				log(line)
			}
		}
		if err := scanner.Err(); err != nil && log != nil {
			log("日志读取失败：" + err.Error())
		}
	}
	wg.Add(2)
	go pipe(bufio.NewScanner(stdout))
	go pipe(bufio.NewScanner(stderr))
	wg.Wait()
	return state.Err(command.Wait())
}

type buildLogState struct {
	mu                    sync.Mutex
	failed                bool
	reason                string
	sawFileNotFound       bool
	sawComponentCachePath bool
}

func newBuildLogState() *buildLogState {
	return &buildLogState{}
}

func (s *buildLogState) Observe(line string) {
	lower := strings.ToLower(line)
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.Contains(lower, "filenotfounderror") || strings.Contains(lower, "no such file or directory") {
		s.sawFileNotFound = true
	}
	if strings.Contains(lower, "componentmanager") && strings.Contains(lower, "cache") {
		s.sawComponentCachePath = true
	}
	if s.sawFileNotFound && s.sawComponentCachePath {
		s.failed = true
		s.reason = "ESP-IDF 组件缓存路径过长或缓存文件损坏，工具已改为使用更短的组件缓存目录，请重新构建一次"
		return
	}

	switch {
	case strings.Contains(lower, "cmake failed with exit code"),
		strings.Contains(lower, "ninja failed with exit code"),
		strings.Contains(lower, "idf.py failed"),
		strings.Contains(lower, "command failed"):
		s.failed = true
		s.reason = line
	case strings.Contains(lower, "cmake error at") && s.reason == "":
		s.failed = true
		s.reason = line
	case strings.Contains(lower, "traceback") && s.reason == "":
		s.failed = true
		s.reason = line
	}
}

func (s *buildLogState) Err(processErr error) error {
	s.mu.Lock()
	failed := s.failed
	reason := s.reason
	s.mu.Unlock()

	if reason == "" {
		reason = "请查看高级日志中的 CMake/idf.py 输出"
	}
	if processErr != nil {
		return fmt.Errorf("ESP-IDF 构建失败：%s：%w", reason, processErr)
	}
	if failed {
		return errors.New("ESP-IDF 构建失败：" + reason)
	}
	return nil
}

package builder

import (
	"bufio"
	"context"
	"errors"
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
	pipe := func(scanner *bufio.Scanner) {
		defer wg.Done()
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			if log != nil {
				log(scanner.Text())
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
	return command.Wait()
}

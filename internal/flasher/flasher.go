package flasher

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/Textloding/skyloong-flasher/internal/packagekit"
	"github.com/Textloding/skyloong-flasher/internal/processutil"
	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

type LogFunc func(line string)

func BuildCommand(status runtimekit.Status, analysis *packagekit.Analysis, port string, baud int) (*exec.Cmd, error) {
	if analysis == nil || !analysis.CanFlash {
		return nil, errors.New("当前固件包不能直接刷机")
	}
	if !status.Available {
		return nil, errors.New("刷机运行时未准备")
	}
	if port == "" {
		return nil, errors.New("请选择刷机串口")
	}
	if baud <= 0 {
		baud = 460800
	}

	args := []string{"-p", port, "-b", strconv.Itoa(baud)}
	if analysis.Before != "" {
		args = append(args, "--before", analysis.Before)
	}
	if analysis.After != "" {
		args = append(args, "--after", analysis.After)
	}
	chip := analysis.Chip
	if chip == "" {
		chip = "esp32s3"
	}
	args = append(args, "--chip", chip, "write_flash")
	args = append(args, analysis.WriteFlashArgs...)
	for _, file := range analysis.FlashFiles {
		args = append(args, file.Offset, file.Path)
	}

	if status.Kind == runtimekit.KindEIM {
		cmdArgs := append([]string{"esptool.py"}, args...)
		return runtimekit.EIMRunCommand(status, cmdArgs...), nil
	}
	if status.Kind == runtimekit.KindPythonScript {
		cmdArgs := append([]string{status.ToolPath}, args...)
		return processutil.Command(status.PythonPath, cmdArgs...), nil
	}
	return processutil.Command(status.ToolPath, args...), nil
}

func Run(ctx context.Context, status runtimekit.Status, analysis *packagekit.Analysis, port string, baud int, log LogFunc) error {
	cmd, err := BuildCommand(status, analysis, port, baud)
	if err != nil {
		return err
	}
	command := processutil.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
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
		for scanner.Scan() {
			if log != nil {
				log(scanner.Text())
			}
		}
	}
	wg.Add(2)
	go pipe(bufio.NewScanner(stdout))
	go pipe(bufio.NewScanner(stderr))
	wg.Wait()
	return command.Wait()
}

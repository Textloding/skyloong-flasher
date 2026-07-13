package esptoolcmd

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/Textloding/skyloong-flasher/internal/processutil"
	"github.com/Textloding/skyloong-flasher/internal/runtimekit"
)

func Build(status runtimekit.Status, args ...string) (*exec.Cmd, error) {
	switch status.Kind {
	case runtimekit.KindExecutable:
		if status.ToolPath == "" {
			return nil, errors.New("esptool 可执行文件路径为空")
		}
		cmd := processutil.Command(status.ToolPath, args...)
		cmd.Env = runtimekit.CommandEnv(status)
		return cmd, nil
	case runtimekit.KindPythonScript:
		if status.PythonPath == "" || status.ToolPath == "" {
			return nil, errors.New("Python 或 esptool.py 路径为空")
		}
		cmdArgs := append([]string{status.ToolPath}, args...)
		cmd := processutil.Command(status.PythonPath, cmdArgs...)
		cmd.Env = runtimekit.CommandEnv(status)
		return cmd, nil
	case runtimekit.KindEIM:
		if status.EIMPath == "" {
			return nil, errors.New("EIM 运行时路径为空")
		}
		cmdArgs := append([]string{"esptool.py"}, args...)
		return runtimekit.EIMRunCommand(status, cmdArgs...), nil
	default:
		return nil, fmt.Errorf("不支持的 esptool 运行时类型：%s", status.Kind)
	}
}

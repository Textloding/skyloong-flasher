//go:build windows

package processutil

import (
	"context"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	HideWindow(cmd)
	return cmd
}

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	HideWindow(cmd)
	return cmd
}

func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

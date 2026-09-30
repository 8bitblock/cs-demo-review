//go:build windows

package service

import (
	"os/exec"
	"syscall"
)

func hideProcess(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

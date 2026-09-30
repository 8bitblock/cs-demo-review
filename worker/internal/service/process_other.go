//go:build !windows

package service

import "os/exec"

func hideProcess(c *exec.Cmd) {}

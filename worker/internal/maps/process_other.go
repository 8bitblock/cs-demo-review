//go:build !windows

package maps

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

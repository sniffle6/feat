package main

import (
	"fmt"
	"os/exec"
	"syscall"
)

// isPIDAlive checks if a process with the given PID is still running.
func isPIDAlive(pid string) bool {
	if pid == "" {
		return false
	}
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`cmd /C tasklist /FI "PID eq %s" /NH 2>nul | findstr /C:"%s" >nul`, pid, pid),
	}
	return cmd.Run() == nil
}

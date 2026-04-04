//go:build !windows

package main

import (
	"os"
	"strconv"
	"syscall"
)

// isPIDAlive checks if a process with the given PID is still running.
func isPIDAlive(pid string) bool {
	if pid == "" {
		return false
	}
	n, err := strconv.Atoi(pid)
	if err != nil {
		return false
	}
	p, err := os.FindProcess(n)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

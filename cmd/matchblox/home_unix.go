//go:build !windows

package main

import "syscall"

func execve(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env) //nolint:gosec // argv is panes.HomeArgv: tmux and this binary
}

//go:build !windows

package main

import "syscall"

// lowestPriority runs the console, and every command it starts, at nice 19.
func lowestPriority() { _ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 19) }

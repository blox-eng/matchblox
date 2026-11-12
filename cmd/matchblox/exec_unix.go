//go:build unix

package main

import "syscall"

// execFn runs a binary in place of this process: same pid, same terminal.
var execFn = syscall.Exec

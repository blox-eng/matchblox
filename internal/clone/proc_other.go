//go:build !unix

package clone

import "os/exec"

// ownGroup is a no-op where there are no process groups: a stop kills git
// only, and WaitDelay bounds the wait for its children.
func ownGroup(*exec.Cmd) {}

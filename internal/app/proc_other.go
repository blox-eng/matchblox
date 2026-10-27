//go:build !unix

package app

import "os/exec"

// ownSession is a no-op where there are no sessions.
func ownSession(*exec.Cmd) {}

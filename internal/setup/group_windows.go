package setup

import "os/exec"

func ownGroup(*exec.Cmd) {} // no user units on Windows

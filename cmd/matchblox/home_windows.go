//go:build windows

package main

import "errors"

func execve(string, []string, []string) error {
	return errors.New("the console cannot start tmux on Windows")
}

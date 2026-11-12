//go:build windows

package main

import "errors"

// execFn: Windows cannot run a binary in place of this process.
var execFn = func(string, []string, []string) error {
	return errors.New("this system cannot start a binary in place of a running one")
}

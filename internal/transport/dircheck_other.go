//go:build !unix

package transport

import "errors"

var ErrNotYours = errors.New("not yours")

func CheckDir(string) error { return nil }

func Mine(string) bool { return true }

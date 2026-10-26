//go:build !unix

package transport

func CheckDir(string) error { return nil }

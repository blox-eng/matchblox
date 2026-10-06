//go:build !unix

package transport

func checkDir(string) error { return nil }

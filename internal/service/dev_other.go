//go:build !unix || ios

package service

func devOf(string) (ttyDevice, bool) { return ttyDevice{}, false }

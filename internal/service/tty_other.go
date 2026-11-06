//go:build !linux && !(darwin && !ios)

package service

// ttyOf reads no terminal here: there is no tmux to hold one, and a sender
// the service cannot place is refused.
func ttyOf(int) (ttyDevice, bool) { return ttyDevice{}, false }

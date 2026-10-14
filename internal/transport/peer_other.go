//go:build !linux && !darwin

package transport

import "net"

// PeerPID is not known on this system: the act's own From stands.
func PeerPID(net.Conn) (int, bool) { return 0, false }

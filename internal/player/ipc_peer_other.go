//go:build !linux && !windows

package player

import "net"

func extractPeerPID(conn net.Conn) int {
	return 0
}

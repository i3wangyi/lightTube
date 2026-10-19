//go:build linux

package player

import (
	"net"
	"syscall"
)

func extractPeerPID(conn net.Conn) int {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0
	}
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return 0
	}

	var pid int
	_ = rawConn.Control(func(fd uintptr) {
		ucred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err == nil && ucred != nil {
			pid = int(ucred.Pid)
		}
	})
	return pid
}

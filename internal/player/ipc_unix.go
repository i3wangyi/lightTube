//go:build !windows

package player

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func defaultSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "lighttube-mpv.sock")
}

func ensureSocketDir(path string) error {
	if path == "" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

func (p *Player) stopIPC() error {
	if p.socketPath == "" {
		p.killCurrentProcess()
		return nil
	}

	if _, err := os.Stat(p.socketPath); os.IsNotExist(err) {
		p.killCurrentProcess()
		return nil
	}

	conn, err := net.DialTimeout("unix", p.socketPath, 500*time.Millisecond)
	if err != nil {
		// Clean up orphaned socket files from dead processes to allow rebinding.
		_ = os.Remove(p.socketPath)
		p.killCurrentProcess()
		return nil
	}
	defer conn.Close()

	peerPID := extractPeerPID(conn)

	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	_, writeErr := conn.Write([]byte("{\"command\": [\"quit\"]}\n"))

	buf := make([]byte, 512)
	_, readErr := conn.Read(buf)

	timedOut := false
	if writeErr != nil {
		timedOut = true
	} else if readErr != nil && !errors.Is(readErr, io.EOF) {
		var netErr net.Error
		if errors.As(readErr, &netErr) && netErr.Timeout() {
			timedOut = true
		}
	}

	if timedOut {
		// Kill the unresponsive player process when the socket does not answer within 500ms.
		if peerPID > 0 && peerPID != os.Getpid() {
			_ = syscall.Kill(peerPID, syscall.SIGKILL)
		}
		p.killCurrentProcess()
	} else if peerPID > 0 && peerPID != os.Getpid() {
		// Wait up to 1s for old mpv process to exit before returning,
		// so the old mpv cannot remove the new socket.
		waitForProcessExit(peerPID, 1*time.Second)
	}

	_ = os.Remove(p.socketPath)
	return nil
}

func waitForProcessExit(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isProcessExited(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func isProcessExited(pid int) bool {
	if pid <= 0 {
		return true
	}
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return true
	}
	// Kill(pid, 0) succeeds for a zombie, so on Linux read the state from
	// /proc. Other systems have no /proc: there the process still runs.
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return runtime.GOOS == "linux"
	}
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx >= 0 && idx+2 < len(s) {
		state := s[idx+2]
		if state == 'Z' || state == 'X' {
			return true
		}
	}
	return false
}

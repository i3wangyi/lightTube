//go:build !windows

package player

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlayerStopNoSocket(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "nonexistent.sock")
	p := NewWithOptions(sockPath, "", "")
	if err := p.Stop(); err != nil {
		t.Fatalf("expected nil error when socket does not exist, got: %v", err)
	}
}

// shortSocketDir returns a directory for Unix sockets with a short path:
// macOS limits socket paths to 104 bytes and its t.TempDir() is longer.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lt-sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestPlayerStopResponsiveSocket(t *testing.T) {
	dir := shortSocketDir(t)
	sockPath := filepath.Join(dir, "mock-mpv.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on mock socket: %v", err)
	}
	defer listener.Close()

	receivedQuit := make(chan bool, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err == nil && strings.Contains(line, "quit") {
			receivedQuit <- true
			_, _ = conn.Write([]byte("{\"error\": \"success\"}\n"))
		}
	}()

	p := NewWithOptions(sockPath, "", "")
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	select {
	case <-receivedQuit:
		// Succeeded in sending quit.
	case <-time.After(1 * time.Second):
		t.Fatal("mock server did not receive quit command in time")
	}

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("expected socket file to be removed after Stop(), but it still exists")
	}
}

func TestPlayerStopUnresponsiveSocket(t *testing.T) {
	dir := shortSocketDir(t)
	sockPath := filepath.Join(dir, "unresponsive-mpv.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on mock socket: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Intentionally sleep longer than the 500ms timeout without responding.
		time.Sleep(800 * time.Millisecond)
	}()

	p := NewWithOptions(sockPath, "", "")
	start := time.Now()
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop() failed on unresponsive socket: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 450*time.Millisecond {
		t.Errorf("expected timeout of at least 450ms, took %v", elapsed)
	}
}

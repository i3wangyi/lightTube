//go:build !windows

package player

import (
	"bufio"
	"net"
	"os"
	"os/exec"
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

func TestPlayerStopResponsiveSocket(t *testing.T) {
	dir := t.TempDir()
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
	dir := t.TempDir()
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

func TestHelperProcessExit(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	sockPath := os.Getenv("HELPER_SOCK_PATH")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		os.Exit(1)
	}
	defer listener.Close()

	conn, err := listener.Accept()
	if err != nil {
		os.Exit(1)
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err == nil && strings.Contains(line, "quit") {
		_, _ = conn.Write([]byte("{\"error\": \"success\"}\n"))
	}
	_ = conn.Close()
	_ = listener.Close()
	// Sleep 150ms before exiting to simulate mpv cleanup.
	time.Sleep(150 * time.Millisecond)
	os.Exit(0)
}

func TestPlayerStopWaitsForPeerProcessExit(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "peer-exit.sock")

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessExit")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_SOCK_PATH="+sockPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// Wait until socket file is created and listening.
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	p := NewWithOptions(sockPath, "", "")
	start := time.Now()
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("expected Stop() to wait at least 100ms for peer process to exit, took %v", elapsed)
	}

	if !isProcessExited(cmd.Process.Pid) {
		t.Errorf("expected process %d to be exited, but it was still running", cmd.Process.Pid)
	}
}

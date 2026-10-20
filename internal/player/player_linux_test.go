//go:build linux

package player

// Stop waits for the mpv process behind the socket only on Linux: the peer
// PID comes from SO_PEERCRED and zombie detection reads /proc. lightTube on
// macOS uses an external mpv window instead.

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
	dir := shortSocketDir(t)
	sockPath := filepath.Join(dir, "peer-exit.sock")

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessExit")
	// Under -race the helper (this test binary) sleeps 1 s at exit by
	// default, which equals Stop's wait; turn that sleep off.
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_SOCK_PATH="+sockPath, "GORACE=atexit_sleep_ms=0")
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

func TestBuildArgsReadsSysfsBattery(t *testing.T) {
	dir := t.TempDir()
	adp := filepath.Join(dir, "ADP1")
	if err := os.MkdirAll(adp, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(adp, "type"), []byte("Mains\n"), 0o644)
	_ = os.WriteFile(filepath.Join(adp, "online"), []byte("0\n"), 0o644)
	args := NewWithOptions("/tmp/lt-test.sock", dir, "app").BuildArgs(Options{URL: "https://youtube.com/watch?v=123"})
	assertContains(t, args, "--ytdl-format="+Format(PowerBattery, ModeVideo))
}

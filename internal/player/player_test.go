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

func TestPlayerBuildArgs(t *testing.T) {
	sockPath := "/tmp/test-lighttube.sock"
	appID := "test-app-id"

	tests := []struct {
		name       string
		setupPower func(dir string)
		opts       Options
		check      func(t *testing.T, args []string)
	}{
		{
			name: "video AC arguments",
			opts: Options{
				URL:   "https://youtube.com/watch?v=123",
				Title: "Test Video",
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--hwdec=vaapi")
				assertContains(t, args, "--vo=gpu")
				assertContains(t, args, "--force-window=immediate")
				assertContains(t, args, "--title=Test Video")
				assertContains(t, args, "--wayland-app-id=test-app-id")
				assertContains(t, args, "--input-ipc-server=/tmp/test-lighttube.sock")
				assertContains(t, args, "--ytdl-format="+formatAC)
				if args[len(args)-1] != "https://youtube.com/watch?v=123" {
					t.Errorf("last argument should be URL, got %q", args[len(args)-1])
				}
			},
		},
		{
			name: "video battery arguments",
			setupPower: func(dir string) {
				adp := filepath.Join(dir, "ADP1")
				_ = os.MkdirAll(adp, 0o755)
				_ = os.WriteFile(filepath.Join(adp, "type"), []byte("Mains\n"), 0o644)
				_ = os.WriteFile(filepath.Join(adp, "online"), []byte("0\n"), 0o644)
			},
			opts: Options{
				URL: "https://youtube.com/watch?v=123",
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--ytdl-format="+formatBattery)
				assertContains(t, args, "--force-window=immediate")
				// Title omitted when empty.
				for _, arg := range args {
					if strings.HasPrefix(arg, "--title=") {
						t.Errorf("did not expect title argument, got %q", arg)
					}
				}
			},
		},
		{
			name: "audio mode arguments",
			opts: Options{
				URL:   "https://youtube.com/watch?v=123",
				Audio: true,
			},
			check: func(t *testing.T, args []string) {
				assertContains(t, args, "--ytdl-format=ba")
				assertContains(t, args, "--no-video")
				assertContains(t, args, "--force-window=no")
				for _, arg := range args {
					if arg == "--force-window=immediate" {
						t.Errorf("audio mode must not include --force-window=immediate")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysfsDir := t.TempDir()
			if tt.setupPower != nil {
				tt.setupPower(sysfsDir)
			}
			player := NewWithOptions(sockPath, sysfsDir, appID)
			args := player.BuildArgs(tt.opts)
			tt.check(t, args)
		})
	}
}

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

func assertContains(t *testing.T, slice []string, item string) {
	t.Helper()
	for _, s := range slice {
		if s == item {
			return
		}
	}
	t.Errorf("expected slice to contain %q, but got %v", item, slice)
}

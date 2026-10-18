package player

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// DefaultAppID is the fixed Wayland app id for the mpv window.
// Hyprland rules match this id to manage the video window.
const DefaultAppID = "lighttube-mpv"

// DefaultSocketPath returns the standard IPC socket location under XDG_RUNTIME_DIR.
func DefaultSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "lighttube-mpv.sock")
}

// Options configures playback parameters.
type Options struct {
	URL    string
	Title  string
	Audio  bool
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Player controls mpv playback and enforces single-instance execution via IPC.
type Player struct {
	socketPath string
	sysfsPath  string
	appID      string

	mu         sync.Mutex
	currentCmd *exec.Cmd
}

// New creates a Player using standard system defaults.
func New() *Player {
	return NewWithOptions(DefaultSocketPath(), defaultSysfsPowerSupply, DefaultAppID)
}

// NewWithOptions allows custom paths for testing and non-default environments.
func NewWithOptions(socketPath, sysfsPath, appID string) *Player {
	return &Player{
		socketPath: socketPath,
		sysfsPath:  sysfsPath,
		appID:      appID,
	}
}

// BuildArgs produces the command-line argument slice for mpv based on the active power source and mode.
func (p *Player) BuildArgs(opts Options) []string {
	mode := ModeVideo
	if opts.Audio {
		mode = ModeAudio
	}
	power := detectPowerFrom(p.sysfsPath)
	formatStr := Format(power, mode)

	args := []string{
		"--hwdec=vaapi",
		"--vo=gpu",
		"--ytdl-format=" + formatStr,
		"--save-position-on-quit",
	}

	if opts.Audio {
		// Audio-only playback suppresses video window allocation to reduce memory and CPU overhead.
		args = append(args, "--no-video", "--force-window=no")
	} else {
		// Immediate window creation prevents UI flicker during stream negotiation.
		args = append(args, "--force-window=immediate")
	}

	if opts.Title != "" {
		args = append(args, "--title="+opts.Title)
	}

	if p.socketPath != "" {
		args = append(args, "--input-ipc-server="+p.socketPath)
	}

	if p.appID != "" {
		args = append(args, "--wayland-app-id="+p.appID)
	}

	args = append(args, opts.URL)
	return args
}

// Stop terminates any running mpv instance.
// It sends quit over the IPC socket; if unresponsive after 500ms, it terminates the process directly.
func (p *Player) Stop() error {
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
	}

	_ = os.Remove(p.socketPath)
	return nil
}

func (p *Player) killCurrentProcess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.currentCmd != nil && p.currentCmd.Process != nil {
		_ = p.currentCmd.Process.Kill()
		_ = p.currentCmd.Wait()
		p.currentCmd = nil
	}
}

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

// Start stops any running instance and spawns a new mpv process asynchronously.
func (p *Player) Start(opts Options) (*exec.Cmd, error) {
	if opts.URL == "" {
		return nil, errors.New("missing media URL")
	}

	if err := p.Stop(); err != nil {
		return nil, err
	}

	if p.socketPath != "" {
		// Ensure socket directory exists before mpv attempts to bind.
		if err := os.MkdirAll(filepath.Dir(p.socketPath), 0o700); err != nil {
			return nil, err
		}
	}

	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		return nil, fmt.Errorf("mpv executable not found in PATH: %w", err)
	}

	args := p.BuildArgs(opts)
	cmd := exec.Command(mpvPath, args...)
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.currentCmd = cmd
	p.mu.Unlock()

	return cmd, nil
}

// Play launches playback and blocks until the player process exits.
func (p *Player) Play(opts Options) error {
	cmd, err := p.Start(opts)
	if err != nil {
		return err
	}

	err = cmd.Wait()

	p.mu.Lock()
	if p.currentCmd == cmd {
		p.currentCmd = nil
	}
	p.mu.Unlock()

	return err
}

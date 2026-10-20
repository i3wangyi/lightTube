package player

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"lighttube/internal/embed"
)

// DefaultAppID is the fixed Wayland app id for the mpv window.
// Hyprland rules match this id to manage the video window.
const DefaultAppID = "lighttube-mpv"

// DefaultSocketPath returns the standard IPC socket location for the active OS.
func DefaultSocketPath() string {
	return defaultSocketPath()
}

// Options configures playback parameters.
type Options struct {
	URL       string
	Title     string
	Audio     bool
	Policy    CodecPolicy
	Caps      embed.Caps
	ExtraArgs []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
}

// Player controls mpv playback and enforces single-instance execution via IPC.
type Player struct {
	socketPath string
	sysfsPath  string
	// powerSource overrides power detection; tests set it because only
	// Linux reads the sysfs directory.
	powerSource func() Power
	appID       string

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
	if p.powerSource != nil {
		power = p.powerSource()
	}
	policy := opts.Policy
	if policy == "" {
		policy = CodecPolicyOldGPU
	}
	formatStr := FormatFor(power, mode, policy, opts.Caps)

	args := []string{
		"--hwdec=" + HWDec(policy, opts.Caps),
		"--demuxer-max-bytes=64MiB",
		"--demuxer-max-back-bytes=16MiB",
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

	if len(opts.ExtraArgs) > 0 {
		args = append(args, opts.ExtraArgs...)
	}

	args = append(args, opts.URL)
	return args
}

// Stop terminates any running mpv instance.
func (p *Player) Stop() error {
	return p.stopIPC()
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

// Start stops any running instance and spawns a new mpv process asynchronously.
func (p *Player) Start(opts Options) (*exec.Cmd, error) {
	if opts.URL == "" {
		return nil, errors.New("missing media URL")
	}

	if err := p.Stop(); err != nil {
		return nil, err
	}

	if p.socketPath != "" {
		if err := ensureSocketDir(p.socketPath); err != nil {
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

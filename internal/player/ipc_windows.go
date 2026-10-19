//go:build windows

package player

import (
	"os"
	"time"
)

func defaultSocketPath() string {
	return `\\.\pipe\lighttube-mpv`
}

func ensureSocketDir(path string) error {
	return nil
}

func (p *Player) stopIPC() error {
	if p.socketPath == "" {
		p.killCurrentProcess()
		return nil
	}

	f, err := os.OpenFile(p.socketPath, os.O_RDWR, 0)
	if err != nil {
		p.killCurrentProcess()
		return nil
	}

	_, _ = f.Write([]byte("{\"command\": [\"quit\"]}\n"))

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 512)
		_, _ = f.Read(buf)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}

	_ = f.Close()
	p.killCurrentProcess()
	return nil
}

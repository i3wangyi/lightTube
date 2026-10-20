package ui

import (
	"runtime/debug"
	"sync"
	"time"
)

// Reclaim startup allocations once, after feed loading and a quiet frame interval.
// The timer does not request frames or run on the UI goroutine.
type startupMemory struct {
	mu          sync.Mutex
	timer       *time.Timer
	lastFrame   time.Time
	ready, done bool
}

func (m *startupMemory) frame(ready bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done {
		return
	}
	m.ready = ready
	m.lastFrame = time.Now()
	if !ready {
		if m.timer != nil {
			m.timer.Stop()
		}
		return
	}
	if m.timer == nil {
		m.timer = time.AfterFunc(3*time.Second, m.release)
	} else {
		m.timer.Reset(3 * time.Second)
	}
}

func (m *startupMemory) release() {
	m.mu.Lock()
	if m.done || !m.ready {
		m.mu.Unlock()
		return
	}
	if remaining := 3*time.Second - time.Since(m.lastFrame); remaining > 0 {
		m.timer.Reset(remaining)
		m.mu.Unlock()
		return
	}
	m.done = true
	m.mu.Unlock()
	debug.FreeOSMemory()
}

func (ui *UI) startupMemoryReady() bool {
	ui.refreshMu.Lock()
	feedsPending := len(ui.refreshingFeeds) != 0
	ui.refreshMu.Unlock()
	return ui.startupStoreLoaded && !feedsPending && !ui.startupFeedPending &&
		!ui.shortsLoading && !ui.podcastsLoading && !ui.musicLoading && !ui.playlistsLoading && len(ui.demoLoading) == 0
}

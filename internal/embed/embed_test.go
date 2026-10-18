package embed

import (
	"image"
	"testing"
)

func TestNewNilPointers(t *testing.T) {
	p, err := New(nil, nil)
	if err == nil {
		t.Fatalf("expected error with nil pointers, got nil")
	}
	if p != nil {
		t.Fatalf("expected nil player with nil pointers, got %v", p)
	}
}

func TestPlayerClosedOperations(t *testing.T) {
	var p Player
	// Ensure operations on an uninitialized player do not panic.
	p.SetRect(image.Rect(0, 0, 100, 100), 2)
	p.SetHidden(true)
	p.TogglePause()
	p.SetPause(true)
	p.Seek(10, true)
	p.SetVolume(50)
	p.ToggleMute()
	p.Stop()
	p.Close()

	st := p.Status()
	if st.IsPlaying {
		t.Errorf("expected is_playing false on uninitialized player")
	}

	err := p.Play(PlayOptions{URL: "https://example.com"})
	if err == nil {
		t.Errorf("expected error playing on uninitialized player")
	}
}

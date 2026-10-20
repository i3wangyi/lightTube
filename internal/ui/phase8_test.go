package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"lighttube/internal/store"
	"lighttube/internal/thumbs"
	"lighttube/internal/yt"
)

func createTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

// TestThumbnailGridPrefetchNeverExceedsHalfBudget tests that a grid of 4 columns
// with thumbnail prefetching never requests more thumbnails than fit in half the LRU budget.
func TestThumbnailGridPrefetchNeverExceedsHalfBudget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	jpegData := createTestJPEG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegData)
	}))
	defer srv.Close()

	thMgr := thumbs.NewWithOptions(srv.Client(), dir+"/thumbs", thumbs.DefaultLRUByteBudget)

	ui := New(Options{
		Store:   st,
		Thumbs:  thMgr,
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	// Populate 100 videos to ensure grid has many rows.
	videos := make([]yt.Video, 100)
	for i := 0; i < 100; i++ {
		videos[i] = yt.Video{
			ID:      fmt.Sprintf("vid_%04d", i),
			Title:   fmt.Sprintf("Video %d", i),
			Channel: "Test Channel",
		}
	}
	ui.state.SetVideos(videos)

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1600, 1000)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Now(),
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	frame()

	requestedCount := thMgr.LoadCount()
	if requestedCount > int64(thumbs.MaxThumbnailsInHalfBudget) {
		t.Fatalf("grid requested %d thumbnails, exceeds half budget clamp of %d",
			requestedCount, thumbs.MaxThumbnailsInHalfBudget)
	}

	// Verify that the requested byte capacity does not exceed half the budget.
	maxBytesRequested := int(requestedCount) * thumbs.MaxThumbnailBytes
	halfBudget := thumbs.DefaultLRUByteBudget / 2
	if maxBytesRequested > halfBudget {
		t.Fatalf("requested bytes %d exceeds half budget %d", maxBytesRequested, halfBudget)
	}
}

// TestSteadyFrameLoopEvictionDoesNotCauseReRequests tests that eviction does not cause
// re-requests in a steady frame loop (count Load calls over 10 frames: must be 0).
func TestSteadyFrameLoopEvictionDoesNotCauseReRequests(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	jpegData := createTestJPEG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegData)
	}))
	defer srv.Close()

	thMgr := thumbs.NewWithOptions(srv.Client(), dir+"/thumbs", thumbs.DefaultLRUByteBudget)

	ui := New(Options{
		Store:   st,
		Thumbs:  thMgr,
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	videos := make([]yt.Video, 50)
	for i := 0; i < 50; i++ {
		videos[i] = yt.Video{
			ID:      fmt.Sprintf("steady_vid_%03d", i),
			Title:   fmt.Sprintf("Video %d", i),
			Channel: "Channel",
		}
	}
	ui.state.SetVideos(videos)

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1600, 1000)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Now(),
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	// First frame triggers initial thumbnail loads.
	frame()

	// Wait for background thumbnail fetches to complete and update LRU.
	deadline := time.Now().Add(5 * time.Second)
	for thMgr.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	// Drain any pending actions from completed thumbnail loads.
	ui.drainActions()
	frame()

	// Capture baseline Load calls once all initial loads are finished.
	baselineLoads := thMgr.LoadCount()

	// Run 10 consecutive frames in steady state.
	for i := 0; i < 10; i++ {
		frame()
		ui.drainActions()
	}

	subsequentLoads := thMgr.LoadCount() - baselineLoads
	if subsequentLoads != 0 {
		t.Fatalf("expected 0 Load calls over 10 steady frames, got %d", subsequentLoads)
	}
}

// TestHoverPreviewFrameTimerHeadless verifies that no InvalidateCmd is issued when
// nothing is hovered, and that the timer starts when a card is hovered.
func TestHoverPreviewFrameTimerHeadless(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}

	ui := New(Options{
		Store:   st,
		Account: yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	videos := []yt.Video{
		{ID: "hover_vid_001", Title: "Hover Test Video", Channel: "Channel"},
		{ID: "hover_vid_002", Title: "Second Video", Channel: "Channel"},
	}
	ui.state.SetVideos(videos)

	var r input.Router
	var ops op.Ops
	frame := func(now time.Time) {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1600, 1000)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         now,
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	now := time.Now()

	// 1. Initial frame without pointer events: nothing is hovered.
	frame(now)

	// Verify that no InvalidateCmd was issued when nothing is hovered.
	if wakeup, ok := r.WakeupTime(); ok {
		t.Fatalf("expected no InvalidateCmd when nothing is hovered, got wakeup at %v", wakeup)
	}

	// 2. Move pointer to hover the thumbnail (at x=350, y=250 in the 216dp rail shell).
	r.Queue(pointer.Event{
		Kind:     pointer.Move,
		Position: f32.Pt(350, 250),
	})
	now = now.Add(50 * time.Millisecond)
	frame(now)

	// Since the card is hovered and elapsed < 500ms, an InvalidateCmd MUST be issued for hover start + 500ms.
	wakeup, ok := r.WakeupTime()
	if !ok {
		t.Fatalf("expected InvalidateCmd when card is hovered, got none")
	}
	if wakeup.Before(now) {
		t.Fatalf("expected wakeup in the future, got %v (now is %v)", wakeup, now)
	}

	// 3. Move pointer away from cards (e.g. back to top-left margin).
	r.Queue(pointer.Event{
		Kind:     pointer.Move,
		Position: f32.Pt(5, 5),
	})
	now = now.Add(100 * time.Millisecond)
	frame(now)

	// Hover is cleared; no InvalidateCmd should be issued.
	if wakeup, ok := r.WakeupTime(); ok {
		t.Fatalf("expected no InvalidateCmd after hover was cleared, got wakeup at %v", wakeup)
	}
}

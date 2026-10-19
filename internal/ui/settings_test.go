package ui

import (
	"image"
	"net/http"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

func TestSettingsChipsHeadlessRouter(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)

	st, err := store.NewWithDir(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ui := New(Options{
		YTClient: yt.NewClientWithOptions(fakeYTDLP{}, http.DefaultClient, dir),
		Store:    st,
		Account:  yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/cookies.txt", CacheDir: dir}),
	})

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1600, 1000)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	// Switch to You / Settings tab
	ui.state.CurrentTab = TabAccount
	frame()

	// Initial setting must be default "auto"
	if got := st.Settings().CodecPolicy; got != "auto" {
		t.Fatalf("initial codec policy = %q, want 'auto'", got)
	}

	// 1. Click Old GPU (H.264) chip
	ui.codecOldGPUChip.Click()
	frame()

	if got := ui.settings.CodecPolicy; got != "old-gpu" {
		t.Errorf("ui.settings.CodecPolicy after Old GPU click = %q, want 'old-gpu'", got)
	}
	if got := st.Settings().CodecPolicy; got != "old-gpu" {
		t.Errorf("st.Settings().CodecPolicy after Old GPU click = %q, want 'old-gpu'", got)
	}
	diskSettings, err := store.LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if diskSettings.CodecPolicy != "old-gpu" {
		t.Errorf("diskSettings.CodecPolicy = %q, want 'old-gpu'", diskSettings.CodecPolicy)
	}

	// 2. Click Any chip
	ui.codecAnyChip.Click()
	frame()

	if got := ui.settings.CodecPolicy; got != "any" {
		t.Errorf("ui.settings.CodecPolicy after Any click = %q, want 'any'", got)
	}
	if got := st.Settings().CodecPolicy; got != "any" {
		t.Errorf("st.Settings().CodecPolicy after Any click = %q, want 'any'", got)
	}
	diskSettings, _ = store.LoadSettings(dir)
	if diskSettings.CodecPolicy != "any" {
		t.Errorf("diskSettings.CodecPolicy = %q, want 'any'", diskSettings.CodecPolicy)
	}

	// 3. Click Player chip: External mpv
	ui.playerExternalChip.Click()
	frame()

	if got := st.Settings().Player; got != "external" {
		t.Errorf("st.Settings().Player after external click = %q, want 'external'", got)
	}
	if !ui.externalPlayer {
		t.Errorf("ui.externalPlayer should be true after External mpv click")
	}

	// 4. Click Player chip: In window
	ui.playerInplaceChip.Click()
	frame()

	if got := st.Settings().Player; got != "inplace" {
		t.Errorf("st.Settings().Player after inplace click = %q, want 'inplace'", got)
	}
	if ui.externalPlayer {
		t.Errorf("ui.externalPlayer should be false after In window click")
	}
}

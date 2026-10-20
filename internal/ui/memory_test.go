package ui

import (
	"fmt"
	"lighttube/internal/yt"
	"os"
	"strings"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

func TestLazyFontsLatinAndCJK(t *testing.T) {
	th, _, fonts := newLazyDarkTheme()
	if len(fonts.fallback) == 0 {
		t.Skip("no platform collection font")
	}
	ui := &UI{th: th, fonts: fonts}
	latin := th.Shaper
	ui.label(unit.Sp(16), "lightTube · Watch — Something for right now…")
	if th.Shaper != latin || len(fonts.fallback) == 0 {
		t.Fatal("Latin text loaded fallback fonts")
	}
	want, _ := NewDarkTheme()
	for _, sample := range []string{"資深工程師", "日本語", "한국어", "Hello 中文"} {
		label := ui.label(unit.Sp(16), sample)
		params := text.Parameters{Font: font.Font{}, PxPerEm: 16 << 6, MaxWidth: 1000}
		label.Shaper.LayoutString(params, sample)
		want.Shaper.LayoutString(params, sample)
		for {
			got, gok := label.Shaper.NextGlyph()
			expected, wok := want.Shaper.NextGlyph()
			if gok != wok || got != expected {
				t.Fatalf("%q glyph mismatch: got %+v (%t), want %+v (%t)", sample, got, gok, expected, wok)
			}
			if !gok {
				break
			}
		}
	}
	if th.Shaper == latin || len(fonts.fallback) != 0 {
		t.Fatal("CJK text did not register fallback fonts")
	}
	loaded := th.Shaper
	ui.label(unit.Sp(16), "更多中文")
	if th.Shaper != loaded {
		t.Fatal("fallback reloaded")
	}
}

func TestLazyFontsEditorAndButton(t *testing.T) {
	for _, kind := range []string{"editor", "button"} {
		t.Run(kind, func(t *testing.T) {
			th, _, fonts := newLazyDarkTheme()
			if len(fonts.fallback) == 0 {
				t.Skip("no platform collection font")
			}
			ui := &UI{th: th, fonts: fonts}
			previous := th.Shaper
			if kind == "editor" {
				var editor widget.Editor
				editor.SetText("中文検索")
				ui.editor(&editor, "Search")
			} else {
				ui.button(&widget.Clickable{}, "中文")
			}
			if th.Shaper == previous {
				t.Fatal("CJK control text did not load fallback fonts")
			}
		})
	}
}

func TestStreamingFontMissingFile(t *testing.T) {
	if faces := loadFontFile(t.TempDir() + "/missing.ttf"); len(faces) != 0 {
		t.Fatal("missing font returned faces")
	}
	path := t.TempDir() + "/invalid.ttf"
	if err := os.WriteFile(path, []byte("invalid font"), 0600); err != nil {
		t.Fatal(err)
	}
	if faces := loadFontFile(path); len(faces) != 0 {
		t.Fatal("invalid font returned faces")
	}
}

func TestWatchGridOnlyShapesVisibleRows(t *testing.T) {
	ui := reviewUI(t)
	videos := make([]yt.Video, 120)
	for i := range videos {
		videos[i] = yt.Video{ID: fmt.Sprint(i), Title: fmt.Sprintf("Visible row item %03d", i), Channel: "Creator"}
	}
	videos[len(videos)-1].Title = "最後の行"
	ui.setWatchVideos(videos)
	seen := make(map[string]bool)
	ui.labelHook = func(value string) { seen[value] = true }
	reviewFrame(ui)
	count := 0
	for value := range seen {
		if strings.HasPrefix(value, "Visible row item") {
			count++
		}
	}
	if count == 0 || count > 12 || seen["最後の行"] {
		t.Fatalf("offscreen rows were shaped: %d titles, last=%t", count, seen["最後の行"])
	}
	ui.spaces.Active().ScrollList.ScrollTo(61)
	reviewFrame(ui)
	if !seen["最後の行"] {
		t.Fatal("last row is not reachable by scrolling")
	}
}

func TestDebugSequenceShowsAllSpaces(t *testing.T) {
	ui := reviewUI(t)
	ui.firstFramePresented = true
	ui.debugSequence = newDebugSequence("watch:show,shorts:show,podcasts:show,music:show")
	ui.debugSequence.ready = true
	for i, want := range []SpaceID{SpaceWatch, SpaceShorts, SpacePodcasts, SpaceMusic} {
		ui.spaces.State(want).ContentLoaded = true
		if step := ui.advanceDebugSequence(time.Now()); step != ui.debugSequence.steps[i] {
			t.Fatalf("step failed: %s", step)
		}
		reviewFrame(ui)
		if ui.spaces.ActiveSpace != want {
			t.Fatalf("active=%v, want %v", ui.spaces.ActiveSpace, want)
		}
	}
}

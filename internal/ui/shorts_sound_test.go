package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"lighttube/internal/yt"
)

func TestShortsSoundLabelsFollowPlayer(t *testing.T) {
	ui := reviewUI(t)
	player := &sequencePlayer{}
	ui.embedPlayer = player
	ui.spaces.ActiveSpace = SpaceShorts
	state := ui.spaces.Active()
	state.ContentLoaded = true
	state.Shorts = []yt.Video{{ID: "short_first", Title: "First"}, {ID: "short_next", Title: "Next"}}
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		ui.layout(layout.Context{Ops: &ops, Source: router.Source(), Constraints: layout.Exact(image.Pt(1200, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now()})
		router.Frame(&ops)
	}
	find := func(label string) image.Rectangle {
		t.Helper()
		var found image.Rectangle
		var scan func([]input.SemanticNode)
		scan = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Label == label {
					found = node.Desc.Bounds
				}
				scan(node.Children)
			}
		}
		scan(router.AppendSemantics(nil))
		if found.Empty() {
			t.Fatalf("missing visible label %q; tree=%+v", label, router.AppendSemantics(nil))
		}
		return found
	}
	click := func(label string) {
		bounds := find(label)
		pos := f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		frame()
		frame()
	}
	check := func(muted bool) {
		t.Helper()
		frame()
		if player.Status().Muted != muted {
			t.Fatalf("mute=%t, want %t", player.Status().Muted, muted)
		}
		if muted {
			find("Sound off")
			find("Turn sound on")
		} else {
			find("Sound on")
			find("Turn sound off")
		}
	}
	ui.selectShort(state.Shorts[0], state.Shorts)
	check(true)
	click("Turn sound on")
	check(false)
	click("Next Short")
	check(false)
	if ui.playerOwnerID != "short_next" {
		t.Fatalf("Next did not load second Short: %s", ui.playerOwnerID)
	}
	click("Turn sound off")
	check(true)
}

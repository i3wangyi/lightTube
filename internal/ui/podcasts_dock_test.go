package ui

import (
	"fmt"
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"lighttube/internal/yt"
)

func TestPodcastDockLongTimeKeepsBarAndFullText(t *testing.T) {
	const tail = "3:50:19 / 3:50:19"
	for _, scale := range []int{1, 2} {
		for _, width := range []int{360, 599, 1000} {
			t.Run(fmt.Sprintf("%d/scale%d", width, scale), func(t *testing.T) {
				ui := reviewUI(t)
				ui.spaces.ActiveSpace = SpacePodcasts
				state := ui.spaces.Active()
				state.ContentLoaded = true
				state.ResumeItem = &yt.Video{ID: "long_episode", Title: "A long episode title which must yield space to the time and progress bar", Duration: 13819}
				ui.playerOwnerSpace, ui.playerOwnerID = SpacePodcasts, state.ResumeItem.ID
				ui.watchCurrentTime = 13819
				ui.embedPlayer = &sequencePlayer{position: 13819}
				var router input.Router
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Constraints: layout.Exact(image.Pt(width*scale, 600*scale)), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}, Now: time.Now()}
				_, natural := ui.recordPodcastTime(gtx, tail)
				ui.layout(gtx)
				router.Frame(&ops)
				var timeBounds, barBounds image.Rectangle
				for _, node := range router.AppendSemantics(nil) {
					switch node.Desc.Label {
					case tail:
						timeBounds = node.Desc.Bounds
					case "Podcast progress":
						barBounds = node.Desc.Bounds
					}
				}
				if timeBounds.Empty() || timeBounds.Dx() < natural.Size.X {
					t.Fatalf("time clipped: bounds=%v natural=%v", timeBounds, natural.Size)
				}
				if !timeBounds.In(image.Rect(0, 0, width*scale, 600*scale)) {
					t.Fatalf("time outside window: %v", timeBounds)
				}
				if barBounds.Dx() < 80*scale {
					t.Fatalf("bar=%v, minimum=%d", barBounds, 80*scale)
				}
			})
		}
	}
}

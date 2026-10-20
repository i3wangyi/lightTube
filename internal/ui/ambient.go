package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// AmbientShape draws fixed, layered rounded rectangles to produce a subtle glow
// behind featured content without per-frame image blurring or GPU re-uploads.
type AmbientShape struct {
	lastW, lastH int
	ops          op.Ops
	cachedOp     op.CallOp
	recorded     bool
	drawCount    int // for test verification
}

// DrawWatchGlow paints the static ambient glow for Watch space if ambient is enabled.
func (a *AmbientShape) DrawWatchGlow(gtx layout.Context, bounds image.Rectangle, enabled bool) {
	if !enabled {
		return
	}
	a.draw(gtx, bounds, DarkColors.AmbientWatch)
}

// DrawMusicGlow paints the static ambient glow for Music space if ambient is enabled.
func (a *AmbientShape) DrawMusicGlow(gtx layout.Context, bounds image.Rectangle, enabled bool) {
	if !enabled {
		return
	}
	a.draw(gtx, bounds, DarkColors.AmbientMusic)
}

func (a *AmbientShape) draw(gtx layout.Context, bounds image.Rectangle, baseCol color.NRGBA) {
	w := bounds.Dx()
	h := bounds.Dy()
	if w <= 0 || h <= 0 {
		return
	}

	a.drawCount++

	// Re-record only when dimensions change
	if !a.recorded || a.lastW != w || a.lastH != h {
		a.ops.Reset()
		m := op.Record(&a.ops)

		// Centered diffuse glow behind the card.
		// Uses 24 concentric layers with quadratic alpha falloff (1 - t)^2
		// so alpha decreases smoothly to 0 at the outermost edge.
		// Corner radius expands proportionally so the outer shape softens without straight rectangle lines.
		const numLayers = 24
		const maxExpDp = 22
		cardRad := gtx.Dp(unit.Dp(14))

		for i := numLayers; i >= 1; i-- {
			tOuter := float32(i) / float32(numLayers)
			tInner := float32(i-1) / float32(numLayers)

			// Quadratic falloff: accumulated alpha at t is baseCol.A * (1 - t)^2
			aOuter := float32(baseCol.A) * (1.0 - tOuter) * (1.0 - tOuter)
			aInner := float32(baseCol.A) * (1.0 - tInner) * (1.0 - tInner)
			layerAlpha := uint8(aInner - aOuter)
			if layerAlpha == 0 {
				layerAlpha = 1
			}

			expDp := float32(maxExpDp) * tOuter
			exp := gtx.Dp(unit.Dp(expDp))
			rad := cardRad + exp + exp/2

			rect := image.Rect(
				bounds.Min.X-exp,
				bounds.Min.Y-exp,
				bounds.Max.X+exp,
				bounds.Max.Y+exp,
			)
			col := color.NRGBA{
				R: baseCol.R,
				G: baseCol.G,
				B: baseCol.B,
				A: layerAlpha,
			}
			rrect := clip.UniformRRect(rect, rad)
			paint.FillShape(&a.ops, col, rrect.Op(&a.ops))
		}

		a.cachedOp = m.Stop()
		a.recorded = true
		a.lastW = w
		a.lastH = h
	}

	a.cachedOp.Add(gtx.Ops)
}

// DrawCount returns the number of times ambient shapes were painted.
func (a *AmbientShape) DrawCount() int {
	return a.drawCount
}

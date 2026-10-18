package ui

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// channelAvatarColor computes a deterministic, vibrant background color from a channel ID.
func channelAvatarColor(channelID string) color.NRGBA {
	palette := []color.NRGBA{
		{R: 0xe5, G: 0x39, B: 0x35, A: 0xff}, // Red
		{R: 0xd8, G: 0x1b, B: 0x60, A: 0xff}, // Pink
		{R: 0x8e, G: 0x24, B: 0xaa, A: 0xff}, // Purple
		{R: 0x5e, G: 0x35, B: 0xb1, A: 0xff}, // Deep Purple
		{R: 0x39, G: 0x49, B: 0xab, A: 0xff}, // Indigo
		{R: 0x1e, G: 0x88, B: 0xe5, A: 0xff}, // Blue
		{R: 0x03, G: 0x9b, B: 0xe5, A: 0xff}, // Light Blue
		{R: 0x00, G: 0x89, B: 0x7b, A: 0xff}, // Teal
		{R: 0x43, G: 0xa0, B: 0x47, A: 0xff}, // Green
		{R: 0x7c, G: 0xb3, B: 0x42, A: 0xff}, // Light Green
		{R: 0xfb, G: 0x8c, B: 0x00, A: 0xff}, // Orange
		{R: 0xf4, G: 0x51, B: 0x1e, A: 0xff}, // Deep Orange
	}
	var h uint32 = 2166136261
	for i := 0; i < len(channelID); i++ {
		h ^= uint32(channelID[i])
		h *= 16777619
	}
	return palette[h%uint32(len(palette))]
}

// channelInitial returns the uppercase first rune of the channel title.
func channelInitial(title string) string {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "?"
	}
	r, _ := utf8.DecodeRuneInString(trimmed)
	return strings.ToUpper(string(r))
}

// layoutAvatar draws a solid circle with the channel's initial letter in white.
func layoutAvatar(gtx layout.Context, th *material.Theme, channelID, channelTitle string, sizeDp int) layout.Dimensions {
	sizePx := gtx.Dp(unit.Dp(sizeDp))
	box := image.Pt(sizePx, sizePx)
	gtx.Constraints.Min = box
	gtx.Constraints.Max = box

	col := channelAvatarColor(channelID)
	// Clip circle
	circle := clip.Ellipse(image.Rect(0, 0, sizePx, sizePx))
	paint.FillShape(gtx.Ops, col, circle.Op(gtx.Ops))

	// White letter centered
	initial := channelInitial(channelTitle)
	spSize := unit.Sp(sizeDp * 14 / 36)
	if spSize < 9 {
		spSize = 9
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, spSize, initial)
		lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		lbl.Font.Weight = 600
		return lbl.Layout(gtx)
	})
}

// drawPlayTriangle draws a centered white play triangle inside a rectangle.
func drawPlayTriangle(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)

	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx-float32(gtx.Dp(4)), cy-float32(gtx.Dp(5))))
	p.LineTo(f32.Pt(cx-float32(gtx.Dp(4)), cy+float32(gtx.Dp(5))))
	p.LineTo(f32.Pt(cx+float32(gtx.Dp(5)), cy))
	p.Close()

	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
}

// drawMagnifier draws a magnifying glass inside bounds.
func drawMagnifier(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)

	r := float32(gtx.Dp(6))
	circleCx := cx - float32(gtx.Dp(2))
	circleCy := cy - float32(gtx.Dp(2))

	circleRect := image.Rect(
		int(circleCx-r), int(circleCy-r),
		int(circleCx+r), int(circleCy+r),
	)
	strokeWidth := float32(gtx.Dp(2))
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  clip.Ellipse(circleRect).Path(gtx.Ops),
		Width: strokeWidth,
	}.Op())

	// Handle
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(circleCx+float32(gtx.Dp(4)), circleCy+float32(gtx.Dp(4))))
	p.LineTo(f32.Pt(circleCx+float32(gtx.Dp(9)), circleCy+float32(gtx.Dp(9))))
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  p.End(),
		Width: strokeWidth + 0.5,
	}.Op())
}

// drawHouseIcon draws the Home house icon.
func drawHouseIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	minX := float32(bounds.Min.X)
	minY := float32(bounds.Min.Y)
	scale := float32(bounds.Dx()) / 24.0

	strokeW := 1.8 * scale

	// Roof
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(minX+3*scale, minY+12*scale))
	p.LineTo(f32.Pt(minX+12*scale, minY+4*scale))
	p.LineTo(f32.Pt(minX+21*scale, minY+12*scale))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: strokeW}.Op())

	// Walls
	var w clip.Path
	w.Begin(gtx.Ops)
	w.MoveTo(f32.Pt(minX+5.5*scale, minY+11*scale))
	w.LineTo(f32.Pt(minX+5.5*scale, minY+20*scale))
	w.LineTo(f32.Pt(minX+18.5*scale, minY+20*scale))
	w.LineTo(f32.Pt(minX+18.5*scale, minY+11*scale))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: w.End(), Width: strokeW}.Op())

	// Door
	doorRect := image.Rect(
		int(minX+10*scale), int(minY+14*scale),
		int(minX+14*scale), int(minY+20*scale),
	)
	paint.FillShape(gtx.Ops, col, clip.Rect(doorRect).Op())
}

// drawStackedRectsIcon draws the Subscriptions stacked rectangles icon.
func drawStackedRectsIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	minX := float32(bounds.Min.X)
	minY := float32(bounds.Min.Y)
	scale := float32(bounds.Dx()) / 24.0

	// Top line
	top := clip.UniformRRect(image.Rect(
		int(minX+7*scale), int(minY+4*scale),
		int(minX+17*scale), int(minY+6*scale),
	), int(scale))
	paint.FillShape(gtx.Ops, col, top.Op(gtx.Ops))

	// Middle line
	mid := clip.UniformRRect(image.Rect(
		int(minX+5*scale), int(minY+8*scale),
		int(minX+19*scale), int(minY+10*scale),
	), int(scale))
	paint.FillShape(gtx.Ops, col, mid.Op(gtx.Ops))

	// Bottom main box (with play triangle inside)
	bot := clip.UniformRRect(image.Rect(
		int(minX+3*scale), int(minY+12*scale),
		int(minX+21*scale), int(minY+21*scale),
	), int(2*scale))
	paint.FillShape(gtx.Ops, col, bot.Op(gtx.Ops))

	// Small cutout triangle inside bottom box
	cx := minX + 12*scale
	cy := minY + 16.5*scale
	var tri clip.Path
	tri.Begin(gtx.Ops)
	tri.MoveTo(f32.Pt(cx-2*scale, cy-2.5*scale))
	tri.LineTo(f32.Pt(cx-2*scale, cy+2.5*scale))
	tri.LineTo(f32.Pt(cx+2.5*scale, cy))
	tri.Close()
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x0f, G: 0x0f, B: 0x0f, A: 0xff}, clip.Outline{Path: tri.End()}.Op())
}

// drawClockIcon draws a clock icon inside bounds.
func drawClockIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)
	scale := float32(bounds.Dx()) / 24.0

	r := 8.5 * scale
	circleRect := image.Rect(int(cx-r), int(cy-r), int(cx+r), int(cy+r))
	strokeW := 1.8 * scale
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  clip.Ellipse(circleRect).Path(gtx.Ops),
		Width: strokeW,
	}.Op())

	// Hands: 12 o'clock and ~3:15
	var h clip.Path
	h.Begin(gtx.Ops)
	h.MoveTo(f32.Pt(cx, cy))
	h.LineTo(f32.Pt(cx, cy-5*scale))
	h.MoveTo(f32.Pt(cx, cy))
	h.LineTo(f32.Pt(cx+4.5*scale, cy))
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  h.End(),
		Width: strokeW,
	}.Op())
}

// drawHistoryIcon draws a counter-clockwise arrow / history clock icon.
func drawHistoryIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)
	scale := float32(bounds.Dx()) / 24.0

	r := 8.0 * scale
	circleRect := image.Rect(int(cx-r), int(cy-r), int(cx+r), int(cy+r))
	strokeW := 1.8 * scale

	// Outer circle
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  clip.Ellipse(circleRect).Path(gtx.Ops),
		Width: strokeW,
	}.Op())

	// Arrowhead at top left (pointing counter-clockwise)
	var arrow clip.Path
	arrow.Begin(gtx.Ops)
	arrow.MoveTo(f32.Pt(cx-6*scale, cy-7*scale))
	arrow.LineTo(f32.Pt(cx-3*scale, cy-11*scale))
	arrow.LineTo(f32.Pt(cx-1*scale, cy-6*scale))
	arrow.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: arrow.End()}.Op())

	// Center hands
	var h clip.Path
	h.Begin(gtx.Ops)
	h.MoveTo(f32.Pt(cx, cy))
	h.LineTo(f32.Pt(cx, cy-4*scale))
	h.MoveTo(f32.Pt(cx, cy))
	h.LineTo(f32.Pt(cx-3.5*scale, cy))
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  h.End(),
		Width: strokeW,
	}.Op())
}

// drawPlayPauseIcon draws either play triangle or pause bars.
func drawPlayPauseIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA, paused bool) {
	if paused {
		drawPlayTriangle(gtx, bounds, col)
		return
	}
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)
	barW := float32(gtx.Dp(3))
	barH := float32(gtx.Dp(10))
	gap := float32(gtx.Dp(2.5))

	r1 := image.Rect(int(cx-gap-barW), int(cy-barH/2), int(cx-gap), int(cy+barH/2))
	r2 := image.Rect(int(cx+gap), int(cy-barH/2), int(cx+gap+barW), int(cy+barH/2))
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(r1, gtx.Dp(1)).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(r2, gtx.Dp(1)).Op(gtx.Ops))
}

// drawSpeakerIcon draws a speaker cone, with waves or slash if muted.
func drawSpeakerIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA, muted bool) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)
	scale := float32(bounds.Dx()) / 24.0

	// Speaker body
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx-6*scale, cy-3*scale))
	p.LineTo(f32.Pt(cx-3*scale, cy-3*scale))
	p.LineTo(f32.Pt(cx+1*scale, cy-7*scale))
	p.LineTo(f32.Pt(cx+1*scale, cy+7*scale))
	p.LineTo(f32.Pt(cx-3*scale, cy+3*scale))
	p.LineTo(f32.Pt(cx-6*scale, cy+3*scale))
	p.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())

	if muted {
		// Mute X mark
		var x clip.Path
		x.Begin(gtx.Ops)
		x.MoveTo(f32.Pt(cx+3.5*scale, cy-3*scale))
		x.LineTo(f32.Pt(cx+7.5*scale, cy+3*scale))
		x.MoveTo(f32.Pt(cx+7.5*scale, cy-3*scale))
		x.LineTo(f32.Pt(cx+3.5*scale, cy+3*scale))
		paint.FillShape(gtx.Ops, col, clip.Stroke{Path: x.End(), Width: 1.8 * scale}.Op())
	} else {
		// Sound wave arc
		var w clip.Path
		w.Begin(gtx.Ops)
		w.MoveTo(f32.Pt(cx+4*scale, cy-4*scale))
		w.QuadTo(f32.Pt(cx+7*scale, cy), f32.Pt(cx+4*scale, cy+4*scale))
		paint.FillShape(gtx.Ops, col, clip.Stroke{Path: w.End(), Width: 1.8 * scale}.Op())
	}
}

// drawFullscreenIcon draws a 4-corner fullscreen bracket icon.
func drawFullscreenIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA, isFullscreen bool) {
	minX := float32(bounds.Min.X)
	minY := float32(bounds.Min.Y)
	scale := float32(bounds.Dx()) / 24.0
	arm := 4.0 * scale
	strokeW := 1.8 * scale

	x1, y1 := minX+4*scale, minY+4*scale
	x2, y2 := minX+20*scale, minY+20*scale

	var p clip.Path
	p.Begin(gtx.Ops)
	if !isFullscreen {
		// Top-left
		p.MoveTo(f32.Pt(x1, y1+arm))
		p.LineTo(f32.Pt(x1, y1))
		p.LineTo(f32.Pt(x1+arm, y1))
		// Top-right
		p.MoveTo(f32.Pt(x2-arm, y1))
		p.LineTo(f32.Pt(x2, y1))
		p.LineTo(f32.Pt(x2, y1+arm))
		// Bottom-left
		p.MoveTo(f32.Pt(x1, y2-arm))
		p.LineTo(f32.Pt(x1, y2))
		p.LineTo(f32.Pt(x1+arm, y2))
		// Bottom-right
		p.MoveTo(f32.Pt(x2-arm, y2))
		p.LineTo(f32.Pt(x2, y2))
		p.LineTo(f32.Pt(x2, y2-arm))
	} else {
		// Inwards corners when fullscreen
		cx := minX + 12*scale
		cy := minY + 12*scale
		p.MoveTo(f32.Pt(cx-arm, cy-1*scale))
		p.LineTo(f32.Pt(cx-arm, cy-arm))
		p.LineTo(f32.Pt(cx-1*scale, cy-arm))

		p.MoveTo(f32.Pt(cx+arm, cy-1*scale))
		p.LineTo(f32.Pt(cx+arm, cy-arm))
		p.LineTo(f32.Pt(cx+1*scale, cy-arm))

		p.MoveTo(f32.Pt(cx-arm, cy+1*scale))
		p.LineTo(f32.Pt(cx-arm, cy+arm))
		p.LineTo(f32.Pt(cx-1*scale, cy+arm))

		p.MoveTo(f32.Pt(cx+arm, cy+1*scale))
		p.LineTo(f32.Pt(cx+arm, cy+arm))
		p.LineTo(f32.Pt(cx+1*scale, cy+arm))
	}
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: strokeW}.Op())
}

// drawSkipPrevIcon draws a previous track glyph (|◀).
func drawSkipPrevIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)
	scale := float32(bounds.Dx()) / 24.0

	// Bar
	barRect := image.Rect(int(cx-5*scale), int(cy-5*scale), int(cx-3.5*scale), int(cy+5*scale))
	paint.FillShape(gtx.Ops, col, clip.Rect(barRect).Op())

	// Triangle pointing left
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx+4*scale, cy-5*scale))
	p.LineTo(f32.Pt(cx+4*scale, cy+5*scale))
	p.LineTo(f32.Pt(cx-3*scale, cy))
	p.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
}

// drawSkipNextIcon draws a next track glyph (▶|).
func drawSkipNextIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)
	scale := float32(bounds.Dx()) / 24.0

	// Triangle pointing right
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx-4*scale, cy-5*scale))
	p.LineTo(f32.Pt(cx-4*scale, cy+5*scale))
	p.LineTo(f32.Pt(cx+3*scale, cy))
	p.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())

	// Bar
	barRect := image.Rect(int(cx+3.5*scale), int(cy-5*scale), int(cx+5*scale), int(cy+5*scale))
	paint.FillShape(gtx.Ops, col, clip.Rect(barRect).Op())
}

// drawClearXIcon draws a small 'x' icon.
func drawClearXIcon(gtx layout.Context, bounds image.Rectangle, col color.NRGBA) {
	cx := float32(bounds.Min.X + bounds.Dx()/2)
	cy := float32(bounds.Min.Y + bounds.Dy()/2)
	r := float32(gtx.Dp(4))

	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx-r, cy-r))
	p.LineTo(f32.Pt(cx+r, cy+r))
	p.MoveTo(f32.Pt(cx+r, cy-r))
	p.LineTo(f32.Pt(cx-r, cy+r))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: float32(gtx.Dp(1.5))}.Op())
}

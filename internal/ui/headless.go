package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"lighttube/internal/demo"
	"lighttube/internal/yt"
)

// RenderPNG renders the UI offscreen at scale 2 to a PNG file.
func RenderPNG(opts Options, sizeStr, outPath string) error {
	if opts.Demo {
		if err := demo.ValidateAssets(); err != nil {
			return fmt.Errorf("demo assets error: %w", err)
		}
	}
	parts := strings.Split(strings.ToLower(sizeStr), "x")
	if len(parts) != 2 {
		return fmt.Errorf("invalid size format %q (expected WxH, e.g. 1440x1080)", sizeStr)
	}
	logW, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid width in size %q: %w", sizeStr, err)
	}
	logH, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid height in size %q: %w", sizeStr, err)
	}

	scale := float32(2.0)
	physW := int(float32(logW) * scale)
	physH := int(float32(logH) * scale)

	w, err := headless.NewWindow(physW, physH)
	if err != nil {
		return fmt.Errorf("initializing headless GPU window (%dx%d): %w", physW, physH, err)
	}
	defer w.Release()

	ui := New(opts)
	defer func() {
		if ui.demoTmpDir != "" {
			_ = os.RemoveAll(ui.demoTmpDir)
		}
	}()

	// Determine target space either from opts.DebugSpace or from outPath filename
	targetSpace := opts.DebugSpace
	if targetSpace == "" {
		lowerOut := strings.ToLower(outPath)
		if strings.Contains(lowerOut, "shorts") {
			targetSpace = "shorts"
		} else if strings.Contains(lowerOut, "podcast") {
			targetSpace = "podcasts"
		} else if strings.Contains(lowerOut, "music") {
			targetSpace = "music"
		}
	}

	if targetSpace != "" {
		switch strings.ToLower(targetSpace) {
		case "shorts":
			ui.switchSpace(SpaceShorts)
		case "podcasts":
			ui.switchSpace(SpacePodcasts)
		case "music":
			ui.switchSpace(SpaceMusic)
		case "watch":
			ui.switchSpace(SpaceWatch)
		}
	}

	if opts.DebugWatch {
		var v yt.Video
		if len(ui.state.Videos) > 0 {
			v = ui.state.Videos[0]
		} else {
			v = yt.Video{ID: "demo_city", Title: "Sample Demo Video", Channel: "Field Notes · Demo", Duration: 1450}
		}
		ui.openWatchPage(v)
	}
	if opts.DebugChip >= 0 {
		ui.spaces.Active().Chip = opts.DebugChip
		switch ui.spaces.ActiveSpace {
		case SpaceWatch:
			ui.loadWatchSpaceContent(opts.DebugChip)
		case SpaceShorts:
			ui.loadShortsSpaceContent(opts.DebugChip)
		case SpacePodcasts:
			ui.loadPodcastsSpaceContent(opts.DebugChip)
		case SpaceMusic:
			ui.loadMusicSpaceContent(opts.DebugChip)
		}
	}
	if opts.AmbientOff {
		if ui.store != nil {
			_ = ui.store.SetAmbient(false)
		}
		off := false
		ui.settings.Ambient = &off
	}

	metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
	constraints := layout.Exact(image.Pt(physW, physH))

	deadline := time.Now().Add(5 * time.Second)
	var ops op.Ops

	for {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      metric,
			Constraints: constraints,
		}
		ui.layout(gtx)
		if err := w.Frame(&ops); err != nil {
			return fmt.Errorf("rendering headless frame: %w", err)
		}

		// Check if visible images are loaded
		allReady := false
		if opts.Demo {
			allReady = len(ui.demoLoading) == 0
		} else {
			ui.requestedMu.Lock()
			reqEmpty := len(ui.requested) == 0
			ui.requestedMu.Unlock()
			if reqEmpty && time.Since(opts.StartTime) > 300*time.Millisecond {
				allReady = true
			}
		}

		if allReady || time.Now().After(deadline) {
			// Render one final frame to ensure all newly decoded images are pushed to GPU
			ops.Reset()
			gtx := layout.Context{
				Ops:         &ops,
				Now:         time.Now(),
				Metric:      metric,
				Constraints: constraints,
			}
			ui.layout(gtx)
			_ = w.Frame(&ops)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Capture screenshot from headless window
	img := image.NewRGBA(image.Rect(0, 0, physW, physH))
	if err := w.Screenshot(img); err != nil {
		return fmt.Errorf("capturing headless screenshot: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating screenshot output file %s: %w", outPath, err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encoding screenshot to PNG: %w", err)
	}

	return nil
}

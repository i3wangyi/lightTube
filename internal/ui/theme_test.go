package ui

import (
	"os"
	"testing"
	"time"

	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/widget/material"
)

func TestFontLoadingSpeed(t *testing.T) {
	// 1. Measure default material.NewTheme() shaping on first use (scanning system fonts).
	thDef := material.NewTheme()
	defStart := time.Now()
	thDef.Shaper.LayoutString(text.Parameters{}, "Hello 資深工程師")
	defDur := time.Since(defStart)
	t.Logf("default shaper first LayoutString took: %v", defDur)

	// 2. Measure explicit collection with NoSystemFonts.
	start := time.Now()

	regularData, err := os.ReadFile("/usr/share/fonts/noto/NotoSans-Regular.ttf")
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	regularFaces, err := opentype.ParseCollection(regularData)
	if err != nil {
		t.Fatalf("parsing regular: %v", err)
	}

	medData, err := os.ReadFile("/usr/share/fonts/noto/NotoSans-Medium.ttf")
	if err != nil {
		t.Fatalf("reading medium: %v", err)
	}
	medFaces, err := opentype.ParseCollection(medData)
	if err != nil {
		t.Fatalf("parsing medium: %v", err)
	}

	cjkData, err := os.ReadFile("/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc")
	if err != nil {
		t.Fatalf("reading cjk: %v", err)
	}
	cjkFaces, err := opentype.ParseCollection(cjkData)
	if err != nil {
		t.Fatalf("parsing cjk: %v", err)
	}

	allFaces := append(regularFaces, medFaces...)
	allFaces = append(allFaces, cjkFaces...)

	shaper := text.NewShaper(text.NoSystemFonts(), text.WithCollection(allFaces))
	loadDur := time.Since(start)
	t.Logf("explicit font collection loading took: %v", loadDur)

	shapeStart := time.Now()
	shaper.LayoutString(text.Parameters{}, "Hello 資深工程師")
	shapeDur := time.Since(shapeStart)
	t.Logf("explicit shaper first LayoutString took: %v", shapeDur)
	t.Logf("explicit total (load + shape): %v", loadDur+shapeDur)
}

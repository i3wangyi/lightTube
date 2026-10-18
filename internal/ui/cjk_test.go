package ui

import (
	"os"
	"testing"

	"gioui.org/font/opentype"
	"gioui.org/text"
)

func TestCJKGlyphRendering(t *testing.T) {
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
	shaper.LayoutString(text.Parameters{}, "資深工程師")

	glyphCount := 0
	for {
		glyph, ok := shaper.NextGlyph()
		if !ok {
			break
		}
		glyphCount++
		t.Logf("glyph #%d: ID=%d, X=%v, Y=%v", glyphCount, glyph.ID, glyph.X, glyph.Y)
		if glyph.ID == 0 {
			t.Errorf("glyph #%d has ID 0 (.notdef / missing glyph!)", glyphCount)
		}
	}
	if glyphCount != 5 {
		t.Errorf("expected 5 glyphs for 資深工程師, got %d", glyphCount)
	}
}

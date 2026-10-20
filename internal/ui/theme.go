package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/widget/material"
	fontapi "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"

	"lighttube/internal/platform"
)

// ThemeColors defines the color scheme for the dark theme matching DESIGN.md Phase 10.
type ThemeColors struct {
	Bg             color.NRGBA
	Surface        color.NRGBA
	Raised         color.NRGBA
	Hover          color.NRGBA
	Border         color.NRGBA
	BorderFocused  color.NRGBA
	Text           color.NRGBA
	Text2          color.NRGBA
	Accent         color.NRGBA
	Action         color.NRGBA
	Overlay        color.NRGBA
	AmbientWatch   color.NRGBA
	AmbientMusic   color.NRGBA
	BadgeBg        color.NRGBA
	BadgeFg        color.NRGBA
	ChipActiveBg   color.NRGBA
	ChipActiveText color.NRGBA

	// Aliases for compatibility with earlier code
	Red           color.NRGBA
	SurfaceHover  color.NRGBA
	SurfaceSelect color.NRGBA
	Fg            color.NRGBA
	Subdued       color.NRGBA
}

// DarkColors contains color definitions for the dark UI mode from DESIGN.md Phase 10.
var DarkColors = ThemeColors{
	Bg:             color.NRGBA{R: 0x0f, G: 0x0f, B: 0x0f, A: 0xff},
	Surface:        color.NRGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xff},
	Raised:         color.NRGBA{R: 0x27, G: 0x27, B: 0x27, A: 0xff},
	Hover:          color.NRGBA{R: 0x35, G: 0x35, B: 0x35, A: 0xff},
	Border:         color.NRGBA{R: 0x38, G: 0x38, B: 0x38, A: 0xff},
	BorderFocused:  color.NRGBA{R: 0x1c, G: 0x62, B: 0xb9, A: 0xff},
	Text:           color.NRGBA{R: 0xf1, G: 0xf1, B: 0xf1, A: 0xff},
	Text2:          color.NRGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff},
	Accent:         color.NRGBA{R: 0xff, G: 0x00, B: 0x33, A: 0xff},
	Action:         color.NRGBA{R: 0xc9, G: 0x00, B: 0x28, A: 0xff},
	Overlay:        color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xb8}, // rgba(0,0,0,.72)
	AmbientWatch:   color.NRGBA{R: 88, G: 126, B: 161, A: 71},       // rgba(88,126,161,.28)
	AmbientMusic:   color.NRGBA{R: 80, G: 57, B: 134, A: 51},        // rgba(80,57,134,.20)
	BadgeBg:        color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xb8},
	BadgeFg:        color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	ChipActiveBg:   color.NRGBA{R: 0xf1, G: 0xf1, B: 0xf1, A: 0xff},
	ChipActiveText: color.NRGBA{R: 0x0f, G: 0x0f, B: 0x0f, A: 0xff},

	// Backwards compatibility
	Red:           color.NRGBA{R: 0xc9, G: 0x00, B: 0x28, A: 0xff},
	SurfaceHover:  color.NRGBA{R: 0x35, G: 0x35, B: 0x35, A: 0xff},
	SurfaceSelect: color.NRGBA{R: 0x27, G: 0x27, B: 0x27, A: 0xff},
	Fg:            color.NRGBA{R: 0xf1, G: 0xf1, B: 0xf1, A: 0xff},
	Subdued:       color.NRGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff},
}

type rawFontFace struct {
	face *fontapi.Font
}

func (r rawFontFace) Face() *fontapi.Face {
	return fontapi.NewFace(r.face)
}

// loadFontFile reads only one face's tables; a TTC can contain ten large faces.
func loadFontFile(path string) []font.FontFace {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	lds, err := ot.NewLoaders(f)
	if err != nil || len(lds) == 0 {
		return nil
	}
	target := lds[0]
	if len(lds) > 1 {
		for _, ld := range lds {
			buf, err := ld.RawTable(ot.MustNewTag("name"))
			if err != nil {
				continue
			}
			names, _, err := tables.ParseName(buf)
			if err == nil && (names.Name(1) == "Noto Sans CJK SC" || names.Name(4) == "Noto Sans CJK SC") {
				target = ld
				break
			}
		}
	}
	ft, err := fontapi.NewFont(target)
	if err != nil {
		return nil
	}
	return []font.FontFace{{Font: opentype.DescriptionToFont(ft.Describe()), Face: rawFontFace{face: ft}}}
}

func loadExplicitFonts() []font.FontFace {
	var faces []font.FontFace
	for _, path := range platform.FontFiles() {
		faces = append(faces, loadFontFile(path)...)
	}
	return faces
}

// themeFonts defers collection faces until displayed text needs a fallback.
// Gio opens every registered face eagerly, so registration itself must wait.
type themeFonts struct {
	faces    []font.FontFace
	fallback []string
}

func (f *themeFonts) ensure(th *material.Theme, value string) {
	if f == nil || len(f.fallback) == 0 {
		return
	}
	for _, r := range value {
		if r <= ' ' {
			continue
		}
		found := false
		for _, face := range f.faces {
			if _, ok := face.Face.(rawFontFace).face.Cmap.Lookup(r); ok {
				found = true
				break
			}
		}
		if found {
			continue
		}
		for _, path := range f.fallback {
			f.faces = append(f.faces, loadFontFile(path)...)
		}
		f.fallback = nil
		th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(f.faces))
		return
	}
}

func newLazyDarkTheme() (*material.Theme, ThemeColors, *themeFonts) {
	f := &themeFonts{}
	for _, path := range platform.FontFiles() {
		if strings.EqualFold(filepath.Ext(path), ".ttc") {
			f.fallback = append(f.fallback, path)
		} else {
			f.faces = append(f.faces, loadFontFile(path)...)
		}
	}
	// Retain the system font fallback when explicit platform fonts are absent.
	if len(f.faces) == 0 {
		for _, path := range f.fallback {
			f.faces = append(f.faces, loadFontFile(path)...)
		}
		f.fallback = nil
	}
	th, colors := darkTheme(f.faces)
	return th, colors, f
}

// NewDarkTheme initializes a material Theme configured with dark palette colors
// and an explicit font collection, disabling full system font scanning for sub-500ms startup.
// When no explicit font files are found, it falls back to Gio's system font scan.
func NewDarkTheme() (*material.Theme, ThemeColors) {
	return darkTheme(loadExplicitFonts())
}

func darkTheme(faces []font.FontFace) (*material.Theme, ThemeColors) {
	var shaper *text.Shaper
	if len(faces) > 0 {
		shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
	} else {
		shaper = text.NewShaper()
	}

	th := material.NewTheme()
	th.Shaper = shaper
	th.Palette = material.Palette{
		Bg:         DarkColors.Bg,
		Fg:         DarkColors.Text,
		ContrastBg: DarkColors.Action,
		ContrastFg: DarkColors.Text,
	}
	return th, DarkColors
}

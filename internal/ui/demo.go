package ui

import (
	"image"
	"time"

	"lighttube/internal/demo"
	"lighttube/internal/store"
	"lighttube/internal/yt"
)

// DemoFiles lists the 12 embedded demo image filenames.
var DemoFiles = []string{
	"city.jpg",
	"workshop.jpg",
	"camera.jpg",
	"performance.jpg",
	"forest.jpg",
	"architecture.jpg",
	"vinyl.jpg",
	"microphone.jpg",
	"ocean.jpg",
	"night.jpg",
	"portrait-city.jpg",
	"portrait-forest.jpg",
}

// DemoData contains the fictional records and position data for --demo mode.
type DemoData struct {
	Hero         yt.Video
	HeroPosition store.Position
	Videos       []yt.Video
	Positions    map[string]store.Position
	SavedTitles  map[string]bool
}

// PrefetchDemoImages triggers background decoding of all embedded demo images
// so they do not block the UI thread on the first frame.
func PrefetchDemoImages() {
	for _, f := range DemoFiles {
		_, _ = demo.GetImage(f)
	}
}

// LoadDemoData returns the fictional demo records and metadata without blocking for image decoding.
func LoadDemoData() *DemoData {
	hero := yt.Video{
		ID:          "demo_city",
		Title:       "The city after everyone goes home",
		Channel:     "Field Notes · Demo",
		ChannelID:   "demo_field_notes",
		Duration:    1450, // 24:10
		Description: "A different view of the streets we think we know. Sample video · 24 minutes",
	}

	heroPos := store.Position{
		Seconds:  522,  // 8:42
		Duration: 1450, // 24:10
		Updated:  time.Now(),
	}

	videos := []yt.Video{
		{
			ID:          "demo_workshop",
			Title:       "The quiet craft of making things",
			Channel:     "Object Studio · Demo",
			ChannelID:   "demo_object_studio",
			Duration:    1104, // 18:24
			Description: "Design · Sample video",
		},
		{
			ID:          "demo_camera",
			Title:       "Why a small camera changes everything",
			Channel:     "Frame by Frame · Demo",
			ChannelID:   "demo_frame_by_frame",
			Duration:    728, // 12:08
			Description: "Photography · Sample video",
		},
		{
			ID:          "demo_performance",
			Title:       "The sound of being in the room",
			Channel:     "Room Tone · Demo",
			ChannelID:   "demo_room_tone",
			Duration:    1936, // 32:16
			Description: "Music culture · Sample video",
		},
		{
			ID:          "demo_forest",
			Title:       "A minute away becomes an afternoon",
			Channel:     "Field Notes · Demo",
			ChannelID:   "demo_field_notes",
			Duration:    1302, // 21:42
			Description: "Outdoors · Sample video",
		},
		{
			ID:          "demo_architecture",
			Title:       "Who gets to shape the skyline?",
			Channel:     "Common Ground · Demo",
			ChannelID:   "demo_common_ground",
			Duration:    1010, // 16:50
			Description: "Architecture · Sample video",
		},
		{
			ID:          "demo_vinyl",
			Title:       "The photographs we leave behind",
			Channel:     "Paper Archive · Demo",
			ChannelID:   "demo_paper_archive",
			Duration:    1623, // 27:03
			Description: "Visual culture · Sample video",
		},
	}

	positions := map[string]store.Position{
		"demo_city":        heroPos,
		"demo_workshop":    {Seconds: 450, Duration: 1104, Updated: time.Now().Add(-1 * time.Hour)},
		"demo_performance": {Seconds: 1900, Duration: 1936, Updated: time.Now().Add(-2 * time.Hour)}, // played (>=95%)
	}

	saved := map[string]bool{
		"The quiet craft of making things":  true,
		"The city after everyone goes home": true,
	}

	return &DemoData{
		Hero:         hero,
		HeroPosition: heroPos,
		Videos:       videos,
		Positions:    positions,
		SavedTitles:  saved,
	}
}

// getDemoImage returns the decoded image if cached, or initiates a background decode.
func (ui *UI) getDemoImage(filename string) (image.Image, bool) {
	if img, ok := demo.CachedImage(filename); ok {
		return img, true
	}
	if ui.demoLoading == nil {
		ui.demoLoading = make(map[string]bool)
	}
	if !ui.demoLoading[filename] {
		ui.demoLoading[filename] = true
		go func() {
			img, err := demo.GetImage(filename)
			ui.sendAction(func() {
				delete(ui.demoLoading, filename)
				if err == nil && img != nil {
					ui.invalidate()
				}
			})
		}()
	}
	return nil, false
}

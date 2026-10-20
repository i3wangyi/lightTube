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

// DemoShort represents a sample portrait short for --demo mode.
type DemoShort struct {
	ID         string
	Title      string
	Caption    string
	ImageFile  string
	Channel    string
	SourceLink string
}

// DemoShow represents a sample podcast show for --demo mode.
type DemoShow struct {
	ID        string
	Title     string
	Creator   string
	Genre     string
	Art       string
	ImageFile string
}

// DemoEpisode represents a sample podcast episode for --demo mode.
type DemoEpisode struct {
	ID           string
	Title        string
	Show         string
	Number       string
	Duration     int
	Seconds      float64
	ProgressText string
	Resume       bool
	ImageFile    string
}

// DemoAlbum represents a sample music album feature for --demo mode.
type DemoAlbum struct {
	Title       string
	Artist      string
	Art         string
	Description string
	ImageFile   string
}

// DemoTrack represents a sample numbered music track for --demo mode.
type DemoTrack struct {
	ID          string
	Title       string
	Artist      string
	Album       string
	Duration    int
	DurationStr string
	ImageFile   string
}

// DemoData contains the fictional records and position data for --demo mode.
type DemoData struct {
	Hero         yt.Video
	HeroPosition store.Position
	Videos       []yt.Video
	Positions    map[string]store.Position
	SavedTitles  map[string]bool

	// Phase 10b Spaces Demo Data
	Shorts       []DemoShort
	RelatedShort DemoShort
	RelatedWatch yt.Video
	Shows        []DemoShow
	Episodes     []DemoEpisode
	Album        DemoAlbum
	Tracks       []DemoTrack
	DockTrack    DemoTrack
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
		"demo_ep_place":    {Seconds: 420, Duration: 1800, Updated: time.Now().Add(-30 * time.Minute)},
		"demo_ep_shape":    {Seconds: 240, Duration: 2700, Updated: time.Now().Add(-15 * time.Minute)},
	}

	saved := map[string]bool{
		"The quiet craft of making things":      true,
		"The city after everyone goes home":     true,
		"After Hours":                           true,
		"Look up. There is a whole other city.": true,
	}

	// Shorts demo items
	shorts := []DemoShort{
		{
			ID:         "demo_short_city",
			Title:      "Look up. There is a whole other city.",
			Caption:    "The details you miss on your morning walk.",
			ImageFile:  "portrait-city.jpg",
			Channel:    "@fieldnotes · Fictional creator",
			SourceLink: "https://www.youtube.com/watch?v=demo_city",
		},
		{
			ID:         "demo_short_forest",
			Title:      "One minute. No notifications.",
			Caption:    "Take the long way home.",
			ImageFile:  "portrait-forest.jpg",
			Channel:    "@fieldnotes · Fictional creator",
			SourceLink: "",
		},
		{
			ID:         "demo_short_night",
			Title:      "The city after the rush.",
			Caption:    "Find a different rhythm in familiar streets.",
			ImageFile:  "city.jpg",
			Channel:    "@fieldnotes · Fictional creator",
			SourceLink: "",
		},
	}

	relatedShort := DemoShort{
		ID:        "demo_related_short",
		Title:     "A minute away from everything",
		Channel:   "Field Notes · Sample short",
		ImageFile: "forest.jpg",
	}

	relatedWatch := yt.Video{
		ID:          "demo_city",
		Title:       "The full story behind the frame",
		Channel:     "Field Notes · Demo",
		Duration:    1450,
		URL:         "https://www.youtube.com/watch?v=demo_city",
		Description: "The city after everyone goes home",
	}

	// Podcasts demo items
	shows := []DemoShow{
		{
			ID:        "demo_podcast_longview",
			Title:     "The Long View",
			Creator:   "Perspective Studio",
			Genre:     "Ideas & culture",
			Art:       "THE LONG VIEW",
			ImageFile: "microphone.jpg",
		},
		{
			ID:        "demo_podcast_offtheclock",
			Title:     "Off the Clock",
			Creator:   "Second Nature",
			Genre:     "Life & work",
			Art:       "OFF THE CLOCK",
			ImageFile: "night.jpg",
		},
		{
			ID:        "demo_podcast_systems",
			Title:     "Systems of Living",
			Creator:   "Common Ground",
			Genre:     "Design & society",
			Art:       "SYSTEMS OF LIVING",
			ImageFile: "architecture.jpg",
		},
		{
			ID:        "demo_podcast_betweentakes",
			Title:     "Between Takes",
			Creator:   "Frame by Frame",
			Genre:     "Film & craft",
			Art:       "BETWEEN TAKES",
			ImageFile: "performance.jpg",
		},
	}

	episodes := []DemoEpisode{
		{
			ID:           "demo_ep_place",
			Title:        "What makes a place feel like home?",
			Show:         "The Long View",
			Number:       "Episode 48",
			Duration:     1800,
			Seconds:      420,
			ProgressText: "23 min left",
			Resume:       true,
			ImageFile:    "microphone.jpg",
		},
		{
			ID:           "demo_ep_case",
			Title:        "The case for doing one thing well",
			Show:         "Off the Clock",
			Number:       "Episode 12",
			Duration:     3120,
			Seconds:      0,
			ProgressText: "52 min",
			Resume:       false,
			ImageFile:    "night.jpg",
		},
		{
			ID:           "demo_ep_shape",
			Title:        "Who gets to shape the city?",
			Show:         "Systems of Living",
			Number:       "Episode 26",
			Duration:     2700,
			Seconds:      240,
			ProgressText: "41 min left",
			Resume:       true,
			ImageFile:    "architecture.jpg",
		},
	}

	// Music demo items
	album := DemoAlbum{
		Title:       "After Hours",
		Artist:      "Low Window, Paper Echo, North Arcade",
		Art:         "AFTER\nHOURS",
		Description: "Low Window, Paper Echo, North Arcade\nInstrumental selections · 5 fictional tracks",
		ImageFile:   "night.jpg",
	}

	tracks := []DemoTrack{
		{
			ID:          "demo_trk_bluehour",
			Title:       "Blue Hour",
			Artist:      "Low Window",
			Album:       "After Hours",
			Duration:    252,
			DurationStr: "4:12",
			ImageFile:   "night.jpg",
		},
		{
			ID:          "demo_trk_geometry",
			Title:       "Soft Geometry",
			Artist:      "Paper Echo",
			Album:       "Quiet Forms",
			Duration:    228,
			DurationStr: "3:48",
			ImageFile:   "ocean.jpg",
		},
		{
			ID:          "demo_trk_space",
			Title:       "The Space Between",
			Artist:      "North Arcade",
			Album:       "After Hours",
			Duration:    306,
			DurationStr: "5:06",
			ImageFile:   "night.jpg",
		},
		{
			ID:          "demo_trk_slowmotion",
			Title:       "Slow Motion",
			Artist:      "Low Window",
			Album:       "After Hours",
			Duration:    272,
			DurationStr: "4:32",
			ImageFile:   "night.jpg",
		},
		{
			ID:          "demo_trk_lastlight",
			Title:       "Last Light",
			Artist:      "Paper Echo",
			Album:       "Quiet Forms",
			Duration:    235,
			DurationStr: "3:55",
			ImageFile:   "ocean.jpg",
		},
	}

	dockTrack := DemoTrack{
		ID:          "demo_trk_bluehour",
		Title:       "Blue Hour · Low Window",
		Artist:      "Low Window",
		Album:       "After Hours",
		Duration:    252,
		DurationStr: "1:31 / 4:12",
		ImageFile:   "night.jpg",
	}

	return &DemoData{
		Hero:         hero,
		HeroPosition: heroPos,
		Videos:       videos,
		Positions:    positions,
		SavedTitles:  saved,
		Shorts:       shorts,
		RelatedShort: relatedShort,
		RelatedWatch: relatedWatch,
		Shows:        shows,
		Episodes:     episodes,
		Album:        album,
		Tracks:       tracks,
		DockTrack:    dockTrack,
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
			})
			if err == nil && img != nil {
				ui.invalidate()
			}
		}()
	}
	return nil, false
}

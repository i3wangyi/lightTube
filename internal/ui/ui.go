package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"lighttube/internal/embed"
	"lighttube/internal/player"
	"lighttube/internal/store"
	"lighttube/internal/thumbs"
	"lighttube/internal/yt"
)

// Options holds runtime dependencies injected into the UI.
type Options struct {
	YTClient         *yt.Client
	Store            *store.Store
	Player           *player.Player
	Thumbs           *thumbs.Manager
	Window           *app.Window
	Debug            bool
	StartTime        time.Time
	Search           string
	ExternalPlayer   bool
	DebugPlay        string
	DebugPlaySeconds int
}

// UI manages the Gio window, layout, and event interactions.
type UI struct {
	ytClient *yt.Client
	store    *store.Store
	player   *player.Player
	thumbs   *thumbs.Manager
	window   *app.Window

	th     *material.Theme
	colors ThemeColors
	state  *State

	power player.Power
	mode  player.Mode

	debug            bool
	startTime        time.Time
	initialSearch    string
	firstFrameLogged bool
	dataLoaded       bool

	// Options & flags
	externalPlayer     bool
	debugPlay          string
	debugPlaySeconds   int
	debugPlayTriggered bool

	// Embedded player & wayland view
	waylandView  app.WaylandViewEvent
	embedPlayer  *embed.Player
	currentScale int
	hwdecLogged  bool
	lastTimePoll time.Time
	previousTab  Tab

	// Watch page state & controls
	watchVideo            yt.Video
	upNextVideos          []yt.Video
	watchPaused           bool
	watchMuted            bool
	watchVolume           float64
	watchCurrentTime      float64
	watchDuration         float64
	watchFullscreen       bool
	watchLastMouseMove    time.Time
	lastVideoClickTime    time.Time
	watchPlayPauseClick   widget.Clickable
	watchMuteClick        widget.Clickable
	watchFullscreenClick  widget.Clickable
	watchExternalMPVClick widget.Clickable
	watchSubscribeClick   widget.Clickable
	watchLaterClick       widget.Clickable
	watchVideoTag         event.Tag
	watchProgressTag      event.Tag
	watchList             layout.List
	upNextList            layout.List
	upNextClicks          []widget.Clickable
	searchClearClick      widget.Clickable

	actionsMu sync.Mutex
	actions   []func()

	// Mini guide (4 items: Home, Subscriptions, Watch later, History)
	guideClicks [4]widget.Clickable

	// Top search bar
	searchEditor widget.Editor
	searchBtn    widget.Clickable
	logoClick    widget.Clickable

	// Chips row on Home
	chipsList         layout.List
	allChipClick      widget.Clickable
	channelChipClicks []widget.Clickable

	// Video grid / lists
	gridList          layout.List
	cardClicks        []widget.Clickable
	cardClockClicks   []widget.Clickable
	searchList        layout.List
	searchItemClicks  []widget.Clickable
	searchClockClicks []widget.Clickable

	// Subscriptions controls
	addChannelEditor  widget.Editor
	addChannelBtn     widget.Clickable
	csvPathEditor     widget.Editor
	importCSVBtn      widget.Clickable
	subList           layout.List
	unsubscribeClicks []widget.Clickable

	// Requested thumbnail set to prevent re-requesting in-flight images on every frame
	requestedMu sync.Mutex
	requested   map[string]bool

	// ImageOp cache to avoid recreating texture operations on every frame
	imageOps map[string]paint.ImageOp

	// Event tags
	windowTag       event.Tag
	pasteTag        event.Tag
	pendingFocusTag event.Tag
}

// New creates and initializes a UI instance with explicit fonts and zero I/O.
func New(opts Options) *UI {
	ytClient := opts.YTClient
	if ytClient == nil {
		ytClient = yt.NewClient()
	}
	st := opts.Store
	if st == nil {
		st, _ = store.NewDeferred(store.DefaultDataDir())
	}
	p := opts.Player
	if p == nil {
		p = player.New()
	}
	thMgr := opts.Thumbs
	if thMgr == nil {
		thMgr = thumbs.New()
	}

	th, colors := NewDarkTheme()

	ui := &UI{
		ytClient:           ytClient,
		store:              st,
		player:             p,
		thumbs:             thMgr,
		window:             opts.Window,
		th:                 th,
		colors:             colors,
		state:              NewState(),
		power:              player.DetectPower(),
		mode:               player.ModeVideo,
		debug:              opts.Debug,
		startTime:          opts.StartTime,
		initialSearch:      opts.Search,
		externalPlayer:     opts.ExternalPlayer,
		debugPlay:          opts.DebugPlay,
		debugPlaySeconds:   opts.DebugPlaySeconds,
		watchLastMouseMove: time.Now(),
		watchVolume:        100,
		currentScale:       2,
		requested:          make(map[string]bool),
		imageOps:           make(map[string]paint.ImageOp),
		windowTag:          new(int),
		pasteTag:           new(int),
	}

	ui.searchEditor.SingleLine = true
	ui.searchEditor.Submit = true
	if opts.Search != "" {
		ui.searchEditor.SetText(opts.Search)
	}

	ui.addChannelEditor.SingleLine = true
	ui.addChannelEditor.Submit = true

	ui.csvPathEditor.SingleLine = true
	ui.csvPathEditor.Submit = true

	ui.chipsList.Axis = layout.Horizontal
	ui.gridList.Axis = layout.Vertical
	ui.searchList.Axis = layout.Vertical
	ui.subList.Axis = layout.Vertical
	ui.watchList.Axis = layout.Vertical
	ui.upNextList.Axis = layout.Vertical

	return ui
}

// Run configures the application ID, initializes the window, and runs the event loop.
func Run(opts Options) error {
	app.ID = "lighttube"

	ui := New(opts)
	if ui.window == nil {
		ui.window = new(app.Window)
	}

	ui.window.Option(
		app.Title("lightTube"),
		app.Size(unit.Dp(1120), unit.Dp(720)),
		app.MinSize(unit.Dp(720), unit.Dp(480)),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- ui.eventLoop()
	}()

	app.Main()
	return <-errCh
}

func (ui *UI) sendAction(fn func()) {
	ui.actionsMu.Lock()
	ui.actions = append(ui.actions, fn)
	ui.actionsMu.Unlock()
}

func (ui *UI) drainActions() {
	ui.actionsMu.Lock()
	actions := ui.actions
	ui.actions = nil
	ui.actionsMu.Unlock()

	for _, fn := range actions {
		if fn != nil {
			fn()
		}
	}
}

func (ui *UI) eventLoop() error {
	var ops op.Ops
	for {
		e := ui.window.Event()
		switch e := e.(type) {
		case *app.ClosingEvent:
			if ui.embedPlayer != nil {
				ui.embedPlayer.Stop()
				ui.embedPlayer.Close()
				ui.embedPlayer = nil
			}

		case app.DestroyEvent:
			if ui.embedPlayer != nil {
				ui.embedPlayer.Stop()
				ui.embedPlayer.Close()
				ui.embedPlayer = nil
			}
			if e.Err != nil {
				os.Exit(1)
			}
			os.Exit(0)
			return nil

		case app.WaylandViewEvent:
			if e.Valid() {
				ui.waylandView = e
			} else {
				if ui.embedPlayer != nil {
					ui.embedPlayer.Stop()
					ui.embedPlayer.Close()
					ui.embedPlayer = nil
				}
				ui.waylandView = app.WaylandViewEvent{}
			}

		case app.ConfigEvent:
			if e.Config.Focused {
				ui.power = player.DetectPower()
			}

		case app.FrameEvent:
			ui.currentScale = int(e.Metric.PxPerDp)
			if ui.currentScale <= 0 {
				ui.currentScale = 2
			}

			// Log startup ms to first FrameEvent if --debug is enabled.
			if ui.debug && !ui.firstFrameLogged {
				ui.firstFrameLogged = true
				if !ui.startTime.IsZero() {
					ms := time.Since(ui.startTime).Milliseconds()
					fmt.Fprintf(os.Stderr, "startup: %d ms to first FrameEvent\n", ms)
				}
			}

			// Load store and feed data off the UI goroutine AFTER first frame.
			if !ui.dataLoaded {
				ui.dataLoaded = true
				go func() {
					_ = ui.store.LoadAll()
					ui.sendAction(func() {
						ui.onStoreLoaded()
						if ui.initialSearch != "" {
							ui.search(ui.initialSearch)
						}
						if ui.debugPlay != "" && !ui.debugPlayTriggered {
							ui.debugPlayTriggered = true
							go func() {
								time.Sleep(6 * time.Second)
								ui.sendAction(func() {
									ui.triggerDebugPlay(ui.debugPlay, ui.debugPlaySeconds)
								})
								ui.window.Invalidate()
							}()
						}
					})
					ui.window.Invalidate()
				}()
			}

			// Poll mpv status at most 4 times a second, and only while playing and window visible
			if ui.state.CurrentTab == TabWatch && ui.embedPlayer != nil {
				if time.Since(ui.lastTimePoll) >= 250*time.Millisecond {
					ui.lastTimePoll = time.Now()
					st := ui.embedPlayer.Status()
					ui.watchCurrentTime = st.TimePos
					if st.Duration > 0 {
						ui.watchDuration = st.Duration
					}
					ui.watchPaused = st.Paused
					ui.watchMuted = st.Muted
					ui.watchVolume = st.Volume
					if ui.debug && !ui.hwdecLogged && st.HWDecCurrent != "" && st.HWDecCurrent != "no" {
						ui.hwdecLogged = true
						fmt.Fprintf(os.Stderr, "hwdec-current=%s\n", st.HWDecCurrent)
						if ui.debugPlaySeconds > 0 {
							go func() {
								time.Sleep(time.Duration(ui.debugPlaySeconds) * time.Second)
								ui.sendAction(func() {
									ui.closeWatchPage()
								})
								ui.window.Invalidate()
							}()
						}
					}
				}
			}

			ui.drainActions()
			gtx := app.NewContext(&ops, e)
			ui.layout(gtx)

			// Invalidate for next timer tick when playing
			if ui.state.CurrentTab == TabWatch && !ui.watchPaused {
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(250 * time.Millisecond)})
			}
			if ui.state.CurrentTab == TabWatch && ui.watchFullscreen && time.Since(ui.watchLastMouseMove) < 3*time.Second {
				gtx.Execute(op.InvalidateCmd{At: ui.watchLastMouseMove.Add(3 * time.Second)})
			}

			e.Frame(gtx.Ops)
		}
	}
}

func (ui *UI) onStoreLoaded() {
	ui.state.Subscriptions = ui.store.Subscriptions()
	if ui.state.CurrentTab == TabHome {
		ui.loadMergedFeed()
	}
	runtime.GC()
	debug.FreeOSMemory()
}

func (ui *UI) updatePower() {
	ui.power = player.DetectPower()
}

func (ui *UI) switchTab(t Tab) {
	ui.state.SwitchTab(t)
	switch t {
	case TabHome:
		if len(ui.state.Subscriptions) == 0 {
			ui.state.Subscriptions = ui.store.Subscriptions()
		}
		if len(ui.state.Videos) == 0 {
			ui.loadMergedFeed()
		}
	case TabSubscriptions:
		ui.state.Subscriptions = ui.store.Subscriptions()
	case TabWatchLater:
		ui.state.SetVideos(ui.store.WatchLater())
	case TabHistory:
		ui.state.SetVideos(ui.store.History())
	case TabSearch:
		// Search results tab
	}
}

func (ui *UI) handleSearchSubmit() {
	txt := strings.TrimSpace(ui.searchEditor.Text())
	if txt == "" {
		return
	}

	// Paste: if the search box text is a YouTube URL, Enter plays it instead of searching.
	if id, err := yt.ExtractVideoID(txt); err == nil && id != "" {
		v := yt.Video{
			ID:    id,
			Title: "YouTube Video (" + id + ")",
			URL:   "https://www.youtube.com/watch?v=" + id,
		}
		ui.playVideo(v, false)
		return
	}

	ui.search(txt)
}

func (ui *UI) search(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	ui.state.CurrentTab = TabSearch
	ui.state.Status = fmt.Sprintf("Searching %q…", query)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		videos, err := ui.ytClient.Search(ctx, query)
		ui.sendAction(func() {
			if err != nil {
				ui.state.Status = "Search error: " + err.Error()
				return
			}
			ui.state.SetVideos(videos)
			ui.state.Status = ""
		})
		ui.window.Invalidate()
	}()
}

func (ui *UI) loadMergedFeed() {
	subs := ui.store.Subscriptions()
	ui.state.Subscriptions = subs
	if len(subs) == 0 {
		ui.state.SetVideos(nil)
		ui.state.Status = "No subscriptions yet. Add channels in Subscriptions tab or import CSV."
		return
	}

	ui.state.Status = "Loading subscription feed…"
	channelIDs := make([]string, 0, len(subs))
	for _, s := range subs {
		if s.ChannelID != "" {
			channelIDs = append(channelIDs, s.ChannelID)
		}
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		videos, err := ui.ytClient.MergedFeed(ctx, channelIDs)
		ui.sendAction(func() {
			if err != nil {
				ui.state.Status = "Feed error: " + err.Error()
				return
			}
			ui.state.SetVideos(videos)
			ui.state.Status = ""
			if ui.state.CurrentTab == TabWatch && len(ui.upNextVideos) == 0 {
				upNext := make([]yt.Video, 0, len(videos))
				for _, item := range videos {
					if item.ID != ui.watchVideo.ID {
						upNext = append(upNext, item)
					} else if ui.watchVideo.Title == "Test Video" {
						ui.watchVideo = item
					}
				}
				ui.upNextVideos = upNext
			}
		})
		ui.window.Invalidate()
	}()
}

func (ui *UI) addSubscription(target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}

	ui.state.Status = fmt.Sprintf("Resolving channel %q…", target)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		channelID, err := ui.ytClient.ResolveChannel(ctx, target)
		if err != nil {
			ui.sendAction(func() {
				ui.state.Status = "Failed to resolve channel: " + err.Error()
			})
			ui.window.Invalidate()
			return
		}

		title := channelID
		if feedVideos, feedErr := ui.ytClient.Feed(ctx, channelID); feedErr == nil && len(feedVideos) > 0 {
			if feedVideos[0].Channel != "" {
				title = feedVideos[0].Channel
			}
		}

		sub := yt.Subscription{
			ChannelID: channelID,
			Title:     title,
			URL:       "https://www.youtube.com/channel/" + channelID,
		}

		_ = ui.store.AddSubscription(sub)
		ui.sendAction(func() {
			ui.addChannelEditor.SetText("")
			ui.state.Subscriptions = ui.store.Subscriptions()
			ui.state.Status = "Subscribed to " + title
			if ui.state.CurrentTab == TabHome {
				ui.loadMergedFeed()
			}
		})
		ui.window.Invalidate()
	}()
}

func (ui *UI) importTakeout(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	ui.state.Status = "Importing Takeout CSV…"
	go func() {
		f, err := os.Open(path)
		if err != nil {
			ui.sendAction(func() {
				ui.state.Status = "CSV file error: " + err.Error()
			})
			ui.window.Invalidate()
			return
		}
		defer f.Close()

		subs, err := yt.ParseTakeoutCSV(f)
		if err != nil {
			ui.sendAction(func() {
				ui.state.Status = "Failed parsing CSV: " + err.Error()
			})
			ui.window.Invalidate()
			return
		}

		_ = ui.store.AddSubscriptions(subs)
		ui.sendAction(func() {
			ui.csvPathEditor.SetText("")
			ui.state.Subscriptions = ui.store.Subscriptions()
			ui.state.Status = fmt.Sprintf("Imported %d channels from CSV", len(subs))
			if ui.state.CurrentTab == TabHome {
				ui.loadMergedFeed()
			}
		})
		ui.window.Invalidate()
	}()
}

func (ui *UI) removeSubscription(channelID string) {
	_ = ui.store.RemoveSubscription(channelID)
	ui.state.Subscriptions = ui.store.Subscriptions()
	if ui.state.CurrentTab == TabHome {
		ui.loadMergedFeed()
	}
}

func (ui *UI) playSelected(audio bool) {
	v, ok := ui.state.SelectedVideo()
	if !ok {
		return
	}
	ui.playVideo(v, audio)
}

func (ui *UI) playVideo(v yt.Video, audio bool) {
	if audio || ui.externalPlayer {
		ui.playVideoExternal(v, audio)
		return
	}
	ui.openWatchPage(v)
}

func (ui *UI) playVideoExternal(v yt.Video, audio bool) {
	ui.mode = player.ModeVideo
	if audio {
		ui.mode = player.ModeAudio
	}
	ui.updatePower()

	modeText := "video"
	if audio {
		modeText = "audio"
	}
	ui.state.Status = fmt.Sprintf("Playing %s (external mpv): %s", modeText, v.Title)

	_ = ui.store.AddHistory(v)
	if ui.state.CurrentTab == TabHistory {
		ui.state.SetVideos(ui.store.History())
	}

	opts := player.Options{
		URL:   v.WatchURL(),
		Title: v.Title,
		Audio: audio,
	}

	go func() {
		_, err := ui.player.Start(opts)
		if err != nil {
			ui.sendAction(func() {
				ui.state.Status = "Playback error: " + err.Error()
			})
			ui.window.Invalidate()
		}
	}()
}

func (ui *UI) handlePastedText(text string) {
	id, err := yt.ExtractVideoID(text)
	if err != nil {
		ui.state.Status = "Clipboard does not contain a valid YouTube URL"
		return
	}

	v := yt.Video{
		ID:    id,
		Title: "YouTube Video (" + id + ")",
		URL:   "https://www.youtube.com/watch?v=" + id,
	}
	ui.playVideo(v, false)
}

func (ui *UI) toggleWatchLater(v yt.Video) {
	added, err := ui.store.ToggleWatchLater(v)
	if err != nil {
		ui.state.Status = "Error updating watch later: " + err.Error()
		return
	}

	if added {
		ui.state.Status = "Added to Watch later: " + v.Title
	} else {
		ui.state.Status = "Removed from Watch later: " + v.Title
	}

	if ui.state.CurrentTab == TabWatchLater {
		ui.state.SetVideos(ui.store.WatchLater())
	}
}

func (ui *UI) requestThumbnail(vid string) {
	if vid == "" {
		return
	}
	if _, ok := ui.thumbs.Get(vid); ok {
		return
	}

	ui.requestedMu.Lock()
	if ui.requested[vid] {
		ui.requestedMu.Unlock()
		return
	}
	ui.requested[vid] = true
	ui.requestedMu.Unlock()

	ui.thumbs.Load(vid, func(img image.Image, err error) {
		if err != nil || img == nil {
			// Keep the id marked so a failed thumbnail is not fetched again on every frame.
			return
		}
		// Clear the mark so the id can be fetched again after LRU eviction.
		ui.requestedMu.Lock()
		delete(ui.requested, vid)
		ui.requestedMu.Unlock()
		ui.window.Invalidate()
	})
}

func (ui *UI) layout(gtx layout.Context) layout.Dimensions {
	if ui.pendingFocusTag != nil {
		gtx.Execute(key.FocusCmd{Tag: ui.pendingFocusTag})
		ui.pendingFocusTag = nil
	}

	// Register clipboard paste target.
	event.Op(gtx.Ops, ui.pasteTag)
	for {
		evt, ok := gtx.Event(transfer.TargetFilter{Target: ui.pasteTag, Type: "application/text"})
		if !ok {
			break
		}
		if de, ok := evt.(transfer.DataEvent); ok {
			r := de.Open()
			if r != nil {
				data, _ := io.ReadAll(r)
				_ = r.Close()
				ui.handlePastedText(string(data))
			}
		}
	}

	// Register global key shortcuts.
	event.Op(gtx.Ops, ui.windowTag)
	for {
		evt, ok := gtx.Event(
			key.Filter{Name: "1", Required: key.ModCtrl},
			key.Filter{Name: "2", Required: key.ModCtrl},
			key.Filter{Name: "3", Required: key.ModCtrl},
			key.Filter{Name: "4", Required: key.ModCtrl},
			key.Filter{Name: "L", Required: key.ModCtrl},
			key.Filter{Name: "l", Required: key.ModCtrl},
			key.Filter{Name: "V", Required: key.ModCtrl},
			key.Filter{Name: "v", Required: key.ModCtrl},
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NameLeftArrow},
			key.Filter{Name: key.NameRightArrow},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: "A"},
			key.Filter{Name: "a"},
			key.Filter{Name: "W"},
			key.Filter{Name: "w"},
			key.Filter{Name: " "},
			key.Filter{Name: "K"},
			key.Filter{Name: "k"},
			key.Filter{Name: "J"},
			key.Filter{Name: "j"},
			key.Filter{Name: "L"},
			key.Filter{Name: "l"},
			key.Filter{Name: "M"},
			key.Filter{Name: "m"},
			key.Filter{Name: "F"},
			key.Filter{Name: "f"},
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "O"},
			key.Filter{Name: "o"},
		)
		if !ok {
			break
		}

		if ke, ok := evt.(key.Event); ok && ke.State == key.Press {
			isEditorFocused := gtx.Focused(&ui.searchEditor) || gtx.Focused(&ui.addChannelEditor) || gtx.Focused(&ui.csvPathEditor)
			if ui.state.CurrentTab == TabWatch {
				action, tab, delta := ResolveWatchKeyAction(ke.Name, ke.Modifiers, isEditorFocused)
				switch action {
				case ActionSwitchTab:
					ui.closeWatchPage()
					ui.switchTab(tab)
				case ActionFocusSearch:
					gtx.Execute(key.FocusCmd{Tag: &ui.searchEditor})
				case ActionPasteURL:
					gtx.Execute(clipboard.ReadCmd{Tag: ui.pasteTag})
				case ActionWatchTogglePause:
					if ui.embedPlayer != nil {
						ui.embedPlayer.TogglePause()
					}
				case ActionWatchSeek:
					if ui.embedPlayer != nil {
						ui.embedPlayer.Seek(float64(delta), true)
					}
				case ActionWatchToggleMute:
					if ui.embedPlayer != nil {
						ui.embedPlayer.ToggleMute()
					}
				case ActionWatchToggleFullscreen:
					ui.toggleFullscreen()
				case ActionWatchBack:
					if ui.watchFullscreen {
						ui.setFullscreen(false)
					} else {
						ui.closeWatchPage()
					}
				case ActionWatchExternalMPV:
					vid := ui.watchVideo
					ui.closeWatchPage()
					ui.playVideoExternal(vid, false)
				}
			} else {
				action, tab, delta := ResolveKeyAction(ke.Name, ke.Modifiers, isEditorFocused)
				switch action {
				case ActionSwitchTab:
					ui.switchTab(tab)
				case ActionFocusSearch:
					gtx.Execute(key.FocusCmd{Tag: &ui.searchEditor})
				case ActionPasteURL:
					gtx.Execute(clipboard.ReadCmd{Tag: ui.pasteTag})
				case ActionMoveSelection:
					ui.state.MoveSelection(delta)
				case ActionPlay:
					ui.playSelected(false)
				case ActionPlayAudio:
					ui.playSelected(true)
				case ActionToggleWatchLater:
					if v, ok := ui.state.SelectedVideo(); ok {
						ui.toggleWatchLater(v)
					}
				}
			}
		}
	}

	// Paint bg itself (opaque), so it does not depend on compositor background.
	paint.FillShape(gtx.Ops, ui.colors.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	if ui.state.CurrentTab == TabWatch {
		if ui.watchFullscreen {
			return ui.layoutWatchPage(gtx)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(ui.layoutTopBar),
			layout.Flexed(1, ui.layoutWatchPage),
			layout.Rigid(ui.layoutStatusBar),
		)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(ui.layoutTopBar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(ui.layoutMiniGuide),
				layout.Flexed(1, ui.layoutMainArea),
			)
		}),
		layout.Rigid(ui.layoutStatusBar),
	)
}

// layoutTopBar renders the 56dp high top bar matching DESIGN.md:
// Left: Logo (28x20 red rounded rect with white triangle + 20sp medium "lightTube")
// Center: Pill search box (centered in window width, max width 640, height 40, radius 20, 1dp border, magnifier button)
// Right: Policy chip (surface bg, text2, 12sp)
func (ui *UI) layoutTopBar(gtx layout.Context) layout.Dimensions {
	// Check search editor submit.
	for {
		ev, ok := ui.searchEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			ui.handleSearchSubmit()
		}
	}
	if ui.searchBtn.Clicked(gtx) {
		ui.handleSearchSubmit()
	}
	if ui.searchClearClick.Clicked(gtx) {
		ui.searchEditor.SetText("")
	}
	if ui.logoClick.Clicked(gtx) {
		if ui.state.CurrentTab == TabWatch {
			ui.closeWatchPage()
		} else {
			ui.switchTab(TabHome)
		}
	}

	winW := gtx.Constraints.Max.X
	h := gtx.Dp(40)
	showChip := winW >= gtx.Dp(760)

	return layout.Inset{
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			// 1. Logo rigid
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.logoClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if ui.logoClick.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							w := gtx.Dp(28)
							lh := gtx.Dp(20)
							rect := image.Rect(0, 0, w, lh)
							rrect := clip.UniformRRect(rect, gtx.Dp(6))
							paint.FillShape(gtx.Ops, ui.colors.Red, rrect.Op(gtx.Ops))
							drawPlayTriangle(gtx, rect, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
							return layout.Dimensions{Size: image.Pt(w, lh)}
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.th, unit.Sp(20), "lightTube")
							lbl.Color = ui.colors.Text
							lbl.Font.Weight = font.Medium
							return lbl.Layout(gtx)
						}),
					)
				})
			}),

			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

			// 2. Search flexed (min 200, max 640, centered in free space)
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				freeW := gtx.Constraints.Max.X
				minW := gtx.Dp(200)
				maxW := gtx.Dp(640)
				pillW := maxW
				if freeW < pillW {
					pillW = freeW
				}
				if pillW < minW {
					pillW = minW
				}
				if pillW > freeW {
					pillW = freeW
				}

				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.layoutSearchBox(gtx, pillW, h)
				})
			}),

			// 3. Policy chip rigid (hidden when window < 760 dp)
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !showChip {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, ui.layoutPolicyLabel)
			}),
		)
	})
}

func (ui *UI) layoutSearchBox(gtx layout.Context, pillW, h int) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(pillW, h)
	gtx.Constraints.Max = image.Pt(pillW, h)

	radius := gtx.Dp(20)
	btnW := gtx.Dp(64)
	inputW := pillW - btnW
	if inputW < gtx.Dp(100) {
		inputW = gtx.Dp(100)
	}

	// 1. Input part: left side
	inputRect := image.Rect(0, 0, inputW, h)
	inputRRect := clip.RRect{
		Rect: inputRect,
		NW:   radius,
		SW:   radius,
	}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xff}, inputRRect.Op(gtx.Ops))

	borderColor := ui.colors.Border
	if gtx.Focused(&ui.searchEditor) {
		borderColor = ui.colors.BorderFocused
	}
	paint.FillShape(gtx.Ops, borderColor, clip.Stroke{
		Path:  inputRRect.Path(gtx.Ops),
		Width: float32(gtx.Dp(1)),
	}.Op())

	// 2. Button part: right side
	btnRect := image.Rect(inputW, 0, pillW, h)
	btnRRect := clip.RRect{
		Rect: btnRect,
		NE:   radius,
		SE:   radius,
	}
	btnBg := ui.colors.Surface
	if ui.searchBtn.Hovered() {
		btnBg = ui.colors.SurfaceHover
	}
	paint.FillShape(gtx.Ops, btnBg, btnRRect.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{
		Path:  btnRRect.Path(gtx.Ops),
		Width: float32(gtx.Dp(1)),
	}.Op())

	drawMagnifier(gtx, btnRect, ui.colors.Text)

	// Button click registration
	btnArea := clip.Rect(btnRect).Push(gtx.Ops)
	ui.searchBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if ui.searchBtn.Hovered() {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return layout.Dimensions{Size: btnRect.Size()}
	})
	btnArea.Pop()

	// Input content
	inputArea := clip.Rect(inputRect).Push(gtx.Ops)
	hasText := ui.searchEditor.Text() != ""

	inputGtx := gtx
	inputGtx.Constraints = layout.Exact(image.Pt(inputW, h))
	layout.Center.Layout(inputGtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		gtx.Constraints.Max.Y = h
		return layout.Inset{
			Left:  unit.Dp(16),
			Right: unit.Dp(12),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
			}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(ui.th, &ui.searchEditor, "Search")
					ed.Color = ui.colors.Text
					ed.HintColor = ui.colors.Text2
					ed.TextSize = unit.Sp(16)
					return ed.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasText {
						return layout.Dimensions{}
					}
					return ui.searchClearClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if ui.searchClearClick.Hovered() {
							pointer.CursorPointer.Add(gtx.Ops)
						}
						sz := gtx.Dp(20)
						drawClearXIcon(gtx, image.Rect(0, 0, sz, sz), ui.colors.Text2)
						return layout.Dimensions{Size: image.Pt(sz, sz)}
					})
				}),
			)
		})
	})
	inputArea.Pop()

	return layout.Dimensions{Size: image.Pt(pillW, h)}
}

func (ui *UI) layoutPolicyLabel(gtx layout.Context) layout.Dimensions {
	policyText := FormatPolicyLabel(ui.power, ui.mode)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
			paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(6),
				Bottom: unit.Dp(6),
				Left:   unit.Dp(12),
				Right:  unit.Dp(12),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.th, unit.Sp(12), policyText)
				lbl.Color = ui.colors.Text2
				return lbl.Layout(gtx)
			})
		},
	)
}

// layoutMiniGuide renders the 72dp wide sidebar with 4 items:
// Home, Subscriptions, Watch later, History.
// Vector icons: house, stacked rectangles, clock, counter-clockwise arrow.
func (ui *UI) layoutMiniGuide(gtx layout.Context) layout.Dimensions {
	guideWidth := gtx.Dp(72)
	gtx.Constraints.Min.X = guideWidth
	gtx.Constraints.Max.X = guideWidth

	items := []struct {
		tab   Tab
		label string
		icon  func(layout.Context, image.Rectangle, color.NRGBA)
	}{
		{TabHome, "Home", drawHouseIcon},
		{TabSubscriptions, "Subs", drawStackedRectsIcon},
		{TabWatchLater, "Later", drawClockIcon},
		{TabHistory, "History", drawHistoryIcon},
	}

	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var children []layout.FlexChild
				for i := range items {
					idx := i
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						item := items[idx]
						if ui.guideClicks[idx].Clicked(gtx) {
							ui.switchTab(item.tab)
						}

						isSelected := ui.state.CurrentTab == item.tab
						isHovered := ui.guideClicks[idx].Hovered()
						if isHovered {
							pointer.CursorPointer.Add(gtx.Ops)
						}

						btnW := gtx.Dp(64)
						btnH := gtx.Dp(60)
						gtx.Constraints.Min = image.Pt(btnW, btnH)
						gtx.Constraints.Max = image.Pt(btnW, btnH)

						return ui.guideClicks[idx].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if isSelected {
								r := clip.UniformRRect(image.Rect(0, 0, btnW, btnH), gtx.Dp(10))
								paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
							} else if isHovered {
								r := clip.UniformRRect(image.Rect(0, 0, btnW, btnH), gtx.Dp(10))
								paint.FillShape(gtx.Ops, ui.colors.SurfaceHover, r.Op(gtx.Ops))
							}

							iconColor := ui.colors.Text2
							labelColor := ui.colors.Text2
							if isSelected {
								iconColor = ui.colors.Text
								labelColor = ui.colors.Text
							}

							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{
									Axis:      layout.Vertical,
									Alignment: layout.Middle,
								}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										iconBox := image.Rect(0, 0, gtx.Dp(24), gtx.Dp(24))
										item.icon(gtx, iconBox, iconColor)
										return layout.Dimensions{Size: iconBox.Size()}
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(ui.th, unit.Sp(10), item.label)
										lbl.Color = labelColor
										if isSelected {
											lbl.Font.Weight = font.Medium
										}
										return lbl.Layout(gtx)
									}),
								)
							})
						})
					}))
					if idx < len(items)-1 {
						children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout))
					}
				}
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
			}),
		)
	})
}

func (ui *UI) layoutMainArea(gtx layout.Context) layout.Dimensions {
	switch ui.state.CurrentTab {
	case TabSearch:
		return ui.layoutSearchList(gtx)
	case TabSubscriptions:
		return ui.layoutSubscriptionsView(gtx)
	case TabHome:
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(ui.layoutChipsRow),
			layout.Flexed(1, ui.layoutCardGrid),
		)
	case TabWatchLater, TabHistory:
		return ui.layoutCardGrid(gtx)
	default:
		return ui.layoutCardGrid(gtx)
	}
}

// layoutChipsRow renders channel filter chips on Home:
// "All" plus one chip per subscribed channel.
func (ui *UI) layoutChipsRow(gtx layout.Context) layout.Dimensions {
	subs := ui.state.Subscriptions
	totalChips := 1 + len(subs)

	for len(ui.channelChipClicks) < len(subs) {
		ui.channelChipClicks = append(ui.channelChipClicks, widget.Clickable{})
	}

	return layout.Inset{
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
		Top:    unit.Dp(4),
		Bottom: unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return ui.chipsList.Layout(gtx, totalChips, func(gtx layout.Context, i int) layout.Dimensions {
			var title string
			var isSelected bool
			var click *widget.Clickable

			if i == 0 {
				title = "All"
				isSelected = ui.state.ActiveChannelFilter == ""
				click = &ui.allChipClick
			} else {
				sub := subs[i-1]
				title = sub.Title
				if title == "" {
					title = sub.ChannelID
				}
				isSelected = ui.state.ActiveChannelFilter == sub.ChannelID
				click = &ui.channelChipClicks[i-1]
			}

			if click.Clicked(gtx) {
				if i == 0 {
					ui.state.ActiveChannelFilter = ""
				} else {
					ui.state.ActiveChannelFilter = subs[i-1].ChannelID
				}
				ui.state.SelectedIndex = 0
			}

			if click.Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}

			chipBg := ui.colors.Surface
			chipFg := ui.colors.Text
			if isSelected {
				chipBg = ui.colors.ChipActiveBg
				chipFg = ui.colors.ChipActiveText
			} else if click.Hovered() {
				chipBg = ui.colors.SurfaceHover
			}

			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
							paint.FillShape(gtx.Ops, chipBg, r.Op(gtx.Ops))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{
								Top:    unit.Dp(6),
								Bottom: unit.Dp(6),
								Left:   unit.Dp(12),
								Right:  unit.Dp(12),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.th, unit.Sp(13), title)
								lbl.Color = chipFg
								if isSelected {
									lbl.Font.Weight = font.Medium
								}
								return lbl.Layout(gtx)
							})
						},
					)
				})
			})
		})
	})
}

// layoutCardGrid implements the YouTube dark card grid from DESIGN.md:
// Columns = max(1, floor((width - 32) / (320 + 16))). Card width fills the column.
// Gap 16 horizontal, 40 vertical. Use one layout.List of rows so only visible rows are laid out.
func (ui *UI) layoutCardGrid(gtx layout.Context) layout.Dimensions {
	videos := ui.state.FilteredVideos()
	if len(videos) == 0 {
		return ui.layoutEmptyState(gtx)
	}

	availW := gtx.Constraints.Max.X - gtx.Dp(32)
	colW := gtx.Dp(320)
	gapX := gtx.Dp(16)
	columns := (availW + gapX) / (colW + gapX)
	if columns < 1 {
		columns = 1
	}

	for len(ui.cardClicks) < len(videos) {
		ui.cardClicks = append(ui.cardClicks, widget.Clickable{})
	}
	for len(ui.cardClockClicks) < len(videos) {
		ui.cardClockClicks = append(ui.cardClockClicks, widget.Clickable{})
	}

	numRows := (len(videos) + columns - 1) / columns

	// Thumbnail prefetching: only visible rows plus 10 margin.
	firstRow := ui.gridList.Position.First
	countRows := ui.gridList.Position.Count
	if countRows == 0 {
		countRows = 4
	}
	startIdx := (firstRow - 10) * columns
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := (firstRow + countRows + 10) * columns
	if endIdx > len(videos) {
		endIdx = len(videos)
	}
	for i := startIdx; i < endIdx; i++ {
		ui.requestThumbnail(videos[i].ID)
	}

	cardWidthPx := (availW - (columns-1)*gapX) / columns
	thumbHeightPx := cardWidthPx * 9 / 16

	return layout.Inset{
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
		Top:    unit.Dp(8),
		Bottom: unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return ui.gridList.Layout(gtx, numRows, func(gtx layout.Context, rowIdx int) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(36)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					ui.layoutGridCards(gtx, rowIdx, columns, cardWidthPx, thumbHeightPx, videos)...,
				)
			})
		})
	})
}

func (ui *UI) layoutGridCards(gtx layout.Context, rowIdx, columns, cardWidthPx, thumbHeightPx int, videos []yt.Video) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, columns*2)

	for col := 0; col < columns; col++ {
		idx := rowIdx*columns + col
		if idx >= len(videos) {
			// Spacer to fill the remainder of the flex row
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(cardWidthPx, thumbHeightPx+gtx.Dp(72))}
			}))
		} else {
			cardIdx := idx
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Pt(cardWidthPx, 0)
				gtx.Constraints.Max = image.Pt(cardWidthPx, gtx.Constraints.Max.Y)
				return ui.layoutCard(gtx, cardIdx, cardWidthPx, thumbHeightPx, videos[cardIdx])
			}))
		}

		if col < columns-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout))
		}
	}

	return children
}

func (ui *UI) layoutCard(gtx layout.Context, index, cardWidthPx, thumbHeightPx int, v yt.Video) layout.Dimensions {
	click := &ui.cardClicks[index]
	clockClick := &ui.cardClockClicks[index]

	// Single click plays!
	if click.Clicked(gtx) {
		ui.state.SelectedIndex = index
		ui.playVideo(v, false)
	}

	if clockClick.Clicked(gtx) {
		ui.toggleWatchLater(v)
	}

	isSelected := index == ui.state.SelectedIndex
	isHovered := click.Hovered()
	if isHovered {
		pointer.CursorPointer.Add(gtx.Ops)
	}

	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				// Hover sets a surface background behind the whole card with radius 12.
				if isHovered {
					r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
					paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				pad := unit.Dp(0)
				if isHovered {
					pad = unit.Dp(6)
				}
				return layout.Inset{
					Top:    pad,
					Bottom: pad,
					Left:   pad,
					Right:  pad,
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// Thumbnail: 16:9, radius 12
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutCardThumbnail(gtx, v, cardWidthPx-gtx.Dp(pad*2), thumbHeightPx-gtx.Dp(pad*2), isSelected, isHovered, clockClick)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
						// Below thumbnail: 36dp avatar + title + channel + meta
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
								// 36dp circle avatar
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layoutAvatar(gtx, ui.th, v.ChannelID, v.Channel, 36)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								// Text metadata
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											title := material.Label(ui.th, unit.Sp(16), v.Title)
											title.Font.Weight = font.Medium
											title.Color = ui.colors.Text
											title.MaxLines = 2
											title.Truncator = "…"
											return title.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											ch := material.Label(ui.th, unit.Sp(14), v.Channel)
											ch.Color = ui.colors.Text2
											ch.MaxLines = 1
											ch.Truncator = "…"
											return ch.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											metaText := FormatMetaLine(v.ViewCount, v.UploadDate, v.PublishedAt, time.Now())
											meta := material.Label(ui.th, unit.Sp(14), metaText)
											meta.Color = ui.colors.Text2
											meta.MaxLines = 1
											meta.Truncator = "…"
											return meta.Layout(gtx)
										}),
									)
								}),
							)
						}),
					)
				})
			},
		)
	})
}

func (ui *UI) layoutCardThumbnail(gtx layout.Context, v yt.Video, w, h int, isSelected, isHovered bool, clockClick *widget.Clickable) layout.Dimensions {
	boxSize := image.Pt(w, h)
	gtx.Constraints.Min = boxSize
	gtx.Constraints.Max = boxSize

	var imgOp paint.ImageOp
	hasImg := false
	if img, ok := ui.thumbs.Get(v.ID); ok && img != nil {
		op, cached := ui.imageOps[v.ID]
		if !cached {
			op = paint.NewImageOp(img)
			ui.imageOps[v.ID] = op
		}
		imgOp = op
		hasImg = true
	}

	rrect := clip.UniformRRect(image.Rectangle{Max: boxSize}, gtx.Dp(12))

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, ui.colors.Surface, rrect.Op(gtx.Ops))
			return layout.Dimensions{Size: boxSize}
		},
		func(gtx layout.Context) layout.Dimensions {
			if hasImg {
				defer rrect.Push(gtx.Ops).Pop()
				widget.Image{
					Src: imgOp,
					Fit: widget.Cover,
				}.Layout(gtx)
			}

			// Selected item outline: 2 dp text outline around the thumbnail
			if isSelected {
				paint.FillShape(gtx.Ops, ui.colors.Text, clip.Stroke{
					Path:  rrect.Path(gtx.Ops),
					Width: float32(gtx.Dp(2)),
				}.Op())
			}

			// Hover watch later clock button at top-right
			if isHovered && clockClick != nil {
				layout.NE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Top:   unit.Dp(6),
						Right: unit.Dp(6),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						btnSize := gtx.Dp(28)
						gtx.Constraints.Min = image.Pt(btnSize, btnSize)
						gtx.Constraints.Max = image.Pt(btnSize, btnSize)

						return clockClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if clockClick.Hovered() {
								pointer.CursorPointer.Add(gtx.Ops)
							}
							clockR := clip.UniformRRect(image.Rect(0, 0, btnSize, btnSize), gtx.Dp(4))
							paint.FillShape(gtx.Ops, ui.colors.BadgeBg, clockR.Op(gtx.Ops))
							drawClockIcon(gtx, image.Rect(0, 0, btnSize, btnSize), ui.colors.Text)
							return layout.Dimensions{Size: image.Pt(btnSize, btnSize)}
						})
					})
				})
			}

			// Duration badge: bottom-right, 4 dp inset, radius 4, badgeBg, white 12 sp medium
			durStr := yt.FormatDuration(v.Duration)
			if durStr != "" && durStr != "00:00" {
				return layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Bottom: unit.Dp(4),
						Right:  unit.Dp(4),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(4))
								paint.FillShape(gtx.Ops, ui.colors.BadgeBg, r.Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{
									Top:    unit.Dp(2),
									Bottom: unit.Dp(2),
									Left:   unit.Dp(5),
									Right:  unit.Dp(5),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.th, unit.Sp(12), durStr)
									lbl.Color = ui.colors.BadgeFg
									lbl.Font.Weight = font.Medium
									return lbl.Layout(gtx)
								})
							},
						)
					})
				})
			}

			return layout.Dimensions{Size: boxSize}
		},
	)
}

// layoutSearchList renders YouTube search results as a list:
// One row per result: thumbnail 360 wide (16:9, radius 12, duration badge) on left;
// on right: title 18 sp medium max 2 lines; meta line "1.2M views · 3 days ago" 12 sp text2;
// channel row (24 dp avatar circle + channel name 12 sp text2). Row gap 16.
func (ui *UI) layoutSearchList(gtx layout.Context) layout.Dimensions {
	videos := ui.state.Videos
	if len(videos) == 0 {
		return ui.layoutEmptyState(gtx)
	}

	for len(ui.searchItemClicks) < len(videos) {
		ui.searchItemClicks = append(ui.searchItemClicks, widget.Clickable{})
	}
	for len(ui.searchClockClicks) < len(videos) {
		ui.searchClockClicks = append(ui.searchClockClicks, widget.Clickable{})
	}

	// Request thumbnails for visible rows plus 10 margin.
	first := ui.searchList.Position.First
	count := ui.searchList.Position.Count
	if count == 0 {
		count = 6
	}
	start := first - 10
	if start < 0 {
		start = 0
	}
	end := first + count + 10
	if end > len(videos) {
		end = len(videos)
	}
	for i := start; i < end; i++ {
		ui.requestThumbnail(videos[i].ID)
	}

	return layout.Inset{
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
		Top:    unit.Dp(16),
		Bottom: unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return ui.searchList.Layout(gtx, len(videos), func(gtx layout.Context, i int) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.layoutSearchRow(gtx, i, videos[i])
			})
		})
	})
}

func (ui *UI) layoutSearchRow(gtx layout.Context, index int, v yt.Video) layout.Dimensions {
	click := &ui.searchItemClicks[index]
	clockClick := &ui.searchClockClicks[index]

	if click.Clicked(gtx) {
		ui.state.SelectedIndex = index
		ui.playVideo(v, false)
	}
	if clockClick.Clicked(gtx) {
		ui.toggleWatchLater(v)
	}

	isSelected := index == ui.state.SelectedIndex
	isHovered := click.Hovered()
	if isHovered {
		pointer.CursorPointer.Add(gtx.Ops)
	}

	thumbW := gtx.Dp(360)
	thumbH := thumbW * 9 / 16

	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				if isHovered {
					r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
					paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				pad := unit.Dp(0)
				if isHovered {
					pad = unit.Dp(6)
				}
				return layout.Inset{
					Top:    pad,
					Bottom: pad,
					Left:   pad,
					Right:  pad,
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
						// Thumbnail 360 wide, 16:9
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutCardThumbnail(gtx, v, thumbW-gtx.Dp(pad*2), thumbH-gtx.Dp(pad*2), isSelected, isHovered, clockClick)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
						// Metadata column
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								// Title: 18 sp medium max 2 lines
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									title := material.Label(ui.th, unit.Sp(18), v.Title)
									title.Color = ui.colors.Text
									title.Font.Weight = font.Medium
									title.MaxLines = 2
									title.Truncator = "…"
									return title.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
								// Meta line: "1.2M views · 3 days ago", 12 sp text2
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									metaText := FormatMetaLine(v.ViewCount, v.UploadDate, v.PublishedAt, time.Now())
									meta := material.Label(ui.th, unit.Sp(12), metaText)
									meta.Color = ui.colors.Text2
									return meta.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
								// Channel row: 24 dp avatar circle + channel name 12 sp text2
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return layoutAvatar(gtx, ui.th, v.ChannelID, v.Channel, 24)
										}),
										layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(ui.th, unit.Sp(12), v.Channel)
											lbl.Color = ui.colors.Text2
											return lbl.Layout(gtx)
										}),
									)
								}),
							)
						}),
					)
				})
			},
		)
	})
}

// layoutSubscriptionsView renders channel management view from DESIGN.md:
// - Text field "Add channel: URL, @handle or UC id" + "Subscribe" button (red bg, radius 18)
// - Text field for Takeout CSV path + "Import" button (surface bg)
// - List of channels: avatar circle, name, and "Subscribed" chip that unsubscribes on click
func (ui *UI) layoutSubscriptionsView(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := ui.addChannelEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			ui.addSubscription(ui.addChannelEditor.Text())
		}
	}
	if ui.addChannelBtn.Clicked(gtx) {
		ui.addSubscription(ui.addChannelEditor.Text())
	}

	for {
		ev, ok := ui.csvPathEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			ui.importTakeout(ui.csvPathEditor.Text())
		}
	}
	if ui.importCSVBtn.Clicked(gtx) {
		ui.importTakeout(ui.csvPathEditor.Text())
	}

	subs := ui.state.Subscriptions
	for len(ui.unsubscribeClicks) < len(subs) {
		ui.unsubscribeClicks = append(ui.unsubscribeClicks, widget.Clickable{})
	}

	return layout.Inset{
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
		Top:    unit.Dp(16),
		Bottom: unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Row 1: Add Channel
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						w := gtx.Dp(400)
						gtx.Constraints.Min.X = w
						gtx.Constraints.Max.X = w
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(18))
								paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{
									Top:    unit.Dp(8),
									Bottom: unit.Dp(8),
									Left:   unit.Dp(14),
									Right:  unit.Dp(14),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									ed := material.Editor(ui.th, &ui.addChannelEditor, "Add channel: URL, @handle or UC id")
									ed.Color = ui.colors.Text
									ed.HintColor = ui.colors.Text2
									ed.TextSize = unit.Sp(13)
									return ed.Layout(gtx)
								})
							},
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(ui.th, &ui.addChannelBtn, "Subscribe")
						btn.Background = ui.colors.Red
						btn.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
						btn.CornerRadius = unit.Dp(18)
						btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

			// Row 2: Import Takeout CSV
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						w := gtx.Dp(400)
						gtx.Constraints.Min.X = w
						gtx.Constraints.Max.X = w
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
								paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{
									Top:    unit.Dp(8),
									Bottom: unit.Dp(8),
									Left:   unit.Dp(14),
									Right:  unit.Dp(14),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									ed := material.Editor(ui.th, &ui.csvPathEditor, "Path to subscriptions.csv…")
									ed.Color = ui.colors.Text
									ed.HintColor = ui.colors.Text2
									ed.TextSize = unit.Sp(13)
									return ed.Layout(gtx)
								})
							},
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(ui.th, &ui.importCSVBtn, "Import")
						btn.Background = ui.colors.Surface
						btn.Color = ui.colors.Text
						btn.CornerRadius = unit.Dp(8)
						btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

			// Channels header
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.th, unit.Sp(18), fmt.Sprintf("Subscriptions (%d)", len(subs)))
				lbl.Font.Weight = font.Bold
				lbl.Color = ui.colors.Text
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

			// Channel list
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(subs) == 0 {
					lbl := material.Label(ui.th, unit.Sp(14), "No channels subscribed yet.")
					lbl.Color = ui.colors.Text2
					return lbl.Layout(gtx)
				}
				return ui.subList.Layout(gtx, len(subs), func(gtx layout.Context, i int) layout.Dimensions {
					sub := subs[i]
					click := &ui.unsubscribeClicks[i]
					if click.Clicked(gtx) {
						ui.removeSubscription(sub.ChannelID)
					}

					return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							// Avatar circle
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutAvatar(gtx, ui.th, sub.ChannelID, sub.Title, 36)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							// Channel title
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								title := sub.Title
								if title == "" {
									title = sub.ChannelID
								}
								lbl := material.Label(ui.th, unit.Sp(15), title)
								lbl.Font.Weight = font.Medium
								lbl.Color = ui.colors.Text
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							// Subscribed chip (unsubscribes on click)
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									if click.Hovered() {
										pointer.CursorPointer.Add(gtx.Ops)
									}
									chipBg := ui.colors.Surface
									if click.Hovered() {
										chipBg = ui.colors.SurfaceHover
									}
									return layout.Background{}.Layout(gtx,
										func(gtx layout.Context) layout.Dimensions {
											r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(16))
											paint.FillShape(gtx.Ops, chipBg, r.Op(gtx.Ops))
											return layout.Dimensions{Size: gtx.Constraints.Min}
										},
										func(gtx layout.Context) layout.Dimensions {
											return layout.Inset{
												Top:    unit.Dp(6),
												Bottom: unit.Dp(6),
												Left:   unit.Dp(14),
												Right:  unit.Dp(14),
											}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												lbl := material.Label(ui.th, unit.Sp(13), "Subscribed")
												lbl.Color = ui.colors.Text
												lbl.Font.Weight = font.Medium
												return lbl.Layout(gtx)
											})
										},
									)
								})
							}),
						)
					})
				})
			}),
		)
	})
}

func (ui *UI) layoutEmptyState(gtx layout.Context) layout.Dimensions {
	msg := "No items to display"
	switch ui.state.CurrentTab {
	case TabHome:
		msg = "No subscription videos yet. Add channels in Subscriptions tab or import CSV."
	case TabSearch:
		msg = "Search for YouTube videos above."
	case TabSubscriptions:
		msg = "No subscriptions yet. Add channels above or import CSV."
	case TabWatchLater:
		msg = "Watch later list is empty. Hover on a video or press 'W' to add it."
	case TabHistory:
		msg = "Playback history is empty."
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(ui.th, unit.Sp(14), msg)
		lbl.Color = ui.colors.Text2
		return lbl.Layout(gtx)
	})
}

// layoutStatusBar renders status line: only when there is a message, 28 high, text2.
func (ui *UI) layoutStatusBar(gtx layout.Context) layout.Dimensions {
	if ui.state.Status == "" {
		return layout.Dimensions{}
	}

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, ui.colors.Surface, clip.Rect{Max: gtx.Constraints.Min}.Op())
			// Top border line
			borderRect := image.Rectangle{
				Min: image.Pt(0, 0),
				Max: image.Pt(gtx.Constraints.Min.X, gtx.Dp(1)),
			}
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(borderRect).Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(4),
				Bottom: unit.Dp(4),
				Left:   unit.Dp(16),
				Right:  unit.Dp(16),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.th, unit.Sp(12), ui.state.Status)
				lbl.Color = ui.colors.Text2
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			})
		},
	)
}

func (ui *UI) isSubscribed(channelID string) bool {
	for _, sub := range ui.state.Subscriptions {
		if sub.ChannelID == channelID {
			return true
		}
	}
	return false
}

func (ui *UI) setFullscreen(fs bool) {
	if ui.watchFullscreen == fs {
		return
	}
	ui.watchFullscreen = fs
	if fs {
		ui.window.Option(app.Fullscreen.Option())
	} else {
		ui.window.Option(app.Windowed.Option())
	}
}

func (ui *UI) toggleFullscreen() {
	ui.setFullscreen(!ui.watchFullscreen)
}

func (ui *UI) openWatchPage(v yt.Video) {
	ui.watchVideo = v
	if ui.state.CurrentTab != TabWatch {
		ui.previousTab = ui.state.CurrentTab
	}
	ui.state.CurrentTab = TabWatch

	// Up next: list = current feed or search results, minus this video
	upNext := make([]yt.Video, 0, len(ui.state.Videos))
	for _, item := range ui.state.Videos {
		if item.ID != v.ID {
			upNext = append(upNext, item)
		}
	}
	ui.upNextVideos = upNext

	_ = ui.store.AddHistory(v)
	ui.state.Status = ""

	if ui.waylandView.Valid() {
		if ui.embedPlayer == nil {
			p, err := embed.New(ui.waylandView.Display, ui.waylandView.Surface)
			if err != nil {
				ui.state.Status = "Failed to create embed player: " + err.Error()
				return
			}
			ui.embedPlayer = p
		}

		cookiePath := filepath.Join(ui.store.DataDir(), "cookies.txt")
		if _, err := os.Stat(cookiePath); err != nil {
			cookiePath = ""
		}

		scale := ui.currentScale
		if scale <= 0 {
			scale = 2
		}

		policy := player.Format(ui.power, player.ModeVideo)
		err := ui.embedPlayer.Play(embed.PlayOptions{
			URL:         v.WatchURL(),
			Title:       v.Title,
			YTDLFormat:  policy,
			CookiesFile: cookiePath,
			WidthPx:     1280,
			HeightPx:    720,
			Scale:       scale,
			Debug:       ui.debug,
		})
		if err != nil {
			ui.state.Status = "Playback error: " + err.Error()
			return
		}
	}

	ui.watchPaused = false
	ui.watchCurrentTime = 0
	ui.watchDuration = float64(v.Duration)
	ui.watchLastMouseMove = time.Now()
	ui.hwdecLogged = false
	ui.lastTimePoll = time.Now()
	ui.window.Invalidate()
}

func (ui *UI) closeWatchPage() {
	if ui.embedPlayer != nil {
		ui.embedPlayer.Stop()
	}
	if ui.watchFullscreen {
		ui.setFullscreen(false)
	}
	ui.state.CurrentTab = ui.previousTab
	if ui.state.CurrentTab == TabWatch {
		ui.state.CurrentTab = TabHome
	}
	ui.upNextVideos = nil
	ui.watchVideo = yt.Video{}
	if ui.thumbs != nil {
		ui.thumbs.Clear()
	}
	ui.imageOps = make(map[string]paint.ImageOp)
	runtime.GC()
	debug.FreeOSMemory()
	if ui.debug {
		fmt.Fprintf(os.Stderr, "watch page closed\n")
	}
	ui.window.Invalidate()
}

func (ui *UI) triggerDebugPlay(urlStr string, playSeconds int) {
	id, err := yt.ExtractVideoID(urlStr)
	v := yt.Video{
		ID:    id,
		Title: "Test Video",
		URL:   urlStr,
	}
	if err == nil {
		for _, vid := range ui.state.Videos {
			if vid.ID == id {
				v = vid
				break
			}
		}
	}

	ui.openWatchPage(v)
}

func (ui *UI) layoutWatchPage(gtx layout.Context) layout.Dimensions {
	scale := ui.currentScale
	if scale <= 0 {
		scale = 2
	}

	// 1. Fullscreen layout
	if ui.watchFullscreen {
		winW := gtx.Constraints.Max.X
		winH := gtx.Constraints.Max.Y
		showControls := time.Since(ui.watchLastMouseMove) < 3*time.Second

		controlsH := gtx.Dp(48)
		videoH := winH
		if showControls {
			videoH = winH - controlsH
		}

		if ui.embedPlayer != nil {
			ui.embedPlayer.SetRect(image.Rect(0, 0, winW, videoH), scale)
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.layoutVideoArea(gtx, winW, videoH)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !showControls {
					return layout.Dimensions{}
				}
				return ui.layoutControlsBar(gtx, winW)
			}),
		)
	}

	// 2. Windowed layout
	availW := gtx.Constraints.Max.X
	isWide := availW >= gtx.Dp(1000)

	upNextW := gtx.Dp(402)
	var colW int
	if isWide {
		colW = availW - gtx.Dp(24) - upNextW - gtx.Dp(32)
	} else {
		colW = availW - gtx.Dp(32)
	}

	videoW := colW
	if videoW < gtx.Dp(200) {
		videoW = gtx.Dp(200)
	}
	videoH := videoW * 9 / 16

	// Video rect in window DIPs:
	// Inset: Left 16dp, Top 56dp (top bar) + 16dp = 72dp
	videoX := gtx.Dp(16)
	videoY := gtx.Dp(56) + gtx.Dp(16)

	if ui.embedPlayer != nil {
		ui.embedPlayer.SetRect(image.Rect(videoX, videoY, videoX+videoW, videoY+videoH), scale)
	}

	return layout.Inset{
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
		Top:    unit.Dp(16),
		Bottom: unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if isWide {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				// Left video column
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = videoW
					gtx.Constraints.Max.X = videoW
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// Video area
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutVideoArea(gtx, videoW, videoH)
						}),
						// Controls bar below video
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutControlsBar(gtx, videoW)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						// Video details (title, channel, description)
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutVideoDetails(gtx, ui.watchVideo, videoW)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
				// Right Up next column
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutUpNextList(gtx, upNextW)
				}),
			)
		}

		// Narrow window (< 1000): Up next goes below description
		return ui.watchList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutVideoArea(gtx, videoW, videoH)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutControlsBar(gtx, videoW)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutVideoDetails(gtx, ui.watchVideo, videoW)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutUpNextList(gtx, videoW)
				}),
			)
		})
	})
}

func (ui *UI) layoutVideoArea(gtx layout.Context, w, h int) layout.Dimensions {
	box := image.Pt(w, h)
	gtx.Constraints.Min = box
	gtx.Constraints.Max = box

	area := clip.Rect(image.Rect(0, 0, w, h)).Push(gtx.Ops)
	event.Op(gtx.Ops, &ui.watchVideoTag)
	area.Pop()

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.watchVideoTag,
			Kinds:  pointer.Press | pointer.Move,
		})
		if !ok {
			break
		}
		pe := ev.(pointer.Event)
		ui.watchLastMouseMove = time.Now()
		if pe.Kind == pointer.Press && pe.Buttons.Contain(pointer.ButtonPrimary) {
			now := time.Now()
			if now.Sub(ui.lastVideoClickTime) < 300*time.Millisecond {
				ui.toggleFullscreen()
				ui.lastVideoClickTime = time.Time{}
			} else {
				ui.lastVideoClickTime = now
				if ui.embedPlayer != nil {
					ui.embedPlayer.TogglePause()
				}
			}
		}
	}

	return layout.Dimensions{Size: box}
}

func (ui *UI) layoutControlsBar(gtx layout.Context, widthPx int) layout.Dimensions {
	barH := gtx.Dp(44)
	gtx.Constraints.Min = image.Pt(widthPx, barH)
	gtx.Constraints.Max = image.Pt(widthPx, barH)

	// Seek progress bar
	progressH := gtx.Dp(12)
	pArea := clip.Rect(image.Rect(0, 0, widthPx, progressH)).Push(gtx.Ops)
	event.Op(gtx.Ops, &ui.watchProgressTag)
	pArea.Pop()

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.watchProgressTag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release,
		})
		if !ok {
			break
		}
		pe := ev.(pointer.Event)
		ui.watchLastMouseMove = time.Now()
		if pe.Kind == pointer.Press || pe.Kind == pointer.Drag {
			frac := float64(pe.Position.X) / float64(widthPx)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			if ui.watchDuration > 0 && ui.embedPlayer != nil {
				targetTime := frac * ui.watchDuration
				ui.watchCurrentTime = targetTime
				ui.embedPlayer.Seek(targetTime, false)
			}
		}
	}

	// Render progress line (3dp)
	progY := gtx.Dp(4)
	progLineH := gtx.Dp(3)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff}, clip.Rect(image.Rect(0, progY, widthPx, progY+progLineH)).Op())

	frac := 0.0
	if ui.watchDuration > 0 {
		frac = ui.watchCurrentTime / ui.watchDuration
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
	}
	playedW := int(frac * float64(widthPx))
	if playedW > 0 {
		paint.FillShape(gtx.Ops, ui.colors.Red, clip.Rect(image.Rect(0, progY, playedW, progY+progLineH)).Op())
	}
	handleR := gtx.Dp(5)
	handleRect := image.Rect(playedW-handleR, progY+progLineH/2-handleR, playedW+handleR, progY+progLineH/2+handleR)
	paint.FillShape(gtx.Ops, ui.colors.Red, clip.Ellipse(handleRect).Op(gtx.Ops))

	if ui.watchPlayPauseClick.Clicked(gtx) {
		if ui.embedPlayer != nil {
			ui.embedPlayer.TogglePause()
		}
	}
	if ui.watchMuteClick.Clicked(gtx) {
		if ui.embedPlayer != nil {
			ui.embedPlayer.ToggleMute()
		}
	}
	if ui.watchFullscreenClick.Clicked(gtx) {
		ui.toggleFullscreen()
	}

	return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnSize := gtx.Dp(28)
						gtx.Constraints.Min = image.Pt(btnSize, btnSize)
						gtx.Constraints.Max = image.Pt(btnSize, btnSize)
						return ui.watchPlayPauseClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if ui.watchPlayPauseClick.Hovered() {
								pointer.CursorPointer.Add(gtx.Ops)
							}
							drawPlayPauseIcon(gtx, image.Rect(0, 0, btnSize, btnSize), ui.colors.Text, ui.watchPaused)
							return layout.Dimensions{Size: image.Pt(btnSize, btnSize)}
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnSize := gtx.Dp(28)
						gtx.Constraints.Min = image.Pt(btnSize, btnSize)
						gtx.Constraints.Max = image.Pt(btnSize, btnSize)
						return ui.watchMuteClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if ui.watchMuteClick.Hovered() {
								pointer.CursorPointer.Add(gtx.Ops)
							}
							drawSpeakerIcon(gtx, image.Rect(0, 0, btnSize, btnSize), ui.colors.Text, ui.watchMuted)
							return layout.Dimensions{Size: image.Pt(btnSize, btnSize)}
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						curStr := yt.FormatDuration(int(ui.watchCurrentTime))
						durStr := yt.FormatDuration(int(ui.watchDuration))
						timeStr := fmt.Sprintf("%s / %s", curStr, durStr)
						lbl := material.Label(ui.th, unit.Sp(13), timeStr)
						lbl.Color = ui.colors.Text
						return lbl.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btnSize := gtx.Dp(28)
				gtx.Constraints.Min = image.Pt(btnSize, btnSize)
				gtx.Constraints.Max = image.Pt(btnSize, btnSize)
				return ui.watchFullscreenClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if ui.watchFullscreenClick.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					drawFullscreenIcon(gtx, image.Rect(0, 0, btnSize, btnSize), ui.colors.Text, ui.watchFullscreen)
					return layout.Dimensions{Size: image.Pt(btnSize, btnSize)}
				})
			}),
		)
	})
}

func (ui *UI) layoutVideoDetails(gtx layout.Context, v yt.Video, widthPx int) layout.Dimensions {
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx

	if ui.watchSubscribeClick.Clicked(gtx) {
		if ui.isSubscribed(v.ChannelID) {
			ui.removeSubscription(v.ChannelID)
		} else {
			ui.addSubscription(v.ChannelID)
		}
	}
	if ui.watchLaterClick.Clicked(gtx) {
		ui.toggleWatchLater(v)
	}
	if ui.watchExternalMPVClick.Clicked(gtx) {
		vid := ui.watchVideo
		ui.closeWatchPage()
		ui.playVideoExternal(vid, false)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Title: 20 sp medium, max 2 lines
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.th, unit.Sp(20), v.Title)
			lbl.Font.Weight = font.Medium
			lbl.Color = ui.colors.Text
			lbl.MaxLines = 2
			lbl.Truncator = "…"
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

		// Channel row: (A) Channel name 16sp medium  [Subscribe]  [Watch later] [mpv]
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
			}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutAvatar(gtx, ui.th, v.ChannelID, v.Channel, 36)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.th, unit.Sp(16), v.Channel)
					lbl.Font.Weight = font.Medium
					lbl.Color = ui.colors.Text
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					isSub := ui.isSubscribed(v.ChannelID)
					label := "Subscribe"
					btnBg := ui.colors.Red
					btnFg := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
					if isSub {
						label = "Subscribed"
						btnBg = ui.colors.Surface
						btnFg = ui.colors.Text
					}
					btn := material.Button(ui.th, &ui.watchSubscribeClick, label)
					btn.Background = btnBg
					btn.Color = btnFg
					btn.CornerRadius = unit.Dp(18)
					btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(ui.th, &ui.watchLaterClick, "Watch later")
					btn.Background = ui.colors.Surface
					btn.Color = ui.colors.Text
					btn.CornerRadius = unit.Dp(18)
					btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(ui.th, &ui.watchExternalMPVClick, "mpv")
					btn.Background = ui.colors.Surface
					btn.Color = ui.colors.Text
					btn.CornerRadius = unit.Dp(18)
					btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}
					return btn.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Surface box radius 12: "1.2M views · 3 days ago" + description
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
					paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Top:    unit.Dp(12),
						Bottom: unit.Dp(12),
						Left:   unit.Dp(16),
						Right:  unit.Dp(16),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						metaText := FormatMetaLine(v.ViewCount, v.UploadDate, v.PublishedAt, time.Now())
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.th, unit.Sp(14), metaText)
								lbl.Font.Weight = font.Medium
								lbl.Color = ui.colors.Text
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if v.Description == "" {
									return layout.Dimensions{}
								}
								return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									desc := material.Label(ui.th, unit.Sp(13), v.Description)
									desc.Color = ui.colors.Text
									desc.MaxLines = 6
									desc.Truncator = "…"
									return desc.Layout(gtx)
								})
							}),
						)
					})
				},
			)
		}),
	)
}

func (ui *UI) layoutUpNextList(gtx layout.Context, widthPx int) layout.Dimensions {
	videos := ui.upNextVideos
	if len(videos) == 0 {
		return layout.Dimensions{}
	}

	for len(ui.upNextClicks) < len(videos) {
		ui.upNextClicks = append(ui.upNextClicks, widget.Clickable{})
	}

	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.th, unit.Sp(16), "Up next")
			lbl.Font.Weight = font.Bold
			lbl.Color = ui.colors.Text
			return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, lbl.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.upNextList.Layout(gtx, len(videos), func(gtx layout.Context, i int) layout.Dimensions {
				v := videos[i]
				click := &ui.upNextClicks[i]
				if click.Clicked(gtx) {
					ui.openWatchPage(v)
				}
				if click.Hovered() {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				thumbW := gtx.Dp(168)
				thumbH := thumbW * 9 / 16

				return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return ui.layoutCardThumbnail(gtx, v, thumbW, thumbH, false, click.Hovered(), nil)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(ui.th, unit.Sp(14), v.Title)
										lbl.Font.Weight = font.Medium
										lbl.Color = ui.colors.Text
										lbl.MaxLines = 2
										lbl.Truncator = "…"
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(ui.th, unit.Sp(12), v.Channel)
										lbl.Color = ui.colors.Text2
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										metaText := FormatMetaLine(v.ViewCount, v.UploadDate, v.PublishedAt, time.Now())
										lbl := material.Label(ui.th, unit.Sp(12), metaText)
										lbl.Color = ui.colors.Text2
										return lbl.Layout(gtx)
									}),
								)
							}),
						)
					})
				})
			})
		}),
	)
}

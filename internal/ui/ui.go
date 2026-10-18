package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
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

	"lighttube/internal/player"
	"lighttube/internal/store"
	"lighttube/internal/thumbs"
	"lighttube/internal/yt"
)

// Options holds runtime dependencies injected into the UI.
type Options struct {
	YTClient  *yt.Client
	Store     *store.Store
	Player    *player.Player
	Thumbs    *thumbs.Manager
	Window    *app.Window
	Debug     bool
	StartTime time.Time
	Search    string
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
		ytClient:      ytClient,
		store:         st,
		player:        p,
		thumbs:        thMgr,
		window:        opts.Window,
		th:            th,
		colors:        colors,
		state:         NewState(),
		power:         player.DetectPower(),
		mode:          player.ModeVideo,
		debug:         opts.Debug,
		startTime:     opts.StartTime,
		initialSearch: opts.Search,
		requested:     make(map[string]bool),
		imageOps:      make(map[string]paint.ImageOp),
		windowTag:     new(int),
		pasteTag:      new(int),
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
		case app.DestroyEvent:
			if e.Err != nil {
				return e.Err
			}
			os.Exit(0)
			return nil

		case app.ConfigEvent:
			if e.Config.Focused {
				ui.power = player.DetectPower()
			}

		case app.FrameEvent:
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
					})
					ui.window.Invalidate()
				}()
			}

			ui.drainActions()
			gtx := app.NewContext(&ops, e)
			ui.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *UI) onStoreLoaded() {
	ui.state.Subscriptions = ui.store.Subscriptions()
	if ui.state.CurrentTab == TabHome {
		ui.loadMergedFeed()
	}
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
	ui.mode = player.ModeVideo
	if audio {
		ui.mode = player.ModeAudio
	}
	ui.updatePower()

	modeText := "video"
	if audio {
		modeText = "audio"
	}
	ui.state.Status = fmt.Sprintf("Playing %s: %s", modeText, v.Title)

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
		)
		if !ok {
			break
		}

		if ke, ok := evt.(key.Event); ok && ke.State == key.Press {
			isEditorFocused := gtx.Focused(&ui.searchEditor) || gtx.Focused(&ui.addChannelEditor) || gtx.Focused(&ui.csvPathEditor)
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

	// Paint bg itself (opaque), so it does not depend on compositor background.
	paint.FillShape(gtx.Ops, ui.colors.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

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
// Center: Pill search box (max width 640, height 40, radius 20, 1dp border, magnifier button)
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
	if ui.logoClick.Clicked(gtx) {
		ui.switchTab(TabHome)
	}

	return layout.Inset{
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			// Left Logo
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.logoClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if ui.logoClick.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							w := gtx.Dp(28)
							h := gtx.Dp(20)
							rect := image.Rect(0, 0, w, h)
							rrect := clip.UniformRRect(rect, gtx.Dp(6))
							paint.FillShape(gtx.Ops, ui.colors.Red, rrect.Op(gtx.Ops))
							drawPlayTriangle(gtx, rect, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
							return layout.Dimensions{Size: image.Pt(w, h)}
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

			// Center Pill Search Box (max width 640, height 40, radius 20)
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					maxW := gtx.Dp(640)
					if gtx.Constraints.Max.X < maxW {
						maxW = gtx.Constraints.Max.X - gtx.Dp(32)
					}
					if maxW < gtx.Dp(200) {
						maxW = gtx.Dp(200)
					}
					h := gtx.Dp(40)
					gtx.Constraints.Min = image.Pt(maxW, h)
					gtx.Constraints.Max = image.Pt(maxW, h)

					radius := gtx.Dp(20)
					pillRect := image.Rect(0, 0, maxW, h)
					pillShape := clip.UniformRRect(pillRect, radius)

					// Pill background
					paint.FillShape(gtx.Ops, color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xff}, pillShape.Op(gtx.Ops))

					// Border (1dp border, focused #1c62b9)
					borderColor := ui.colors.Border
					if gtx.Focused(&ui.searchEditor) {
						borderColor = ui.colors.BorderFocused
					}
					paint.FillShape(gtx.Ops, borderColor, clip.Stroke{
						Path:  pillShape.Path(gtx.Ops),
						Width: float32(gtx.Dp(1)),
					}.Op())

					// Inner layout: Editor on left + 64 wide button with magnifier glyph on right
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{
								Left:  unit.Dp(16),
								Right: unit.Dp(8),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								ed := material.Editor(ui.th, &ui.searchEditor, "Search")
								ed.Color = ui.colors.Text
								ed.HintColor = ui.colors.Text2
								ed.TextSize = unit.Sp(15)
								return ed.Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btnW := gtx.Dp(64)
							btnBox := image.Pt(btnW, h)
							gtx.Constraints.Min = btnBox
							gtx.Constraints.Max = btnBox

							return ui.searchBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								if ui.searchBtn.Hovered() {
									pointer.CursorPointer.Add(gtx.Ops)
								}
								// Surface background with rounded right edge
								rRight := clip.RRect{
									Rect: image.Rect(0, 0, btnW, h),
									SE:   radius,
									NE:   radius,
								}
								btnBg := ui.colors.Surface
								if ui.searchBtn.Hovered() {
									btnBg = ui.colors.SurfaceHover
								}
								paint.FillShape(gtx.Ops, btnBg, rRight.Op(gtx.Ops))

								// Left divider inside pill
								paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, 0, gtx.Dp(1), h)).Op())

								// Magnifier glyph
								drawMagnifier(gtx, image.Rect(0, 0, btnW, h), ui.colors.Text)
								return layout.Dimensions{Size: btnBox}
							})
						}),
					)
				})
			}),

			// Right Policy Chip (surface bg, text2, 12sp)
			layout.Rigid(ui.layoutPolicyLabel),
		)
	})
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

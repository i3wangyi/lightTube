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
	YTClient *yt.Client
	Store    *store.Store
	Player   *player.Player
	Thumbs   *thumbs.Manager
	Window   *app.Window
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

	actionsMu sync.Mutex
	actions   []func()

	// Navigation
	tabClicks [4]widget.Clickable

	// Search bar
	searchEditor widget.Editor
	pasteBtn     widget.Clickable

	// Video list
	videoList  widget.List
	itemClicks []widget.Clickable

	// Subscriptions controls
	addChannelEditor  widget.Editor
	addChannelBtn     widget.Clickable
	csvPathEditor     widget.Editor
	importCSVBtn      widget.Clickable
	manageChannelsBtn widget.Clickable
	showChannelList   bool
	subList           widget.List
	removeSubClicks   []widget.Clickable

	// ImageOp cache to avoid recreating texture operations on every frame.
	imageOps map[string]paint.ImageOp

	// Event tags
	windowTag event.Tag
	pasteTag  event.Tag

	// Focus requests
	pendingFocusTag event.Tag
}

// New creates and initializes a UI instance.
func New(opts Options) *UI {
	ytClient := opts.YTClient
	if ytClient == nil {
		ytClient = yt.NewClient()
	}
	st := opts.Store
	if st == nil {
		st, _ = store.New()
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
		ytClient:  ytClient,
		store:     st,
		player:    p,
		thumbs:    thMgr,
		window:    opts.Window,
		th:        th,
		colors:    colors,
		state:     NewState(),
		power:     player.DetectPower(),
		mode:      player.ModeVideo,
		imageOps:  make(map[string]paint.ImageOp),
		windowTag: new(int),
		pasteTag:  new(int),
	}

	ui.searchEditor.SingleLine = true
	ui.searchEditor.Submit = true

	ui.addChannelEditor.SingleLine = true
	ui.addChannelEditor.Submit = true

	ui.csvPathEditor.SingleLine = true
	ui.csvPathEditor.Submit = true

	ui.videoList.Axis = layout.Vertical
	ui.subList.Axis = layout.Vertical

	return ui
}

// Run configures the application ID, initializes the window, and runs the event loop.
func Run(opts Options) error {
	// Set fixed application ID so Wayland compositors can match window rules.
	app.ID = "lighttube"

	ui := New(opts)
	if ui.window == nil {
		ui.window = new(app.Window)
	}

	ui.window.Option(
		app.Title("lightTube"),
		app.Size(unit.Dp(960), unit.Dp(640)),
		app.MinSize(unit.Dp(640), unit.Dp(400)),
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
				// Re-read power source when window gains focus, avoiding periodic polling.
				ui.power = player.DetectPower()
			}

		case app.FrameEvent:
			ui.drainActions()
			gtx := app.NewContext(&ops, e)
			ui.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *UI) updatePower() {
	ui.power = player.DetectPower()
}

func (ui *UI) switchTab(t Tab) {
	ui.state.SwitchTab(t)
	switch t {
	case TabSearch:
		if len(ui.state.Videos) == 0 {
			ui.state.Status = "Ready"
		}
	case TabSubscriptions:
		ui.loadMergedFeed()
	case TabWatchLater:
		ui.state.SetVideos(ui.store.WatchLater())
		ui.state.Status = fmt.Sprintf("%d videos in Watch later", len(ui.state.Videos))
	case TabHistory:
		ui.state.SetVideos(ui.store.History())
		ui.state.Status = fmt.Sprintf("%d videos in history", len(ui.state.Videos))
	}
}

func (ui *UI) search(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
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
			ui.state.Status = fmt.Sprintf("%d results for %q", len(videos), query)
		})
		ui.window.Invalidate()
	}()
}

func (ui *UI) loadMergedFeed() {
	subs := ui.store.Subscriptions()
	ui.state.Subscriptions = subs
	if len(subs) == 0 {
		ui.state.SetVideos(nil)
		ui.state.Status = "No subscriptions yet. Add channels above or import CSV."
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
			ui.state.Status = fmt.Sprintf("%d subscription videos", len(videos))
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
			ui.state.Status = "Added channel: " + title
			ui.loadMergedFeed()
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
			ui.state.Status = fmt.Sprintf("Imported %d channels from CSV", len(subs))
			ui.loadMergedFeed()
		})
		ui.window.Invalidate()
	}()
}

func (ui *UI) removeSubscription(channelID string) {
	_ = ui.store.RemoveSubscription(channelID)
	ui.loadMergedFeed()
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

	// Save playback to history.
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

func (ui *UI) toggleWatchLater() {
	v, ok := ui.state.SelectedVideo()
	if !ok {
		return
	}

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

func (ui *UI) scrollToSelected() {
	idx := ui.state.SelectedIndex
	if idx < 0 || len(ui.state.Videos) == 0 {
		return
	}
	first := ui.videoList.Position.First
	count := ui.videoList.Position.Count
	if idx < first {
		ui.videoList.Position.First = idx
		ui.videoList.Position.Offset = 0
	} else if count > 0 && idx >= first+count {
		ui.videoList.Position.First = idx - count + 1
		ui.videoList.Position.Offset = 0
	}
}

func (ui *UI) layout(gtx layout.Context) layout.Dimensions {
	// Handle pending programmatic focus requests (such as Ctrl+L).
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
				ui.scrollToSelected()
			case ActionPlay:
				ui.playSelected(false)
			case ActionPlayAudio:
				ui.playSelected(true)
			case ActionToggleWatchLater:
				ui.toggleWatchLater()
			}
		}
	}

	// Background fill for window.
	paint.FillShape(gtx.Ops, ui.colors.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(ui.layoutTopBar),
		layout.Rigid(ui.layoutTabBarSection),
		layout.Flexed(1, ui.layoutMainContent),
		layout.Rigid(ui.layoutStatusBar),
	)
}

func (ui *UI) layoutTopBar(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(10),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					ui.layoutTabButton(gtx, 0, TabSearch),
					ui.layoutTabButton(gtx, 1, TabSubscriptions),
					ui.layoutTabButton(gtx, 2, TabWatchLater),
					ui.layoutTabButton(gtx, 3, TabHistory),
				)
			}),
			layout.Rigid(ui.layoutPolicyLabel),
		)
	})
}

func (ui *UI) layoutTabButton(gtx layout.Context, idx int, tab Tab) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ui.tabClicks[idx].Clicked(gtx) {
			ui.switchTab(tab)
		}

		isSelected := ui.state.CurrentTab == tab
		return ui.tabClicks[idx].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(6),
				Bottom: unit.Dp(6),
				Left:   unit.Dp(12),
				Right:  unit.Dp(12),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.th, unit.Sp(14), tab.String())
				if isSelected {
					lbl.Color = ui.colors.Accent
					lbl.Font.Weight = font.Bold
				} else {
					lbl.Color = ui.colors.Subdued
				}
				dims := lbl.Layout(gtx)

				// Draw active indicator line under selected tab.
				if isSelected {
					indicatorRect := image.Rectangle{
						Min: image.Pt(0, dims.Size.Y-gtx.Dp(2)),
						Max: image.Pt(dims.Size.X, dims.Size.Y),
					}
					paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(indicatorRect).Op())
				}
				return dims
			})
		})
	})
}

func (ui *UI) layoutPolicyLabel(gtx layout.Context) layout.Dimensions {
	policyText := FormatPolicyLabel(ui.power, ui.mode)
	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
		Left:   unit.Dp(8),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(ui.th, unit.Sp(12), "[policy: "+policyText+"]")
		lbl.Color = ui.colors.Subdued
		return lbl.Layout(gtx)
	})
}

func (ui *UI) layoutTabBarSection(gtx layout.Context) layout.Dimensions {
	switch ui.state.CurrentTab {
	case TabSearch:
		return ui.layoutSearchBar(gtx)
	case TabSubscriptions:
		return ui.layoutSubscriptionsControls(gtx)
	default:
		return layout.Dimensions{}
	}
}

func (ui *UI) layoutSearchBar(gtx layout.Context) layout.Dimensions {
	// Check for search editor submit.
	for {
		ev, ok := ui.searchEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			ui.search(ui.searchEditor.Text())
		}
	}

	if ui.pasteBtn.Clicked(gtx) {
		gtx.Execute(clipboard.ReadCmd{Tag: ui.pasteTag})
	}

	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(4))
						paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Min}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Top:    unit.Dp(8),
							Bottom: unit.Dp(8),
							Left:   unit.Dp(12),
							Right:  unit.Dp(12),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							ed := material.Editor(ui.th, &ui.searchEditor, "Search YouTube (Enter to search, Ctrl+L to focus)…")
							ed.Color = ui.colors.Fg
							ed.HintColor = ui.colors.Subdued
							return ed.Layout(gtx)
						})
					},
				)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(ui.th, &ui.pasteBtn, "Paste URL")
				btn.Background = ui.colors.SurfaceHover
				btn.Color = ui.colors.Fg
				btn.CornerRadius = unit.Dp(4)
				btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}
				return btn.Layout(gtx)
			}),
		)
	})
}

func (ui *UI) layoutSubscriptionsControls(gtx layout.Context) layout.Dimensions {
	// Add channel submit.
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

	// CSV import submit.
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

	if ui.manageChannelsBtn.Clicked(gtx) {
		ui.showChannelList = !ui.showChannelList
	}

	return layout.Inset{
		Top:    unit.Dp(4),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Row 1: Add Channel
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(4))
								paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									ed := material.Editor(ui.th, &ui.addChannelEditor, "Channel URL, @handle, or UC id…")
									ed.Color = ui.colors.Fg
									ed.HintColor = ui.colors.Subdued
									return ed.Layout(gtx)
								})
							},
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(ui.th, &ui.addChannelBtn, "Add")
						btn.Background = ui.colors.SurfaceHover
						btn.Color = ui.colors.Fg
						btn.CornerRadius = unit.Dp(4)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),

			// Row 2: Import Takeout CSV
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(4))
								paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									ed := material.Editor(ui.th, &ui.csvPathEditor, "Path to subscriptions.csv…")
									ed.Color = ui.colors.Fg
									ed.HintColor = ui.colors.Subdued
									return ed.Layout(gtx)
								})
							},
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(ui.th, &ui.importCSVBtn, "Import Takeout CSV")
						btn.Background = ui.colors.SurfaceHover
						btn.Color = ui.colors.Fg
						btn.CornerRadius = unit.Dp(4)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),

			// Row 3: Subscribed channels toggle and management
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				subsCount := len(ui.state.Subscriptions)
				manageText := fmt.Sprintf("Channels (%d) ▸ Manage", subsCount)
				if ui.showChannelList {
					manageText = fmt.Sprintf("Channels (%d) ▾ Hide", subsCount)
				}
				btn := material.Button(ui.th, &ui.manageChannelsBtn, manageText)
				btn.Background = color.NRGBA{A: 0}
				btn.Color = ui.colors.Accent
				btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(0), Right: unit.Dp(0)}
				return btn.Layout(gtx)
			}),

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !ui.showChannelList || len(ui.state.Subscriptions) == 0 {
					return layout.Dimensions{}
				}
				return ui.layoutSubscribedChannelsList(gtx)
			}),
		)
	})
}

func (ui *UI) layoutSubscribedChannelsList(gtx layout.Context) layout.Dimensions {
	subs := ui.state.Subscriptions
	for len(ui.removeSubClicks) < len(subs) {
		ui.removeSubClicks = append(ui.removeSubClicks, widget.Clickable{})
	}

	gtx.Constraints.Max.Y = gtx.Dp(120)

	return material.List(ui.th, &ui.subList).Layout(gtx, len(subs), func(gtx layout.Context, i int) layout.Dimensions {
		if ui.removeSubClicks[i].Clicked(gtx) {
			ui.removeSubscription(subs[i].ChannelID)
		}

		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					title := subs[i].Title
					if title == "" {
						title = subs[i].ChannelID
					}
					lbl := material.Label(ui.th, unit.Sp(13), title)
					lbl.Color = ui.colors.Fg
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(ui.th, &ui.removeSubClicks[i], "Remove")
					btn.Background = color.NRGBA{R: 0x88, G: 0x22, B: 0x22, A: 0xff}
					btn.Color = ui.colors.Fg
					btn.CornerRadius = unit.Dp(3)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
			)
		})
	})
}

func (ui *UI) layoutMainContent(gtx layout.Context) layout.Dimensions {
	videos := ui.state.Videos
	if len(videos) == 0 {
		return ui.layoutEmptyState(gtx)
	}

	// Request thumbnails only for visible rows plus 10 margin.
	first := ui.videoList.Position.First
	count := ui.videoList.Position.Count
	if count == 0 {
		count = 10
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
		vid := videos[i].ID
		if vid == "" {
			continue
		}
		if _, ok := ui.thumbs.Get(vid); !ok {
			ui.thumbs.Load(vid, func(img image.Image, err error) {
				if err == nil && img != nil {
					ui.window.Invalidate()
				}
			})
		}
	}

	for len(ui.itemClicks) < len(videos) {
		ui.itemClicks = append(ui.itemClicks, widget.Clickable{})
	}

	return material.List(ui.th, &ui.videoList).Layout(gtx, len(videos), func(gtx layout.Context, i int) layout.Dimensions {
		return ui.layoutVideoRow(gtx, i)
	})
}

func (ui *UI) layoutVideoRow(gtx layout.Context, index int) layout.Dimensions {
	v := ui.state.Videos[index]
	click := &ui.itemClicks[index]

	for {
		c, ok := click.Update(gtx)
		if !ok {
			break
		}
		ui.state.SelectedIndex = index
		if c.NumClicks >= 2 {
			ui.playSelected(false)
		}
	}

	isSelected := index == ui.state.SelectedIndex
	isHovered := click.Hovered()

	bgColor := ui.colors.Bg
	if isSelected {
		bgColor = ui.colors.SurfaceSelect
	} else if isHovered {
		bgColor = ui.colors.SurfaceHover
	}

	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
				if isSelected {
					// Draw accent line on left edge of selected row.
					indicator := image.Rectangle{
						Min: image.Pt(0, 0),
						Max: image.Pt(gtx.Dp(4), gtx.Constraints.Min.Y),
					}
					paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(indicator).Op())
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top:    unit.Dp(6),
					Bottom: unit.Dp(6),
					Left:   unit.Dp(16),
					Right:  unit.Dp(16),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Alignment: layout.Middle,
					}.Layout(gtx,
						// 160x90 Thumbnail box
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.layoutThumbnail(gtx, v)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
						// Metadata column: Title + Channel · duration · age
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									title := material.Label(ui.th, unit.Sp(14), v.Title)
									title.Font.Weight = font.SemiBold
									title.Color = ui.colors.Fg
									title.MaxLines = 2
									title.Truncator = "…"
									return title.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									subtitle := ui.formatSubtitle(v)
									meta := material.Label(ui.th, unit.Sp(12), subtitle)
									meta.Color = ui.colors.Subdued
									meta.MaxLines = 1
									meta.Truncator = "…"
									return meta.Layout(gtx)
								}),
							)
						}),
					)
				})
			},
		)
	})
}

func (ui *UI) layoutThumbnail(gtx layout.Context, v yt.Video) layout.Dimensions {
	w := gtx.Dp(160)
	h := gtx.Dp(90)
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

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			r := clip.UniformRRect(image.Rectangle{Max: boxSize}, gtx.Dp(4))
			paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
			return layout.Dimensions{Size: boxSize}
		},
		func(gtx layout.Context) layout.Dimensions {
			if hasImg {
				defer clip.UniformRRect(image.Rectangle{Max: boxSize}, gtx.Dp(4)).Push(gtx.Ops).Pop()
				widget.Image{
					Src: imgOp,
					Fit: widget.Cover,
				}.Layout(gtx)
			}

			// Duration badge at bottom right.
			durStr := yt.FormatDuration(v.Duration)
			if durStr != "" && durStr != "00:00" {
				return layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Bottom: unit.Dp(4),
						Right:  unit.Dp(4),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(2))
								paint.FillShape(gtx.Ops, ui.colors.BadgeBg, r.Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{
									Top:    unit.Dp(1),
									Bottom: unit.Dp(1),
									Left:   unit.Dp(4),
									Right:  unit.Dp(4),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.th, unit.Sp(10), durStr)
									lbl.Color = ui.colors.BadgeFg
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

func (ui *UI) formatSubtitle(v yt.Video) string {
	parts := make([]string, 0, 3)
	if v.Channel != "" {
		parts = append(parts, v.Channel)
	}
	if dur := yt.FormatDuration(v.Duration); dur != "" && dur != "00:00" {
		parts = append(parts, dur)
	}
	if age := FormatAge(v.UploadDate, v.PublishedAt, time.Now()); age != "" {
		parts = append(parts, age)
	}
	return strings.Join(parts, " · ")
}

func (ui *UI) layoutEmptyState(gtx layout.Context) layout.Dimensions {
	msg := "No items to display"
	switch ui.state.CurrentTab {
	case TabSearch:
		msg = "Search for YouTube videos above, or paste a video URL"
	case TabSubscriptions:
		msg = "No subscription videos yet. Add channels above or import CSV"
	case TabWatchLater:
		msg = "Watch later list is empty. Press 'W' on any video to add it"
	case TabHistory:
		msg = "Playback history is empty"
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(ui.th, unit.Sp(14), msg)
		lbl.Color = ui.colors.Subdued
		return lbl.Layout(gtx)
	})
}

func (ui *UI) layoutStatusBar(gtx layout.Context) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, ui.colors.Surface, clip.Rect{Max: gtx.Constraints.Min}.Op())
			// Top separator border line
			borderRect := image.Rectangle{
				Min: image.Pt(0, 0),
				Max: image.Pt(gtx.Constraints.Min.X, gtx.Dp(1)),
			}
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(borderRect).Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(6),
				Bottom: unit.Dp(6),
				Left:   unit.Dp(16),
				Right:  unit.Dp(16),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.th, unit.Sp(12), ui.state.Status)
				lbl.Color = ui.colors.Subdued
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			})
		},
	)
}

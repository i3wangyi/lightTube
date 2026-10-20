package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"gioui.org/app"
	"gioui.org/f32"
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
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"lighttube/internal/demo"
	"lighttube/internal/embed"
	"lighttube/internal/platform"
	"lighttube/internal/player"
	"lighttube/internal/store"
	"lighttube/internal/thumbs"
	"lighttube/internal/yt"
)

// Options holds runtime dependencies injected into the UI.
type Options struct {
	YTClient           *yt.Client
	Store              *store.Store
	Player             *player.Player
	Account            *yt.Account
	Thumbs             *thumbs.Manager
	Window             *app.Window
	Debug              bool
	StartTime          time.Time
	Search             string
	ExternalPlayer     bool
	DebugPlay          string
	DebugPlaySeconds   int
	DebugTab           string
	DebugHover         *int
	DebugHoverSeconds  int
	DebugHeap          string
	DebugSearch        string
	DebugPlaylist      bool
	GoogleSignInRunner yt.CommandRunner
	SuggestClient      *yt.SuggestionsClient
	SuggestClock       yt.Clock
	Demo               bool
	DebugSize          string
	DebugChip          int
	AmbientOff         bool
	DebugWatch         bool
	DebugSpace         string
	DebugVideoMarker   bool
	DebugPlayFirst     bool
	DebugSeekSeconds   int
	DebugSequence      string
}

// UI manages the Gio window, layout, and event interactions.
type UI struct {
	startupMemory      startupMemory
	startupStoreLoaded bool
	startupFeedPending bool
	fonts              *themeFonts
	ytClient           *yt.Client
	store              *store.Store
	player             *player.Player
	account            *yt.Account
	thumbs             *thumbs.Manager
	window             *app.Window

	th     *material.Theme
	colors ThemeColors
	state  *State

	power player.Power
	mode  player.Mode

	debug               bool
	startTime           time.Time
	initialSearch       string
	firstFrameLogged    bool
	firstFramePresented bool
	dataLoaded          bool

	// Options & flags
	externalPlayer     bool
	debugPlay          string
	debugPlaySeconds   int
	debugPlayTriggered bool
	debugPlayFirst     bool
	debugSeekSeconds   int
	debugSequence      *debugSequence
	debugVideoMarker   bool
	spaceOrigin        image.Point
	spaceSize          image.Point
	lastLoggedVideoBox image.Rectangle
	debugHeap          string
	heapWritten        bool

	// Embedded player & platform view
	view            platformView
	embedPlayer     EmbedPlayerController
	activeVideoBox  image.Rectangle
	videoBoxLaidOut bool
	currentScale    int
	hwdecLogged     bool
	lastTimePoll    time.Time
	previousTab     Tab

	// Watch page state & controls
	watchVideo            yt.Video
	upNextVideos          []yt.Video
	watchPaused           bool
	watchMuted            bool
	watchVolume           float64
	watchCurrentTime      float64
	watchDuration         float64
	playerOwnerSpace      SpaceID
	playerOwnerID         string
	shortsSoundOn         bool
	playerMuteKnown       bool
	playerMuted           bool
	pendingResumeID       string
	pendingResumeSeconds  float64
	lastPositionSave      time.Time
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

	// Settings & GPU capabilities
	settings           store.Settings
	cliExternalPlayer  bool
	cachedCaps         embed.Caps
	capsLoaded         bool
	capsErr            error
	codecAutoChip      widget.Clickable
	codecOldGPUChip    widget.Clickable
	codecAnyChip       widget.Clickable
	playerInplaceChip  widget.Clickable
	playerExternalChip widget.Clickable

	// Account & Feeds
	accountSigningIn     bool
	accountError         string
	accountBrowserEditor widget.Editor
	accountSignInBtn     widget.Clickable
	refreshMu            sync.Mutex
	refreshingFeeds      map[yt.FeedKind]bool

	// Phase 9: Typeahead
	suggestMgr      *yt.SuggestManager
	suggestItems    []string
	suggestSelected int
	suggestVisible  bool
	suggestClicks   [10]widget.Clickable
	lastSearchText  string

	// Phase 9: Playlists
	playlistsLoading bool
	playlistsLoaded  bool
	playlistClicks   []widget.Clickable
	playlistBackBtn  widget.Clickable
	debugPlaylist    bool

	// Phase 10b: Spaces Loading
	shortsLoading   bool
	podcastsLoading bool
	musicLoading    bool

	// Phase 9: Multi-Account
	accountSwitchClicks  map[string]*widget.Clickable
	accountSignOutClicks map[string]*widget.Clickable
	accountRemoveClicks  map[string]*widget.Clickable
	addAccountBtn        widget.Clickable
	cancelAddAccountBtn  widget.Clickable
	accountSignOutBtn    widget.Clickable
	showAddAccount       bool
	discoveredProfiles   []yt.BrowserProfile

	// Test hooks
	labelHook            func(string)
	addProfileClicks     []widget.Clickable
	signInGoogleBtn      widget.Clickable
	signInGoogleStatus   string
	googleSignInLauncher *yt.GoogleSignInLauncher
	accountList          layout.List
	accountNavSubsClick  widget.Clickable
	accountNavLibClick   widget.Clickable

	// Mini guide (5 items: Home, Subscriptions, Watch later, History, Account)
	guideClicks [5]widget.Clickable

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
	subPageList       layout.List
	unsubscribeClicks []widget.Clickable

	// Requested thumbnail set to prevent re-requesting in-flight images on every frame
	requestedMu sync.Mutex
	requested   map[string]bool

	// ImageOp cache to avoid recreating texture operations on every frame
	imageOps map[string]paint.ImageOp

	// Account channels & Subscriptions channels row
	accountChannelsFetching bool
	subsChannelsList        layout.List
	subsChannelClicks       []widget.Clickable

	// Hover preview state
	debugHover        int
	debugHoverSeconds int
	hoverVideoID      string
	hoverCardIdx      int
	hoverStartTime    time.Time
	hoverFetchStarted bool
	hoverCancel       context.CancelFunc
	hoverFrames       []image.Image
	// One ImageOp per frame, made once: a new ImageOp is a new GPU upload.
	hoverOps           []paint.ImageOp
	hoverFrameIdx      int
	hoverLastFrameTime time.Time

	// Event tags
	windowTag       event.Tag
	pasteTag        event.Tag
	pendingFocusTag event.Tag

	// Phase 10: Four Spaces & Shell
	spaces             *SpacesManager
	showingUtility     bool
	utilityTab         Tab
	spaceNavClicks     [4]widget.Clickable
	railLibraryClick   widget.Clickable
	railSubsClick      widget.Clickable
	railYouClick       widget.Clickable
	ambientToggleClick widget.Clickable
	ambientShape       AmbientShape
	demo               bool
	demoData           *DemoData
	demoLoading        map[string]bool
	generation         int64
	generationMu       sync.Mutex
	heroPrimaryClick   widget.Clickable
	heroQuietClick     widget.Clickable
	heroPreviewClick   widget.Clickable
	topBarLibraryClick widget.Clickable
	topBarProfileClick widget.Clickable
	allYouTubeClick    widget.Clickable
	watchBackBtn       widget.Clickable
	demoTmpDir         string
	lastLoggedSecond   int
}

// New creates and initializes a UI instance with explicit fonts and zero I/O.
func New(opts Options) *UI {
	ytClient := opts.YTClient
	if !opts.Demo && ytClient == nil {
		ytClient = yt.NewClient()
	}
	st := opts.Store
	var demoTmpDir string
	if st == nil {
		if opts.Demo {
			tmp, err := os.MkdirTemp("", "lighttube-demo-ui-*")
			if err == nil {
				demoTmpDir = tmp
				st, _ = store.NewDeferred(demoTmpDir)
			}
		} else {
			st, _ = store.NewDeferred(store.DefaultDataDir())
		}
	}
	p := opts.Player
	if !opts.Demo && p == nil {
		p = player.New()
	}
	thMgr := opts.Thumbs
	if !opts.Demo && thMgr == nil {
		thMgr = thumbs.New()
	}

	th, colors, fonts := newLazyDarkTheme()

	acc := opts.Account
	if !opts.Demo && acc == nil {
		acc = yt.NewAccount()
	}

	settings := store.DefaultSettings()
	if opts.Store != nil {
		settings = opts.Store.Settings()
		if !opts.Demo {
			activeAcc := opts.Store.ActiveAccount()
			if acc != nil {
				if activeAcc.CookieFile != "" {
					acc.CookieFile = activeAcc.CookieFile
				}
				if activeAcc.CacheDir != "" {
					acc.CacheDir = activeAcc.CacheDir
				}
				if activeAcc.BrowserSpec != "" {
					acc.SetBrowser(activeAcc.BrowserSpec)
				} else {
					acc.SetBrowser(settings.BrowserSpec)
				}
			}
		}
	} else if opts.Account != nil && opts.Account.Browser != "" {
		settings.BrowserSpec = opts.Account.Browser
	} else if acc != nil {
		acc.SetBrowser(settings.BrowserSpec)
	}

	initSearch := opts.Search
	if opts.DebugSearch != "" {
		initSearch = opts.DebugSearch
	}

	extPlayer := opts.ExternalPlayer
	if !extPlayer {
		extPlayer = (settings.Player == store.PlayerExternal)
	}

	ui := &UI{
		fonts:                fonts,
		ytClient:             ytClient,
		store:                st,
		player:               p,
		account:              acc,
		thumbs:               thMgr,
		window:               opts.Window,
		th:                   th,
		colors:               colors,
		state:                NewState(),
		power:                player.DetectPower(),
		mode:                 player.ModeVideo,
		debug:                opts.Debug,
		startTime:            opts.StartTime,
		initialSearch:        initSearch,
		settings:             settings,
		cliExternalPlayer:    opts.ExternalPlayer,
		externalPlayer:       extPlayer,
		debugPlay:            opts.DebugPlay,
		debugPlaySeconds:     opts.DebugPlaySeconds,
		debugHover:           -1,
		debugHoverSeconds:    opts.DebugHoverSeconds,
		hoverCardIdx:         -1,
		debugHeap:            opts.DebugHeap,
		debugPlaylist:        opts.DebugPlaylist,
		debugVideoMarker:     opts.DebugVideoMarker,
		debugPlayFirst:       opts.DebugPlayFirst,
		debugSeekSeconds:     opts.DebugSeekSeconds,
		debugSequence:        newDebugSequence(opts.DebugSequence),
		watchLastMouseMove:   time.Now(),
		watchVolume:          100,
		currentScale:         2,
		suggestSelected:      -1,
		accountSwitchClicks:  make(map[string]*widget.Clickable),
		accountSignOutClicks: make(map[string]*widget.Clickable),
		accountRemoveClicks:  make(map[string]*widget.Clickable),
		googleSignInLauncher: yt.NewGoogleSignInLauncher(opts.GoogleSignInRunner),
		refreshingFeeds:      make(map[yt.FeedKind]bool),
		requested:            make(map[string]bool),
		imageOps:             make(map[string]paint.ImageOp),
		windowTag:            new(int),
		pasteTag:             new(int),
		spaces:               NewSpacesManager(),
		demo:                 opts.Demo,
		demoTmpDir:           demoTmpDir,
		demoLoading:          make(map[string]bool),
	}

	if opts.Demo {
		ui.demoData = LoadDemoData()
		if ui.demoData != nil {
			ui.state.Videos = ui.demoData.Videos
		}
	} else {
		suggestClient := opts.SuggestClient
		if suggestClient == nil {
			suggestClient = yt.NewSuggestionsClient(nil)
		}
		ui.suggestMgr = yt.NewSuggestManagerWithClock(suggestClient, opts.SuggestClock, 150*time.Millisecond, func(query string, suggestions []string) {
			ui.sendAction(func() {
				if yt.IsURLQuery(ui.searchEditor.Text()) {
					ui.suggestVisible = false
					ui.suggestItems = nil
					return
				}
				ui.suggestItems = suggestions
				ui.suggestSelected = -1
				ui.suggestVisible = len(suggestions) > 0
			})
			ui.invalidate()
		})
	}

	ui.subsChannelsList.Axis = layout.Horizontal
	ui.accountList.Axis = layout.Vertical
	ui.chipsList.Axis = layout.Horizontal

	if ui.thumbs != nil {
		ui.thumbs.SetOnEvict(func(id string) {
			ui.sendAction(func() {
				delete(ui.imageOps, id)
			})
		})
	}

	if opts.DebugTab == "you" || opts.DebugTab == "account" {
		ui.state.CurrentTab = TabAccount
	}
	if opts.DebugTab == "subs" || opts.DebugTab == "subscriptions" {
		ui.state.CurrentTab = TabSubscriptions
	}
	if opts.DebugHover != nil {
		ui.debugHover = *opts.DebugHover
	}

	go func() {
		caps, err := embed.DecodeCaps()
		ui.sendAction(func() {
			ui.cachedCaps = caps
			ui.capsLoaded = true
			ui.capsErr = err
		})
		ui.invalidate()
	}()

	ui.searchEditor.SingleLine = true
	ui.searchEditor.Submit = true
	if initSearch != "" {
		ui.searchEditor.SetText(initSearch)
	}
	if opts.DebugSearch != "" {
		ui.search(opts.DebugSearch)
	}
	if opts.DebugPlaylist {
		ui.state.CurrentTab = TabPlaylist
		ui.loadSavedPlaylists()
	}
	if opts.DebugSpace != "" {
		switch strings.ToLower(opts.DebugSpace) {
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

	ui.accountBrowserEditor.SingleLine = true
	ui.accountBrowserEditor.Submit = true
	if acc != nil {
		ui.accountBrowserEditor.SetText(acc.Browser)
	}

	ui.addChannelEditor.SingleLine = true
	ui.addChannelEditor.Submit = true

	ui.csvPathEditor.SingleLine = true
	ui.csvPathEditor.Submit = true

	ui.chipsList.Axis = layout.Horizontal
	ui.gridList.Axis = layout.Vertical
	ui.searchList.Axis = layout.Vertical
	ui.subList.Axis = layout.Vertical
	ui.subPageList.Axis = layout.Vertical
	ui.watchList.Axis = layout.Vertical
	ui.upNextList.Axis = layout.Vertical

	return ui
}

// Run configures the application ID, initializes the window, and runs the event loop.
func (ui *UI) writeHeapProfile() {
	if ui.debugHeap == "" {
		return
	}
	runtime.GC()
	f, err := os.Create(ui.debugHeap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create heap profile: %v\n", err)
		return
	}
	defer f.Close()
	if err := pprof.WriteHeapProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write heap profile: %v\n", err)
		return
	}
	ui.heapWritten = true
	if ui.debug {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		fmt.Fprintf(os.Stderr, "memory: heap_live=%d heap_sys=%d heap_released=%d heap_goal=%d thumbnails=%d thumbnail_bytes=%d\n", stats.HeapAlloc, stats.HeapSys, stats.HeapReleased, stats.NextGC, ui.thumbs.Len(), ui.thumbs.UsedBytes())
		fmt.Fprintf(os.Stderr, "heap profile written to %s\n", ui.debugHeap)
	}
}

func Run(opts Options) error {
	if opts.Demo {
		if err := demo.ValidateAssets(); err != nil {
			return fmt.Errorf("demo assets error: %w", err)
		}
	}
	app.ID = "lighttube"

	ui := New(opts)
	defer func() {
		if ui.demoTmpDir != "" {
			_ = os.RemoveAll(ui.demoTmpDir)
		}
	}()
	if ui.window == nil {
		ui.window = new(app.Window)
	}

	winW := 1120
	winH := 720
	minW := 360
	minH := 400
	if opts.DebugSize != "" {
		parts := strings.Split(opts.DebugSize, "x")
		if len(parts) == 2 {
			if w, err := strconv.Atoi(parts[0]); err == nil && w > 0 {
				winW = w
				if w < minW {
					minW = w
				}
			}
			if h, err := strconv.Atoi(parts[1]); err == nil && h > 0 {
				winH = h
				if h < minH {
					minH = h
				}
			}
		}
	}

	title := "lightTube"
	if opts.DebugSize != "" {
		title = "lightTube-test"
	}

	ui.window.Option(
		app.Title(title),
		app.Size(unit.Dp(winW), unit.Dp(winH)),
		app.MinSize(unit.Dp(minW), unit.Dp(minH)),
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		if !ui.heapWritten {
			ui.writeHeapProfile()
		}
		os.Exit(0)
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ui.eventLoop()
	}()

	app.Main()
	if !ui.heapWritten {
		ui.writeHeapProfile()
	}
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

func (ui *UI) invalidate() {
	if ui.window != nil {
		ui.window.Invalidate()
	}
}

func (ui *UI) eventLoop() error {
	var ops op.Ops
	for {
		e := ui.window.Event()
		switch e := e.(type) {
		case *app.ClosingEvent:
			if ui.embedPlayer != nil {
				ui.saveCurrentPosition()
				ui.embedPlayer.Stop()
				ui.embedPlayer.Close()
				ui.embedPlayer = nil
			}

		case app.DestroyEvent:
			if ui.embedPlayer != nil {
				ui.saveCurrentPosition()
				ui.embedPlayer.Stop()
				ui.embedPlayer.Close()
				ui.embedPlayer = nil
			}
			if !ui.heapWritten {
				ui.writeHeapProfile()
			}
			if e.Err != nil {
				os.Exit(1)
			}
			os.Exit(0)
			return nil

		default:
			if ui.handleViewEvent(e) {
				break
			}

		case app.ConfigEvent:
			if e.Config.Focused {
				ui.power = player.DetectPower()
			}

		case app.FrameEvent:
			if ui.debug {
				fmt.Fprintf(os.Stderr, "FRAME_EVENT: size=%v metric=%+v\n", e.Size, e.Metric)
			}
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
								time.Sleep(1 * time.Second)
								ui.sendAction(func() {
									ui.triggerDebugPlay(ui.debugPlay, ui.debugPlaySeconds)
								})
								ui.invalidate()
							}()
						}
					})
					ui.invalidate()
				}()
			}

			// Poll mpv status at most 4 times a second, and only while playing and window visible
			if ui.embedPlayer != nil && (!ui.watchPaused || ui.state.CurrentTab == TabWatch) {
				if time.Since(ui.lastTimePoll) >= 250*time.Millisecond {
					ui.lastTimePoll = time.Now()
					st := ui.embedPlayer.Status()
					if ui.pendingResumeID != "" && ui.pendingResumeID == ui.playerOwnerID && st.Duration > 0 {
						ui.embedPlayer.Seek(ui.pendingResumeSeconds, false)
						ui.pendingResumeID = ""
						ui.pendingResumeSeconds = 0
					}
					if ui.debugSeekSeconds > 0 && st.Duration > 0 && st.TimePos > 0 && !st.Paused {
						ui.embedPlayer.Seek(float64(ui.debugSeekSeconds), false)
						if ui.debug {
							fmt.Fprintf(os.Stderr, "debug-seek=%d\n", ui.debugSeekSeconds)
						}
						ui.debugSeekSeconds = 0
					}
					ui.watchCurrentTime = st.TimePos
					if st.Duration > 0 {
						ui.watchDuration = st.Duration
					}
					ui.watchPaused = st.Paused
					ui.watchMuted = st.Muted
					ui.playerMuted, ui.playerMuteKnown = st.Muted, true
					ui.watchVolume = st.Volume
					if !st.Paused && time.Since(ui.lastPositionSave) >= 10*time.Second {
						ui.saveCurrentPosition()
						ui.lastPositionSave = time.Now()
					}
					if ui.debug && !ui.hwdecLogged && st.HWDecCurrent != "" && st.HWDecCurrent != "no" {
						ui.hwdecLogged = true
						fmt.Fprintf(os.Stderr, "hwdec-current=%s video-codec=%s\n", st.HWDecCurrent, st.VideoCodec)
						if ui.debugPlaySeconds > 0 {
							go func() {
								time.Sleep(time.Duration(ui.debugPlaySeconds) * time.Second)
								ui.writeHeapProfile()
								ui.sendAction(func() {
									if ui.state.CurrentTab == TabWatch {
										ui.closeWatchPage()
									} else {
										ui.pausePlayback()
									}
								})
								ui.invalidate()
							}()
						}
					}
					if ui.debug && int(st.TimePos) > 0 && int(st.TimePos) != ui.lastLoggedSecond {
						ui.lastLoggedSecond = int(st.TimePos)
						fmt.Fprintf(os.Stderr, "playback-pos=%d duration=%.1f paused=%t playing=%t hwdec=%s\n", int(st.TimePos), st.Duration, st.Paused, st.IsPlaying, st.HWDecCurrent)
					}
				}
			}

			ui.drainActions()
			gtx := app.NewContext(&ops, e)
			step := ui.advanceDebugSequence(gtx.Now)
			ui.layout(gtx)
			ui.logDebugSequenceStep(step)
			if ui.debugSequence != nil && !ui.debugSequence.done {
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(100 * time.Millisecond)})
			}

			// Invalidate for next timer tick when playing
			if ui.embedPlayer != nil && !ui.watchPaused {
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(250 * time.Millisecond)})
			}
			if ui.state.CurrentTab == TabWatch && ui.watchFullscreen && time.Since(ui.watchLastMouseMove) < 3*time.Second {
				gtx.Execute(op.InvalidateCmd{At: ui.watchLastMouseMove.Add(3 * time.Second)})
			}

			e.Frame(gtx.Ops)
			ui.startupMemory.frame(ui.startupMemoryReady())
			ui.firstFramePresented = true
		}
	}
}

func (ui *UI) onStoreLoaded() {
	ui.startupStoreLoaded = true
	if ui.debugSequence != nil {
		ui.debugSequence.ready = true
	}
	ui.settings = ui.store.Settings()
	if !ui.cliExternalPlayer {
		ui.externalPlayer = (ui.settings.Player == store.PlayerExternal)
	}
	if ui.account != nil {
		ui.account.SetBrowser(ui.settings.BrowserSpec)
	}
	ui.state.Subscriptions = ui.store.Subscriptions()
	if ui.demo {
		return
	}
	if ui.state.CurrentTab == TabHome {
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedHome)
			ui.setWatchVideos(cached)
			ui.refreshAccountFeed(yt.FeedHome)
			if chs, _ := ui.account.CachedChannels(); len(chs) > 0 {
				ui.state.AccountChannels = chs
			}
			ui.refreshAccountChannels()
		} else {
			ui.loadMergedFeed()
		}
	} else if ui.state.CurrentTab == TabSubscriptions {
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedSubs)
			ui.setWatchVideos(cached)
			ui.refreshAccountFeed(yt.FeedSubs)
			if chs, _ := ui.account.CachedChannels(); len(chs) > 0 {
				ui.state.AccountChannels = chs
			}
			ui.refreshAccountChannels()
		} else {
			ui.state.SetVideos(nil)
		}
	} else if ui.state.CurrentTab == TabAccount {
		ui.loadSavedPlaylists()
	} else if ui.state.CurrentTab == TabPlaylist || ui.debugPlaylist {
		ui.loadSavedPlaylists()
	}
}

func (ui *UI) updatePower() {
	ui.power = player.DetectPower()
}

func (ui *UI) refreshAccountFeed(kind yt.FeedKind) {
	if ui.account == nil || !ui.account.SignedIn() {
		return
	}
	utility, tab := ui.showingUtility, ui.state.CurrentTab
	kindOfLoad := loadContent
	if utility {
		kindOfLoad = loadLibrary
	}
	request := ui.beginSpaceRequest(SpaceWatch, kindOfLoad)
	ui.refreshMu.Lock()
	if ui.refreshingFeeds == nil {
		ui.refreshingFeeds = make(map[yt.FeedKind]bool)
	}
	ui.refreshingFeeds[kind] = true
	ui.refreshMu.Unlock()
	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		videos, err := source.Feed(ctx, kind)
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}

			ui.refreshMu.Lock()
			delete(ui.refreshingFeeds, kind)
			ui.refreshMu.Unlock()

			if err != nil {
				cleanErr := yt.SanitizeErrorOutput(err.Error())
				ui.accountError = cleanErr
				ui.setSpaceStatus(request.state, cleanErr)
				return
			}
			ui.accountError = ""

			if utility {
				if ui.showingUtility && ui.state.CurrentTab == tab {
					ui.state.SetVideos(videos)
					ui.state.Status = ""
				}
				return
			}
			if kind == yt.FeedWatchLater {
				videos = mergeVideos(videos, ui.store.WatchLater())
			}
			ui.setWatchVideos(videos)
			ui.setSpaceStatus(request.state, "")
		})
		ui.invalidate()
	}()
}

func (ui *UI) signInAccount() {
	if ui.account == nil || ui.accountSigningIn {
		return
	}
	spec := strings.TrimSpace(ui.accountBrowserEditor.Text())
	if spec == "" {
		spec = yt.DefaultBrowserSpec()
	}
	ui.account.SetBrowser(spec)
	ui.accountSigningIn = true
	ui.accountError = ""
	ui.state.Status = "Signing in…"

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := ui.account.SignIn(ctx)
		ui.sendAction(func() {
			ui.accountSigningIn = false
			if err != nil {
				cleanErr := yt.SanitizeErrorOutput(err.Error())
				ui.accountError = cleanErr
				ui.state.Status = cleanErr
			} else {
				ui.accountError = ""
				ui.state.Status = "Signed in"
				if ui.state.CurrentTab == TabHome {
					cached, _ := ui.account.Cached(yt.FeedHome)
					ui.state.SetVideos(cached)
					ui.refreshAccountFeed(yt.FeedHome)
				}
			}
		})
		ui.invalidate()
	}()
}

func (ui *UI) signOutAccount() {
	ui.clearAccountSpaceState(true, true)
	if ui.account == nil {
		return
	}
	_ = ui.account.SignOut()
	ui.accountError = ""
	ui.state.Status = "Signed out"
	ui.state.SavedPlaylists = nil
	ui.state.AccountChannels = nil
	ui.playlistsLoaded = false
	ui.reloadAllFeeds()
}

func (ui *UI) switchAccount(acc store.AccountConfig) {
	ui.clearAccountSpaceState(true, true)
	if ui.store == nil {
		return
	}
	switched, err := ui.store.SwitchAccount(acc.ID)
	if err != nil {
		ui.state.Status = fmt.Sprintf("Failed to switch account: %v", err)
		return
	}
	ui.settings = ui.store.Settings()
	if ui.account != nil {
		ui.account = yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: switched.CookieFile, CacheDir: switched.CacheDir, Browser: switched.BrowserSpec, BinaryPath: ui.account.BinaryPath})
		ui.accountBrowserEditor.SetText(switched.BrowserSpec)
	}
	ui.state.SavedPlaylists = nil
	ui.playlistsLoaded = false
	ui.state.AccountChannels = nil
	ui.reloadAllFeeds()
	ui.state.Status = fmt.Sprintf("Switched to account %s", switched.Label)
	ui.invalidate()
}

func (ui *UI) removeAccount(id string) {
	if ui.store == nil {
		return
	}
	wasActive := ui.settings.ActiveAccount == id
	if wasActive {
		ui.saveCurrentPosition()
	}
	if err := ui.store.RemoveAccount(id); err != nil {
		ui.state.Status = fmt.Sprintf("Failed to remove account: %v", err)
		return
	}
	ui.settings = ui.store.Settings()
	if wasActive {
		ui.clearAccountSpaceState(true, false)
		active := ui.store.ActiveAccount()
		if ui.account != nil {
			ui.account = yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: active.CookieFile, CacheDir: active.CacheDir, Browser: active.BrowserSpec, BinaryPath: ui.account.BinaryPath})
			ui.accountBrowserEditor.SetText(active.BrowserSpec)
		}
		ui.state.SavedPlaylists = nil
		ui.playlistsLoaded = false
		ui.state.AccountChannels = nil
		ui.reloadAllFeeds()
	}
	ui.invalidate()
}

func (ui *UI) reloadAllFeeds() {
	if ui.account != nil && ui.account.SignedIn() {
		switch ui.state.CurrentTab {
		case TabHome:
			cached, _ := ui.account.Cached(yt.FeedHome)
			ui.setWatchVideos(cached)
			ui.refreshAccountFeed(yt.FeedHome)
			if chs, _ := ui.account.CachedChannels(); len(chs) > 0 {
				ui.state.AccountChannels = chs
			}
			ui.refreshAccountChannels()
		case TabSubscriptions:
			cached, _ := ui.account.Cached(yt.FeedSubs)
			ui.setWatchVideos(cached)
			ui.refreshAccountFeed(yt.FeedSubs)
			if chs, _ := ui.account.CachedChannels(); len(chs) > 0 {
				ui.state.AccountChannels = chs
			}
			ui.refreshAccountChannels()
		case TabWatchLater:
			cached, _ := ui.account.Cached(yt.FeedWatchLater)
			ui.state.SetVideos(cached)
			ui.refreshAccountFeed(yt.FeedWatchLater)
		case TabHistory:
			cached, _ := ui.account.Cached(yt.FeedHistory)
			ui.state.SetVideos(cached)
			ui.refreshAccountFeed(yt.FeedHistory)
		case TabAccount:
			ui.loadSavedPlaylists()
		}
	} else {
		switch ui.state.CurrentTab {
		case TabHome:
			ui.loadMergedFeed()
		case TabSubscriptions:
			ui.state.SetVideos(nil)
		case TabWatchLater:
			if ui.store != nil {
				ui.state.SetVideos(ui.store.WatchLater())
			}
		case TabHistory:
			if ui.store != nil {
				ui.state.SetVideos(ui.store.History())
			}
		case TabAccount:
			ui.state.SavedPlaylists = nil
		}
	}
}

func (ui *UI) getAccountClicks(id string) (*widget.Clickable, *widget.Clickable, *widget.Clickable) {
	if ui.accountSwitchClicks == nil {
		ui.accountSwitchClicks = make(map[string]*widget.Clickable)
	}
	if ui.accountSignOutClicks == nil {
		ui.accountSignOutClicks = make(map[string]*widget.Clickable)
	}
	if ui.accountRemoveClicks == nil {
		ui.accountRemoveClicks = make(map[string]*widget.Clickable)
	}
	sw, ok := ui.accountSwitchClicks[id]
	if !ok {
		sw = new(widget.Clickable)
		ui.accountSwitchClicks[id] = sw
	}
	so, ok := ui.accountSignOutClicks[id]
	if !ok {
		so = new(widget.Clickable)
		ui.accountSignOutClicks[id] = so
	}
	rm, ok := ui.accountRemoveClicks[id]
	if !ok {
		rm = new(widget.Clickable)
		ui.accountRemoveClicks[id] = rm
	}
	return sw, so, rm
}

func (ui *UI) loadSavedPlaylists() {
	if ui.account == nil {
		return
	}

	if cached, err := ui.account.CachedPlaylists(); err == nil && len(cached) > 0 {
		ui.state.SavedPlaylists = cached
		if ui.debugPlaylist && len(cached) > 0 && ui.state.CurrentPlaylist.ID == "" {
			target := cached[0]
			for _, p := range cached {
				if strings.HasPrefix(p.ID, "PL") {
					target = p
					break
				}
			}
			ui.openPlaylist(target)
		}
	}

	if !ui.account.SignedIn() || ui.playlistsLoading || ui.playlistsLoaded {
		return
	}

	ui.playlistsLoading = true
	request := ui.beginSpaceRequest(ui.spaces.ActiveSpace, loadPlaylists)
	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		playlists, err := source.Playlists(ctx)
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			ui.playlistsLoading = false
			ui.playlistsLoaded = true
			if err != nil {
				cleanErr := yt.SanitizeErrorOutput(err.Error())
				ui.accountError = cleanErr
				return
			}
			ui.accountError = ""
			ui.state.SavedPlaylists = playlists
			if ui.debugPlaylist && len(playlists) > 0 && ui.state.CurrentPlaylist.ID == "" {
				target := playlists[0]
				for _, p := range playlists {
					if strings.HasPrefix(p.ID, "PL") {
						target = p
						break
					}
				}
				ui.openPlaylist(target)
			}
		})
		ui.invalidate()
	}()
}

func (ui *UI) openPlaylist(pl yt.Playlist) {
	ui.dropThumbnails()
	ui.state.CurrentTab = TabPlaylist
	ui.state.CurrentPlaylist = pl
	ui.state.SetVideos(nil)
	ui.state.Status = "Loading playlist…"

	targetURL := pl.URL
	if targetURL == "" && pl.ID != "" {
		targetURL = "https://www.youtube.com/playlist?list=" + pl.ID
	}

	request := ui.beginSpaceRequest(ui.spaces.ActiveSpace, loadLibrary)
	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var title string
		var videos []yt.Video
		var err error
		if source != nil {
			title, videos, err = source.PlaylistVideos(ctx, targetURL)
		} else {
			err = errors.New("no account configured")
		}

		ui.sendAction(func() {
			if !ui.requestCurrent(request) || ui.state.CurrentTab != TabPlaylist || ui.state.CurrentPlaylist.ID != pl.ID {
				return
			}
			if err != nil {
				cleanErr := yt.SanitizeErrorOutput(err.Error())
				ui.state.Status = cleanErr
				return
			}
			if title != "" {
				ui.state.CurrentPlaylist.Title = title
			}
			ui.state.SetVideos(videos)
			ui.state.Status = ""
		})
		ui.invalidate()
	}()
}

func extractThumbnailVideoID(thumbURL string) string {
	if idx := strings.Index(thumbURL, "/vi/"); idx != -1 {
		rest := thumbURL[idx+4:]
		if slash := strings.IndexByte(rest, '/'); slash != -1 {
			return rest[:slash]
		}
		if q := strings.IndexByte(rest, '?'); q != -1 {
			return rest[:q]
		}
		return rest
	}
	return ""
}

func (ui *UI) switchTab(t Tab) {
	if ui.state.CurrentTab != t {
		ui.dropThumbnails()
		if ui.state.CurrentTab == TabAccount {
			ui.playlistsLoaded = false
		}
	}
	ui.state.SwitchTab(t)
	ui.state.ActiveChannelFilter = ""
	ui.state.ActiveChannelTitle = ""
	switch t {
	case TabHome:
		ui.state.Subscriptions = ui.store.Subscriptions()
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedHome)
			ui.state.SetVideos(cached)
			ui.refreshAccountFeed(yt.FeedHome)
			if chs, _ := ui.account.CachedChannels(); len(chs) > 0 {
				ui.state.AccountChannels = chs
			}
			ui.refreshAccountChannels()
		} else {
			if len(ui.state.Videos) == 0 {
				ui.loadMergedFeed()
			}
		}
	case TabSubscriptions:
		ui.state.Subscriptions = ui.store.Subscriptions()
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedSubs)
			ui.state.SetVideos(cached)
			ui.refreshAccountFeed(yt.FeedSubs)
			if chs, _ := ui.account.CachedChannels(); len(chs) > 0 {
				ui.state.AccountChannels = chs
			}
			ui.refreshAccountChannels()
		} else {
			ui.state.SetVideos(nil)
		}
	case TabWatchLater:
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedWatchLater)
			ui.state.SetVideos(cached)
			ui.refreshAccountFeed(yt.FeedWatchLater)
		} else {
			ui.state.SetVideos(ui.store.WatchLater())
		}
	case TabHistory:
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedHistory)
			ui.state.SetVideos(cached)
			ui.refreshAccountFeed(yt.FeedHistory)
		} else {
			ui.state.SetVideos(ui.store.History())
		}
	case TabAccount:
		ui.loadSavedPlaylists()
		if len(ui.discoveredProfiles) == 0 {
			go func() {
				profs := yt.DiscoverBrowserProfiles()
				ui.sendAction(func() {
					ui.discoveredProfiles = profs
				})
				ui.invalidate()
			}()
		}
	case TabSearch:
		// Search results tab
	}
}

func (ui *UI) handleSearchSubmit() {
	if ui.suggestVisible && ui.suggestSelected >= 0 && ui.suggestSelected < len(ui.suggestItems) {
		selected := ui.suggestItems[ui.suggestSelected]
		ui.suggestVisible = false
		ui.suggestSelected = -1
		ui.searchEditor.SetText(selected)
		ui.search(selected)
		return
	}
	ui.suggestVisible = false
	ui.suggestSelected = -1

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
	ui.dropThumbnails()

	// Scoped search per space
	if ui.spaces != nil {
		switch ui.spaces.ActiveSpace {
		case SpaceShorts:
			ui.searchShorts(query)
			return
		case SpacePodcasts:
			ui.searchPodcasts(query)
			return
		case SpaceMusic:
			ui.searchMusic(query)
			return
		}
	}

	request := ui.beginSpaceRequest(SpaceWatch, loadSearch)
	request.state.Query = query
	ui.state.CurrentTab = TabSearch
	ui.state.Status = fmt.Sprintf("Searching %q…", query)

	if ui.demo {
		if ui.demoData != nil {
			q := strings.ToLower(query)
			var matched []yt.Video
			for _, v := range ui.demoData.Videos {
				if strings.Contains(strings.ToLower(v.Title), q) || strings.Contains(strings.ToLower(v.Channel), q) {
					matched = append(matched, v)
				}
			}
			ui.state.SetVideos(matched)
			if ui.spaces != nil && ui.spaces.ActiveSpace == SpaceWatch {
				state := ui.spaces.State(SpaceWatch)
				state.Videos = append([]yt.Video(nil), matched...)
				state.ContentGeneration = ui.currentGen()
				state.ContentLoaded = true
			}
			ui.state.Status = ""
		}
		ui.invalidate()
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		videos, err := ui.ytClient.Search(ctx, query)
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			if err != nil {
				ui.setSpaceStatus(request.state, "Search error: "+err.Error())
				return
			}
			ui.setWatchVideos(videos)
			ui.setSpaceStatus(request.state, "")
		})
		ui.invalidate()
	}()
}

func (ui *UI) searchShorts(query string) {
	ui.state.Status = "Searching Shorts…"
	state := ui.spaces.State(SpaceShorts)
	request := ui.beginSpaceRequest(SpaceShorts, loadSearch)
	state.Query = query

	if ui.demo {
		if ui.demoData != nil {
			q := strings.ToLower(query)
			var matched []yt.Video
			for _, s := range ui.demoData.Shorts {
				if strings.Contains(strings.ToLower(s.Title), q) || strings.Contains(strings.ToLower(s.Channel), q) {
					matched = append(matched, yt.Video{
						ID:          s.ID,
						Title:       s.Title,
						Channel:     s.Channel,
						Description: s.Caption,
						URL:         s.SourceLink,
					})
				}
			}
			state.Shorts = matched
			state.ContentLoaded = true
			state.ContentGeneration = ui.currentGen()
			ui.state.Status = ""
		}
		ui.invalidate()
		return
	}

	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		searchURL := yt.ShortsSearchURL(query)
		bin := "yt-dlp"
		if source != nil && source.BinaryPath != "" {
			bin = source.BinaryPath
		}
		args := []string{"--flat-playlist", "-J", searchURL}
		if source != nil && source.SignedIn() {
			if cookieCopy, err := source.PlaybackCookieFile(); err == nil {
				args = append([]string{"--cookies", cookieCopy}, args...)
			}
		}

		cmd := exec.CommandContext(ctx, bin, args...)
		out, err := cmd.Output()
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			if err != nil {
				ui.setSpaceStatus(request.state, "Shorts search error")
				return
			}
			vids, pErr := yt.ParseShortsSearchJSON(out)
			if pErr == nil {
				state.Shorts = vids
				state.ContentLoaded = true
				state.ContentGeneration = request.gen
			}
			ui.setSpaceStatus(request.state, "")
		})
		ui.invalidate()
	}()
}

func (ui *UI) searchPodcasts(query string) {
	ui.state.Status = fmt.Sprintf("Searching Podcasts for %q…", query)
	state := ui.spaces.State(SpacePodcasts)
	request := ui.beginSpaceRequest(SpacePodcasts, loadSearch)
	state.Query = query

	if ui.demo {
		if ui.demoData != nil {
			q := strings.ToLower(query)
			var matchedShows []yt.PodcastShow
			for _, s := range ui.demoData.Shows {
				if strings.Contains(strings.ToLower(s.Title), q) || strings.Contains(strings.ToLower(s.Creator), q) {
					matchedShows = append(matchedShows, yt.PodcastShow{
						ID:      s.ID,
						Title:   s.Title,
						Channel: s.Creator,
						Genre:   s.Genre,
					})
				}
			}
			state.Shows = matchedShows
			var matchedEps []yt.Video
			for _, e := range ui.demoData.Episodes {
				if strings.Contains(strings.ToLower(e.Title), q) || strings.Contains(strings.ToLower(e.Show), q) {
					matchedEps = append(matchedEps, yt.Video{
						ID:          e.ID,
						Title:       e.Title,
						Channel:     e.Show + " · " + e.Number,
						Duration:    e.Duration,
						Description: e.ProgressText,
					})
				}
			}
			state.Episodes = matchedEps
			state.PodcastPlaylists = nil
			state.ContentLoaded = true
			state.ContentGeneration = ui.currentGen()
			ui.state.Status = ""
		}
		ui.invalidate()
		return
	}

	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var shows []yt.PodcastShow
		var episodes []yt.Video
		var playlists []yt.Playlist
		var err error
		if source != nil && source.SignedIn() {
			shows, err = source.PodcastsFeed(ctx)
			if err == nil {
				q := strings.ToLower(query)
				for _, show := range shows {
					items, fetchErr := source.ShowEpisodes(ctx, show.URL)
					for i := range items {
						items[i].Channel = show.Title
					}
					if fetchErr == nil {
						for _, item := range items {
							if strings.Contains(strings.ToLower(item.Title), q) || strings.Contains(strings.ToLower(item.Channel), q) {
								episodes = append(episodes, item)
							}
						}
					}
				}
				filtered := shows[:0]
				for _, show := range shows {
					if strings.Contains(strings.ToLower(show.Title), q) || strings.Contains(strings.ToLower(show.Channel), q) {
						filtered = append(filtered, show)
					}
				}
				shows = filtered
			}
		}
		if source == nil {
			source = yt.NewAccount()
		}
		var discoveryErr error
		playlists, discoveryErr = source.SearchPodcastPlaylists(ctx, query)
		if err == nil {
			err = discoveryErr
		}
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			if err != nil {
				ui.setSpaceStatus(request.state, "Could not search podcasts.")
				return
			}
			state.Shows = shows
			state.Episodes = episodes
			state.PodcastPlaylists = playlists
			state.ContentLoaded = true
			state.ContentGeneration = request.gen
			ui.setSpaceStatus(request.state, "")
		})
		ui.invalidate()
	}()
}

func (ui *UI) searchMusic(query string) {
	ui.state.Status = fmt.Sprintf("Searching Music for %q…", query)
	state := ui.spaces.State(SpaceMusic)
	request := ui.beginSpaceRequest(SpaceMusic, loadSearch)
	state.Query = query

	if ui.demo {
		if ui.demoData != nil {
			q := strings.ToLower(query)
			var matchedTracks []yt.Video
			for _, t := range ui.demoData.Tracks {
				if strings.Contains(strings.ToLower(t.Title), q) || strings.Contains(strings.ToLower(t.Artist), q) || strings.Contains(strings.ToLower(t.Album), q) {
					matchedTracks = append(matchedTracks, yt.Video{
						ID:          t.ID,
						Title:       t.Title,
						Channel:     t.Artist,
						Duration:    t.Duration,
						Description: t.Album,
					})
				}
			}
			state.Tracks = matchedTracks
			state.LastMusicTracks = append([]yt.Video(nil), matchedTracks...)
			state.LastMusicAlbum = nil
			state.ContentLoaded = true
			state.ContentGeneration = ui.currentGen()
			ui.state.Status = ""
		}
		ui.invalidate()
		return
	}

	if ui.account == nil {
		return
	}

	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		album, tracks, err := source.SearchMusic(ctx, query)
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			if err != nil {
				ui.setSpaceStatus(request.state, "Music search error: "+err.Error())
				return
			}
			if album != nil {
				state.Album = album
				state.LastMusicAlbum = album
			}
			state.Tracks = tracks
			state.LastMusicTracks = append([]yt.Video(nil), tracks...)
			state.Queue = tracks
			state.ContentLoaded = true
			state.ContentGeneration = request.gen
			ui.setSpaceStatus(request.state, "")
		})
		ui.invalidate()
	}()
}

func (ui *UI) loadShortsSpaceContent(chip int) {
	state := ui.spaces.State(SpaceShorts)
	request := ui.beginSpaceRequest(SpaceShorts, loadContent)
	state.ContentLoaded = true
	state.ContentGeneration = ui.currentGen()
	if ui.demo && ui.demoData != nil {
		switch chip {
		case 0, 2:
			var vids []yt.Video
			for _, s := range ui.demoData.Shorts {
				vids = append(vids, yt.Video{
					ID:          s.ID,
					Title:       s.Title,
					Channel:     s.Channel,
					Description: s.Caption,
					URL:         s.SourceLink,
				})
			}
			state.Shorts = vids
		case 1:
			var vids []yt.Video
			for _, s := range ui.demoData.Shorts {
				if ui.demoData.SavedTitles[s.Title] {
					vids = append(vids, yt.Video{
						ID:          s.ID,
						Title:       s.Title,
						Channel:     s.Channel,
						Description: s.Caption,
						URL:         s.SourceLink,
					})
				}
			}
			state.Shorts = vids
		}
		return
	}

	// Live mode: chips Following (0), Saved (1), Explore (2)
	switch chip {
	case 0: // Following
		if ui.account != nil {
			ui.shortsLoading = true
			state.ShortsError = ""

			source := ui.account
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				vids, err := source.MergedShorts(ctx, 15)
				ui.sendAction(func() {
					if !ui.requestCurrent(request) {
						return
					}
					ui.shortsLoading = false
					state.Shorts = vids
					if err != nil {
						state.ShortsError = err.Error()
					}
				})
				ui.invalidate()
			}()
		}
	case 1: // Saved
		if ui.store != nil {
			wl := ui.store.WatchLater()
			var vids []yt.Video
			for _, v := range wl {
				if yt.IsShort(v, "") {
					vids = append(vids, v)
				}
			}
			state.Shorts = vids
		}
	case 2: // Explore: explicit unsupported state
		state.Shorts = nil
	}
}

func (ui *UI) loadPodcastsSpaceContent(chip int) {
	state := ui.spaces.State(SpacePodcasts)
	request := ui.beginSpaceRequest(SpacePodcasts, loadContent)
	state.ContentLoaded = true
	state.ContentGeneration = ui.currentGen()
	if ui.demo && ui.demoData != nil {
		var shows []yt.PodcastShow
		for _, s := range ui.demoData.Shows {
			shows = append(shows, yt.PodcastShow{
				ID:      s.ID,
				Title:   s.Title,
				Channel: s.Creator,
				Genre:   s.Genre,
			})
		}
		state.Shows = shows
		var eps []yt.Video
		for _, e := range ui.demoData.Episodes {
			eps = append(eps, yt.Video{
				ID:          e.ID,
				Title:       e.Title,
				Channel:     e.Show + " · " + e.Number,
				Duration:    e.Duration,
				Description: e.ProgressText,
			})
		}
		state.Episodes = eps
		return
	}

	state.Shows, state.Episodes, state.Queue = nil, nil, nil
	if chip == 2 {
		if ui.store != nil {
			for _, p := range ui.store.UnfinishedPositions(ui.currentAccountID()) {
				if p.Space == SpacePodcasts.String() && p.MediaKind == "episode" {
					state.Episodes = append(state.Episodes, yt.Video{ID: p.VideoID, Title: p.Title, Channel: p.Show, Duration: int(p.Duration)})
				}
			}
			state.Queue = append([]yt.Video(nil), state.Episodes...)
		}
		state.ContentGeneration = ui.currentGen()
		return
	}
	if ui.account == nil || !ui.account.SignedIn() {
		state.PodcastsError = "Sign in to load followed podcast shows."
		state.ContentGeneration = ui.currentGen()
		return
	}
	ui.podcastsLoading = true

	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		shows, err := source.PodcastsFeed(ctx)
		var episodes []yt.Video
		if err == nil && chip == 1 {
			for _, show := range shows {
				items, fetchErr := source.ShowEpisodes(ctx, show.URL)
				for i := range items {
					items[i].Channel = show.Title
				}
				if fetchErr == nil && len(items) > 0 {
					episodes = append(episodes, items[0])
				}
			}
		} else if err == nil && len(shows) > 0 {
			episodes, err = source.ShowEpisodes(ctx, shows[0].URL)
			for i := range episodes {
				episodes[i].Channel = shows[0].Title
			}
		}
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			ui.podcastsLoading = false
			state.Shows = shows
			if err == nil {
				state.Episodes = episodes
				state.Queue = append([]yt.Video(nil), episodes...)
				state.ContentGeneration = request.gen
				state.PodcastsError = ""
			} else {
				state.PodcastsError = "Could not load podcast shows or episodes."
			}
		})
		ui.invalidate()
	}()
}

func (ui *UI) loadPodcastShowEpisodes(show yt.PodcastShow) {
	ui.loadPodcastEpisodes(show, false)
}

func (ui *UI) loadPodcastEpisodes(show yt.PodcastShow, firstOnly bool) {
	state := ui.spaces.State(SpacePodcasts)
	request := ui.beginSpaceRequest(SpacePodcasts, loadContent)
	state.SelectedShow = &show
	if ui.demo {
		return
	}
	if ui.account == nil {
		return
	}

	ui.podcastsLoading = true
	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var episodes []yt.Video
		var err error
		if firstOnly {
			episodes, err = source.FirstShowEpisode(ctx, show.URL)
		} else {
			episodes, err = source.ShowEpisodes(ctx, show.URL)
		}
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			ui.podcastsLoading = false
			if err != nil {
				state.PodcastsError = "Could not load the selected show episodes."
			}
			if err == nil {
				for i := range episodes {
					episodes[i].Channel = show.Title
				}
				state.Episodes = episodes
				state.Queue = episodes
			}
		})
		ui.invalidate()
	}()
}

func (ui *UI) loadMusicSpaceContent(chip int) {
	state := ui.spaces.State(SpaceMusic)
	request := ui.beginSpaceRequest(SpaceMusic, loadContent)
	state.ContentLoaded = true
	state.ContentGeneration = ui.currentGen()
	if ui.demo && ui.demoData != nil {
		state.Album = &yt.Album{
			Title:  ui.demoData.Album.Title,
			Artist: ui.demoData.Album.Artist,
		}
		var tracks []yt.Video
		for _, t := range ui.demoData.Tracks {
			tracks = append(tracks, yt.Video{
				ID:          t.ID,
				Title:       t.Title,
				Channel:     t.Artist,
				Duration:    t.Duration,
				Description: t.Album,
			})
		}
		state.Tracks = tracks
		state.Queue = tracks
		return
	}

	state.Tracks, state.Queue, state.Album = nil, nil, nil
	if chip == 1 {
		if state.LastMusicAlbum != nil {
			state.Album = state.LastMusicAlbum
		}
		state.Tracks = append([]yt.Video(nil), state.LastMusicTracks...)
		state.Queue = append([]yt.Video(nil), state.Tracks...)
		state.ContentGeneration = ui.currentGen()
		return
	}
	if chip == 2 {
		if ui.store != nil {
			albums := ui.store.SavedCollections(ui.collectionAccountID())
			if len(albums) > 0 {
				state.Album = &albums[0]
				state.Tracks = append([]yt.Video(nil), albums[0].Tracks...)
			}
			state.Queue = append([]yt.Video(nil), state.Tracks...)
		}
		state.ContentGeneration = ui.currentGen()
		return
	}
	if ui.account == nil || !ui.account.SignedIn() {
		state.ContentGeneration = ui.currentGen()
		return
	}
	ui.musicLoading = true

	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		tracks, err := source.LikedMusic(ctx)
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			ui.musicLoading = false
			if err == nil {
				state.Tracks, state.Queue = tracks, append([]yt.Video(nil), tracks...)
				state.ContentGeneration = request.gen
			}
		})
		ui.invalidate()
	}()
}

func (ui *UI) loadMergedFeed() {
	request := ui.beginSpaceRequest(SpaceWatch, loadContent)
	subs := ui.store.Subscriptions()
	ui.state.Subscriptions = subs
	if len(subs) == 0 {
		ui.setWatchVideos(nil)
		ui.state.Status = "No subscriptions yet. Add channels in Subscriptions tab or import CSV."
		return
	}

	ui.state.Status = "Loading subscription feed…"
	ui.startupFeedPending = true
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
			ui.startupFeedPending = false
			if !ui.requestCurrent(request) {
				return
			}
			if err != nil {
				ui.state.Status = "Feed error: " + err.Error()
				return
			}
			ui.setWatchVideos(videos)
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
		ui.invalidate()
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
			ui.invalidate()
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
		ui.invalidate()
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
			ui.invalidate()
			return
		}
		defer f.Close()

		subs, err := yt.ParseTakeoutCSV(f)
		if err != nil {
			ui.sendAction(func() {
				ui.state.Status = "Failed parsing CSV: " + err.Error()
			})
			ui.invalidate()
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
		ui.invalidate()
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
		if ui.debug {
			fmt.Fprintln(os.Stderr, "space-play-route=external")
		}
		ui.playVideoExternal(v, audio)
		return
	}
	if ui.spaces != nil && ui.state.CurrentTab == TabHome && ui.spaces.ActiveSpace != SpaceWatch {
		ui.playSpaceVideo(v)
		return
	}
	ui.openWatchPage(v)
}

func (ui *UI) playSpaceVideo(v yt.Video) {
	if ui.demo {
		return
	}
	ui.saveCurrentPosition()
	_ = ui.store.AddHistory(v)
	ui.state.Status = ""
	ui.watchVideo = v

	if ui.spaces != nil {
		ui.spaces.State(ui.spaces.ActiveSpace).ResumeItem = &v
	}

	if !ui.view.Valid() && ui.embedPlayer == nil {
		if ui.debug {
			fmt.Fprintln(os.Stderr, "space-play-route=external-no-view")
		}
		ui.playVideoExternal(v, false)
		ui.state.Status = "Embedded player unavailable on this display; playing with external mpv"
		return
	}

	if ui.embedPlayer == nil {
		p, err := ui.createEmbedPlayer()
		if err != nil {
			if ui.debug {
				fmt.Fprintln(os.Stderr, "space-play-route=external-create-error")
			}
			ui.playVideoExternal(v, false)
			ui.state.Status = fmt.Sprintf("Embedded player unavailable (%v); playing with external mpv", err)
			return
		}
		ui.embedPlayer = p
	}

	cookiePath := ""
	if ui.account != nil && ui.account.SignedIn() {
		cookiePath, _ = ui.account.PlaybackCookieFile()
	}

	scale := ui.currentScale
	if scale <= 0 {
		scale = 2
	}

	codecPol := player.CodecPolicy(ui.settings.CodecPolicy)
	formatStr := player.FormatFor(ui.power, player.ModeVideo, codecPol, ui.cachedCaps)
	hwdec := player.HWDec(codecPol, ui.cachedCaps)

	err := ui.embedPlayer.Play(embed.PlayOptions{
		URL:         v.WatchURL(),
		Title:       v.Title,
		YTDLFormat:  formatStr,
		CookiesFile: cookiePath,
		HWDec:       hwdec,
		WidthPx:     1280,
		HeightPx:    720,
		Scale:       scale,
		Debug:       ui.debug,
	})
	if err != nil {
		if ui.debug {
			fmt.Fprintln(os.Stderr, "space-play-result=error")
		}
		ui.state.Status = "Playback error: " + err.Error()
		return
	}
	if ui.debug {
		fmt.Fprintln(os.Stderr, "space-play-result=embedded")
	}
	ui.watchPaused = false
	ui.watchCurrentTime = 0
	ui.watchDuration = float64(v.Duration)
	ui.watchLastMouseMove = time.Now()
	ui.hwdecLogged = false
	ui.lastTimePoll = time.Now()

	if ui.spaces != nil && ui.spaces.ActiveSpace == SpaceShorts {
		ui.setPlayerMuted(!ui.shortsSoundOn)
	}
	if ui.spaces != nil {
		ui.playerOwnerSpace = ui.spaces.ActiveSpace
		ui.playerOwnerID = v.ID
	}
	ui.applyResumePosition(v)

	ui.invalidate()
}

func (ui *UI) setPlayerMuted(muted bool) {
	if ui.embedPlayer == nil {
		return
	}
	if !ui.playerMuteKnown {
		ui.playerMuted = ui.embedPlayer.Status().Muted
		ui.playerMuteKnown = true
	}
	if ui.playerMuted != muted {
		ui.embedPlayer.ToggleMute()
		ui.playerMuted = muted
	}
	ui.watchMuted = muted
	if ui.spaces != nil && ui.spaces.ActiveSpace == SpaceShorts {
		ui.shortsSoundOn = !muted
	}
}

func (ui *UI) togglePlayerMute() {
	if ui.embedPlayer == nil {
		return
	}
	if !ui.playerMuteKnown {
		ui.playerMuted = ui.embedPlayer.Status().Muted
		ui.playerMuteKnown = true
	}
	ui.embedPlayer.ToggleMute()
	ui.playerMuted = !ui.playerMuted
	ui.watchMuted = ui.playerMuted
	if ui.spaces != nil && ui.spaces.ActiveSpace == SpaceShorts {
		ui.shortsSoundOn = !ui.playerMuted
	}
}

func (ui *UI) applyResumePosition(v yt.Video) {
	if ui.store == nil || ui.embedPlayer == nil || v.ID == "" {
		return
	}
	pos, ok := ui.store.Position(ui.currentAccountID(), v.ID)
	if !ok || !pos.IsUnfinished() {
		return
	}
	ui.pendingResumeID = v.ID
	ui.pendingResumeSeconds = pos.Seconds
	ui.embedPlayer.Seek(pos.Seconds, false)
}

// EmbedPlayerController defines embedded player operations.
type EmbedPlayerController interface {
	Play(opts embed.PlayOptions) error
	Stop()
	SetRect(rect image.Rectangle, scale int)
	SetHidden(hidden bool)
	TogglePause()
	SetPause(paused bool)
	Seek(seconds float64, relative bool)
	SetVolume(volume float64)
	ToggleMute()
	Status() embed.Status
	Close()
}

func (ui *UI) registerVideoBox(box image.Rectangle) {
	ui.activeVideoBox = box
	ui.videoBoxLaidOut = true
}

func (ui *UI) label(size unit.Sp, txt string) material.LabelStyle {
	if ui.labelHook != nil {
		ui.labelHook(txt)
	}
	ui.fonts.ensure(ui.th, txt)
	return material.Label(ui.th, size, txt)
}

func (ui *UI) editor(editor *widget.Editor, hint string) material.EditorStyle {
	ui.fonts.ensure(ui.th, hint)
	ui.fonts.ensure(ui.th, editor.Text())
	return material.Editor(ui.th, editor, hint)
}

func (ui *UI) button(button *widget.Clickable, label string) material.ButtonStyle {
	ui.fonts.ensure(ui.th, label)
	return material.Button(ui.th, button, label)
}

func (ui *UI) oneLineLabel(size unit.Sp, txt string) material.LabelStyle {
	lbl := ui.label(size, txt)
	lbl.MaxLines = 1
	return lbl
}

// drawCoverImage scales an image to cover targetSize, center-crops it, clips to a rounded rectangle with radius,
// and returns exactly the target dimensions.
func (ui *UI) drawCoverImage(gtx layout.Context, img image.Image, targetSize image.Point, radius unit.Dp) layout.Dimensions {
	w := targetSize.X
	h := targetSize.Y
	if w <= 0 || h <= 0 {
		return layout.Dimensions{}
	}

	gtx.Constraints = layout.Exact(targetSize)
	r := clip.UniformRRect(image.Rectangle{Max: targetSize}, gtx.Dp(radius))

	if img == nil {
		paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
		return layout.Dimensions{Size: targetSize}
	}

	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
		return layout.Dimensions{Size: targetSize}
	}

	scaleX := float32(w) / float32(srcW)
	scaleY := float32(h) / float32(srcH)
	scale := scaleX
	if scaleY > scaleX {
		scale = scaleY
	}

	scaledW := float32(srcW) * scale
	scaledH := float32(srcH) * scale
	offsetX := (float32(w) - scaledW) / 2.0
	offsetY := (float32(h) - scaledH) / 2.0

	cl := r.Op(gtx.Ops).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))

	offOp := op.Offset(image.Pt(int(offsetX), int(offsetY))).Push(gtx.Ops)
	aff := f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scale, scale))
	affOp := op.Affine(aff).Push(gtx.Ops)

	imgOp := paint.NewImageOp(img)
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	affOp.Pop()
	offOp.Pop()
	cl.Pop()

	return layout.Dimensions{Size: targetSize}
}

// layoutPillButton renders a pill-shaped action or outline button with at least 44 dp height.
func (ui *UI) layoutPillButton(gtx layout.Context, click *widget.Clickable, text string, primary bool) layout.Dimensions {
	if click.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}

	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		minH := gtx.Dp(44)
		radius := gtx.Dp(22)

		m := op.Record(gtx.Ops)
		lblDims := layout.Inset{
			Top:    unit.Dp(10),
			Bottom: unit.Dp(10),
			Left:   unit.Dp(20),
			Right:  unit.Dp(20),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(13), text)
			if primary {
				lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			} else {
				lbl.Color = ui.colors.Text
			}
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		})
		textCall := m.Stop()

		btnW := lblDims.Size.X
		btnH := lblDims.Size.Y
		if btnH < minH {
			btnH = minH
		}
		btnSize := image.Pt(btnW, btnH)

		r := clip.UniformRRect(image.Rectangle{Max: btnSize}, radius)
		if primary {
			paint.FillShape(gtx.Ops, ui.colors.Action, r.Op(gtx.Ops))
		} else {
			paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
		}

		textOffsetY := (btnH - lblDims.Size.Y) / 2
		off := op.Offset(image.Pt(0, textOffsetY)).Push(gtx.Ops)
		textCall.Add(gtx.Ops)
		off.Pop()

		return layout.Dimensions{Size: btnSize}
	})
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

	var extraArgs []string
	if ui.account != nil && ui.account.SignedIn() {
		extraArgs = ui.account.YtDlpArgs()
	}

	opts := player.Options{
		URL:       v.WatchURL(),
		Title:     v.Title,
		Audio:     audio,
		Policy:    player.CodecPolicy(ui.settings.CodecPolicy),
		Caps:      ui.cachedCaps,
		ExtraArgs: extraArgs,
	}

	go func() {
		_, err := ui.player.Start(opts)
		if err != nil {
			ui.sendAction(func() {
				ui.state.Status = "Playback error: " + err.Error()
			})
			ui.invalidate()
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

func (ui *UI) dropThumbnails() {
	ui.clearHover()
	if ui.thumbs != nil {
		ui.thumbs.Clear()
	}
	ui.imageOps = make(map[string]paint.ImageOp)
	ui.requestedMu.Lock()
	ui.requested = make(map[string]bool)
	ui.requestedMu.Unlock()
}

func (ui *UI) requestThumbnail(vid string) {
	if vid == "" || ui.thumbs == nil {
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
		ui.invalidate()
	})
}

func (ui *UI) layout(gtx layout.Context) layout.Dimensions {
	ui.activeVideoBox = image.Rectangle{}
	ui.videoBoxLaidOut = false
	ui.maybeTriggerDebugPlayFirst()
	ui.drainActions()
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
	// Key events go to the first gtx.Event call whose filter matches, and this
	// runs before the editors read theirs. So while an editor has focus, ask
	// only for Ctrl (Cmd) shortcuts and Esc; else Enter and the arrows never reach it.
	editorFocused := gtx.Focused(&ui.searchEditor) || gtx.Focused(&ui.addChannelEditor) || gtx.Focused(&ui.csvPathEditor) || gtx.Focused(&ui.accountBrowserEditor)
	searchEditorFocused := gtx.Focused(&ui.searchEditor)
	filters := []event.Filter{
		key.Filter{Name: "1", Required: key.ModShortcut},
		key.Filter{Name: "2", Required: key.ModShortcut},
		key.Filter{Name: "3", Required: key.ModShortcut},
		key.Filter{Name: "4", Required: key.ModShortcut},
		key.Filter{Name: "5", Required: key.ModShortcut},
		key.Filter{Name: "1", Required: key.ModAlt},
		key.Filter{Name: "2", Required: key.ModAlt},
		key.Filter{Name: "3", Required: key.ModAlt},
		key.Filter{Name: "4", Required: key.ModAlt},
		key.Filter{Name: "L", Required: key.ModShortcut},
		key.Filter{Name: "l", Required: key.ModShortcut},
		key.Filter{Name: "V", Required: key.ModShortcut},
		key.Filter{Name: "v", Required: key.ModShortcut},
		key.Filter{Name: key.NameEscape},
	}
	if searchEditorFocused && ui.suggestVisible && len(ui.suggestItems) > 0 {
		filters = append(filters,
			key.Filter{Focus: &ui.searchEditor, Name: key.NameUpArrow},
			key.Filter{Focus: &ui.searchEditor, Name: key.NameDownArrow},
		)
	}
	if !editorFocused {
		filters = append(filters,
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
			key.Filter{Name: "O"},
			key.Filter{Name: "o"},
		)
	}
	event.Op(gtx.Ops, ui.windowTag)
	for {
		evt, ok := gtx.Event(filters...)
		if !ok {
			break
		}

		if ke, ok := evt.(key.Event); ok && ke.State == key.Press {
			if searchEditorFocused && ui.suggestVisible {
				switch ke.Name {
				case key.NameDownArrow:
					ui.suggestSelected++
					if ui.suggestSelected >= len(ui.suggestItems) {
						ui.suggestSelected = len(ui.suggestItems) - 1
					}
					continue
				case key.NameUpArrow:
					ui.suggestSelected--
					if ui.suggestSelected < -1 {
						ui.suggestSelected = -1
					}
					continue
				case key.NameEscape:
					ui.suggestVisible = false
					ui.suggestSelected = -1
					continue
				}
			}

			if ke.Name == key.NameEscape {
				if ui.state.CurrentTab == TabPlaylist {
					ui.switchTab(TabAccount)
					continue
				}
			}

			isEditorFocused := editorFocused
			if ui.state.CurrentTab == TabWatch {
				action, tab, delta := ResolveWatchKeyAction(ke.Name, ke.Modifiers, isEditorFocused)
				switch action {
				case ActionSwitchSpace:
					ui.closeWatchPage()
					ui.switchSpace(SpaceID(tab))
				case ActionSwitchTab:
					ui.closeWatchPage()
					ui.switchTab(tab)
				case ActionFocusSearch:
					gtx.Execute(key.FocusCmd{Tag: &ui.searchEditor})
				case ActionPasteURL:
					gtx.Execute(clipboard.ReadCmd{Tag: ui.pasteTag})
				case ActionWatchTogglePause:
					ui.togglePlaybackPause()
				case ActionWatchSeek:
					if ui.embedPlayer != nil {
						ui.embedPlayer.Seek(float64(delta), true)
					}
				case ActionWatchToggleMute:
					ui.togglePlayerMute()
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
				case ActionSwitchSpace:
					ui.switchSpace(SpaceID(tab))
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

	ui.videoBoxLaidOut = false
	ui.activeVideoBox = image.Rectangle{}

	// Paint bg itself (opaque), so it does not depend on compositor background.
	paint.FillShape(gtx.Ops, ui.colors.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	dims := layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if ui.watchFullscreen {
				return ui.layoutWatchPage(gtx)
			}

			winW := gtx.Constraints.Max.X
			isMobile := winW < gtx.Dp(600)

			if isMobile {
				var topBarH int
				var headingH, chipsH int
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := ui.layoutTopBar(gtx)
						topBarH = dims.Size.Y
						return dims
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if ui.state.CurrentTab == TabWatch {
							ui.spaceOrigin = image.Pt(0, topBarH)
							ui.spaceSize = image.Pt(winW, gtx.Constraints.Max.Y)
							return ui.layoutWatchPage(gtx)
						}
						if ui.showingUtility || ui.state.CurrentTab != TabHome {
							ui.spaceOrigin = image.Pt(0, topBarH)
							ui.spaceSize = image.Pt(winW, gtx.Constraints.Max.Y)
							return ui.layoutMainArea(gtx)
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								dims := ui.layoutPerSpaceHeading(gtx)
								headingH = dims.Size.Y
								return dims
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								dims := ui.layoutChips(gtx)
								chipsH = dims.Size.Y
								return dims
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								ui.spaceOrigin = image.Pt(0, topBarH+headingH+chipsH)
								ui.spaceSize = image.Pt(winW, gtx.Constraints.Max.Y)
								switch ui.spaces.ActiveSpace {
								case SpaceWatch:
									return ui.layoutWatchSpace(gtx)
								case SpaceShorts:
									return ui.layoutShortsSpace(gtx)
								case SpacePodcasts:
									return ui.layoutPodcastsSpace(gtx)
								case SpaceMusic:
									return ui.layoutMusicSpace(gtx)
								default:
									return ui.layoutWatchSpace(gtx)
								}
							}),
						)
					}),
					layout.Rigid(ui.layoutBottomBar),
				)
			}

			// Desktop / Tablet shell: Rail (left) + Right Column
			var railW int
			var topBarH int
			var headingH, chipsH int
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := ui.layoutRail(gtx)
					railW = dims.Size.X
					return dims
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					colW := winW - railW
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							dims := ui.layoutTopBar(gtx)
							topBarH = dims.Size.Y
							return dims
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if ui.state.CurrentTab == TabWatch {
								ui.spaceOrigin = image.Pt(railW, topBarH)
								ui.spaceSize = image.Pt(colW, gtx.Constraints.Max.Y)
								return ui.layoutWatchPage(gtx)
							}
							if ui.showingUtility || ui.state.CurrentTab != TabHome {
								ui.spaceOrigin = image.Pt(railW, topBarH)
								ui.spaceSize = image.Pt(colW, gtx.Constraints.Max.Y)
								return ui.layoutMainArea(gtx)
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									dims := ui.layoutPerSpaceHeading(gtx)
									headingH = dims.Size.Y
									return dims
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									dims := ui.layoutChips(gtx)
									chipsH = dims.Size.Y
									return dims
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									ui.spaceOrigin = image.Pt(railW, topBarH+headingH+chipsH)
									ui.spaceSize = image.Pt(colW, gtx.Constraints.Max.Y)
									switch ui.spaces.ActiveSpace {
									case SpaceWatch:
										return ui.layoutWatchSpace(gtx)
									case SpaceShorts:
										return ui.layoutShortsSpace(gtx)
									case SpacePodcasts:
										return ui.layoutPodcastsSpace(gtx)
									case SpaceMusic:
										return ui.layoutMusicSpace(gtx)
									default:
										return ui.layoutWatchSpace(gtx)
									}
								}),
							)
						}),
						layout.Rigid(ui.layoutStatusBar),
					)
				}),
			)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if !ui.suggestVisible || len(ui.suggestItems) == 0 {
				return layout.Dimensions{}
			}
			return ui.layoutSuggestionsDropdown(gtx)
		}),
	)

	if ui.embedPlayer != nil {
		if !ui.videoBoxLaidOut || ui.activeVideoBox.Empty() {
			ui.embedPlayer.SetRect(image.Rectangle{}, ui.currentScale)
			ui.embedPlayer.SetHidden(true)
			if !ui.watchPaused {
				ui.pausePlayback()
			}
		} else {
			ui.embedPlayer.SetHidden(false)
			if ui.debug && ui.activeVideoBox != ui.lastLoggedVideoBox {
				fmt.Fprintf(os.Stderr, "video-box-px=%v scale=%d\n", ui.activeVideoBox, ui.currentScale)
				ui.lastLoggedVideoBox = ui.activeVideoBox
			}
			ui.embedPlayer.SetRect(ui.activeVideoBox, ui.currentScale)
		}
	}

	return dims
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
	// Detect search text changes
	currentSearchText := ui.searchEditor.Text()
	if currentSearchText != ui.lastSearchText {
		ui.lastSearchText = currentSearchText
		trimmed := strings.TrimSpace(currentSearchText)
		if gtx.Focused(&ui.searchEditor) && !yt.IsURLQuery(trimmed) {
			ui.suggestMgr.Input(trimmed)
		} else {
			ui.suggestVisible = false
			ui.suggestItems = nil
			ui.suggestMgr.Cancel()
		}
	}
	if ui.searchBtn.Clicked(gtx) {
		ui.handleSearchSubmit()
	}
	if ui.searchClearClick.Clicked(gtx) {
		ui.searchEditor.SetText("")
		ui.lastSearchText = ""
		ui.suggestVisible = false
		ui.suggestItems = nil
		ui.suggestMgr.Cancel()
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

	scopeLabel := SearchScopeLabel(ui.spaces.ActiveSpace)
	if ui.showingUtility {
		scopeLabel = "Library"
	}

	isNarrow := winW < gtx.Dp(600)

	padX := unit.Dp(16)
	if isNarrow {
		padX = unit.Dp(12)
	}

	return layout.Inset{
		Left:   padX,
		Right:  padX,
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			// 1. Left element: Back button when on TabWatch, Brand mark in mobile (<600dp), Space scope label on wide desktop (>=760dp)
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if ui.state.CurrentTab == TabWatch {
					return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if ui.watchBackBtn.Clicked(gtx) {
							ui.closeWatchPage()
						}
						if ui.watchBackBtn.Hovered() {
							pointer.CursorPointer.Add(gtx.Ops)
						}
						btn := ui.button(&ui.watchBackBtn, "← Back")
						btn.Background = ui.colors.Surface
						btn.Color = ui.colors.Text
						btn.CornerRadius = unit.Dp(16)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
						return btn.Layout(gtx)
					})
				}
				if isNarrow {
					return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return ui.logoClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if ui.logoClick.Hovered() {
								pointer.CursorPointer.Add(gtx.Ops)
							}
							return ui.drawBrandMark(gtx)
						})
					})
				}
				if !showChip {
					return layout.Dimensions{}
				}
				lbl := ui.label(unit.Sp(14), scopeLabel)
				lbl.Color = ui.colors.Text2
				lbl.Font.Weight = font.Medium
				return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(16)}.Layout(gtx, lbl.Layout)
			}),

			// 2. Search flexed (min 140, max 640, centered in free space)
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				freeW := gtx.Constraints.Max.X
				minW := gtx.Dp(140)
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

			// 3. Right element: Library + Profile buttons in mobile (<600dp), Profile circle on desktop
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if isNarrow {
					return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							// Library button
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return ui.topBarLibraryClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									if ui.topBarLibraryClick.Hovered() {
										pointer.CursorPointer.Add(gtx.Ops)
									}
									if ui.topBarLibraryClick.Clicked(gtx) {
										ui.openUtility(TabWatchLater)
									}
									sz := gtx.Dp(32)
									r := clip.UniformRRect(image.Rect(0, 0, sz, sz), sz/2)
									paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										iconBox := image.Rect(0, 0, gtx.Dp(16), gtx.Dp(16))
										drawLibraryIcon(gtx, iconBox, ui.colors.Text)
										return layout.Dimensions{Size: iconBox.Size()}
									})
								})
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
							// Profile / You button
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return ui.topBarProfileClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									if ui.topBarProfileClick.Hovered() {
										pointer.CursorPointer.Add(gtx.Ops)
									}
									if ui.topBarProfileClick.Clicked(gtx) {
										ui.openUtility(TabAccount)
									}
									return ui.layoutAvatar(gtx, "", "You", 32)
								})
							}),
						)
					})
				}
				if !showChip {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.topBarProfileClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if ui.topBarProfileClick.Hovered() {
							pointer.CursorPointer.Add(gtx.Ops)
						}
						if ui.topBarProfileClick.Clicked(gtx) {
							ui.openUtility(TabAccount)
						}
						return ui.layoutAvatar(gtx, "", "You", 32)
					})
				})
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
					placeholder := SearchPlaceholder(ui.spaces.ActiveSpace)
					if ui.showingUtility {
						placeholder = "Search YouTube"
					}
					ed := ui.editor(&ui.searchEditor, placeholder)
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
				lbl := ui.label(unit.Sp(12), policyText)
				lbl.Color = ui.colors.Text2
				return lbl.Layout(gtx)
			})
		},
	)
}

// layoutMiniGuide renders the 72dp wide sidebar with 5 items:
// Home, Subscriptions, Watch later, History, You (Account).
// Vector icons: house, stacked rectangles, clock, counter-clockwise arrow, person.
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
		{TabAccount, "You", drawPersonIcon},
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
										lbl := ui.label(unit.Sp(10), item.label)
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
	case TabAccount:
		return ui.layoutAccountView(gtx)
	case TabHome:
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(ui.layoutChipsRow),
			layout.Flexed(1, ui.layoutCardGrid),
		)
	case TabPlaylist:
		return ui.layoutPlaylistView(gtx)
	case TabWatchLater, TabHistory:
		return ui.layoutCardGrid(gtx)
	default:
		return ui.layoutCardGrid(gtx)
	}
}

// layoutChipsRow renders channel filter chips on Home:
// "All" plus one chip per account channel (when signed in) or per subscribed channel.
func (ui *UI) layoutChipsRow(gtx layout.Context) layout.Dimensions {
	var chipTitles []string
	var chipIDs []string

	if ui.account != nil && ui.account.SignedIn() && len(ui.state.AccountChannels) > 0 {
		chipTitles = make([]string, len(ui.state.AccountChannels))
		chipIDs = make([]string, len(ui.state.AccountChannels))
		for i, ch := range ui.state.AccountChannels {
			chipTitles[i] = ch.Title
			chipIDs[i] = ch.ID
		}
	} else {
		subs := ui.state.Subscriptions
		chipTitles = make([]string, len(subs))
		chipIDs = make([]string, len(subs))
		for i, sub := range subs {
			t := sub.Title
			if t == "" {
				t = sub.ChannelID
			}
			chipTitles[i] = t
			chipIDs[i] = sub.ChannelID
		}
	}

	totalChips := 1 + len(chipTitles)

	for len(ui.channelChipClicks) < len(chipTitles) {
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
				title = chipTitles[i-1]
				isSelected = ui.state.ActiveChannelFilter == chipIDs[i-1]
				click = &ui.channelChipClicks[i-1]
			}

			if click.Clicked(gtx) {
				if i == 0 {
					ui.state.ActiveChannelFilter = ""
					ui.state.ActiveChannelTitle = ""
					ui.state.SelectedIndex = 0
					if ui.account != nil && ui.account.SignedIn() {
						cached, _ := ui.account.Cached(yt.FeedHome)
						ui.state.SetVideos(cached)
					} else {
						ui.loadMergedFeed()
					}
				} else {
					ui.selectChannel(chipIDs[i-1], chipTitles[i-1])
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
								lbl := ui.label(unit.Sp(13), title)
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

	// Thumbnail prefetching: visible rows plus 1 row margin.
	firstRow := ui.gridList.Position.First
	countRows := ui.gridList.Position.Count
	if countRows == 0 {
		countRows = 4
	}
	startRow := firstRow - 1
	if startRow < 0 {
		startRow = 0
	}
	endRow := firstRow + countRows + 1
	if endRow > numRows {
		endRow = numRows
	}
	startIdx := startRow * columns
	endIdx := endRow * columns
	if endIdx > len(videos) {
		endIdx = len(videos)
	}

	// Clamp total requested thumbnails so they never exceed half the budget.
	if endIdx-startIdx > thumbs.MaxThumbnailsInHalfBudget {
		endIdx = startIdx + thumbs.MaxThumbnailsInHalfBudget
		if endIdx > len(videos) {
			endIdx = len(videos)
		}
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
		if ui.state.ActiveChannelTitle != "" {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(20), ui.state.ActiveChannelTitle)
					lbl.Font.Weight = font.Bold
					lbl.Color = ui.colors.Text
					return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, lbl.Layout)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ui.gridList.Layout(gtx, numRows, func(gtx layout.Context, rowIdx int) layout.Dimensions {
						return layout.Inset{Bottom: unit.Dp(36)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								ui.layoutGridCards(gtx, rowIdx, columns, cardWidthPx, thumbHeightPx, videos)...,
							)
						})
					})
				}),
			)
		}

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
	if ui.debugHover >= 0 && ui.debugHover == index {
		if ui.debugHoverSeconds <= 0 || time.Since(ui.startTime) < time.Duration(ui.debugHoverSeconds)*time.Second {
			isHovered = true
		}
	}
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
									return ui.layoutAvatar(gtx, v.ChannelID, v.Channel, 36)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								// Text metadata
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											title := ui.label(unit.Sp(16), v.Title)
											title.Font.Weight = font.Medium
											title.Color = ui.colors.Text
											title.MaxLines = 2
											title.Truncator = "…"
											return title.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											ch := ui.label(unit.Sp(14), v.Channel)
											ch.Color = ui.colors.Text2
											ch.MaxLines = 1
											ch.Truncator = "…"
											return ch.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											metaText := FormatMetaLine(v.ViewCount, v.UploadDate, v.PublishedAt, time.Now())
											meta := ui.label(unit.Sp(14), metaText)
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

	// Hover storyboard preview state logic
	if isHovered {
		if ui.hoverVideoID != v.ID {
			ui.startHover(v.ID, -1)
		}
		if ui.hoverVideoID == v.ID {
			if len(ui.hoverFrames) == 0 {
				elapsed := gtx.Now.Sub(ui.hoverStartTime)
				if elapsed >= 500*time.Millisecond {
					if !ui.hoverFetchStarted {
						ui.hoverFetchStarted = true
						ctx, cancel := context.WithCancel(context.Background())
						ui.hoverCancel = cancel
						go ui.loadStoryboardPreview(ctx, v.ID)
					}
				} else {
					gtx.Execute(op.InvalidateCmd{At: ui.hoverStartTime.Add(500 * time.Millisecond)})
				}
			} else {
				if ui.hoverLastFrameTime.IsZero() {
					ui.hoverLastFrameTime = gtx.Now
				}
				frameElapsed := gtx.Now.Sub(ui.hoverLastFrameTime)
				if frameElapsed >= 400*time.Millisecond {
					adv := int(frameElapsed / (400 * time.Millisecond))
					ui.hoverFrameIdx = (ui.hoverFrameIdx + adv) % len(ui.hoverFrames)
					ui.hoverLastFrameTime = gtx.Now
				}
				gtx.Execute(op.InvalidateCmd{At: ui.hoverLastFrameTime.Add(400 * time.Millisecond)})
			}
		}
	} else {
		if ui.hoverVideoID == v.ID {
			ui.clearHover()
		}
	}

	var imgOp paint.ImageOp
	hasImg := false

	if isHovered && ui.hoverVideoID == v.ID && len(ui.hoverFrames) > 0 {
		imgOp = ui.hoverOps[ui.hoverFrameIdx]
		hasImg = true
	} else if ui.demo {
		filename := demoFilenameForID(v.ID)
		if img, ok := ui.getDemoImage(filename); ok && img != nil {
			op, cached := ui.imageOps[v.ID]
			if !cached {
				op = paint.NewImageOp(img)
				ui.imageOps[v.ID] = op
			}
			imgOp = op
			hasImg = true
		}
	} else if ui.thumbs != nil {
		if img, ok := ui.thumbs.Get(v.ID); ok && img != nil {
			op, cached := ui.imageOps[v.ID]
			if !cached {
				op = paint.NewImageOp(img)
				ui.imageOps[v.ID] = op
			}
			imgOp = op
			hasImg = true
		}
	} else {
		delete(ui.imageOps, v.ID)
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

			// Thin red progress line at thumbnail bottom while preview plays
			if isHovered && ui.hoverVideoID == v.ID && len(ui.hoverFrames) > 0 {
				progH := gtx.Dp(3)
				progW := (ui.hoverFrameIdx + 1) * w / len(ui.hoverFrames)
				if progW > w {
					progW = w
				}
				progRect := clip.Rect(image.Rect(0, h-progH, progW, h))
				paint.FillShape(gtx.Ops, ui.colors.Accent, progRect.Op())
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
									lbl := ui.label(unit.Sp(12), durStr)
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

	// Request thumbnails for visible rows plus 1 row margin, clamped to half budget.
	first := ui.searchList.Position.First
	count := ui.searchList.Position.Count
	if count == 0 {
		count = 6
	}
	start := first - 1
	if start < 0 {
		start = 0
	}
	end := first + count + 1
	if end > len(videos) {
		end = len(videos)
	}
	if end-start > thumbs.MaxThumbnailsInHalfBudget {
		end = start + thumbs.MaxThumbnailsInHalfBudget
		if end > len(videos) {
			end = len(videos)
		}
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
									title := ui.label(unit.Sp(18), v.Title)
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
									meta := ui.label(unit.Sp(12), metaText)
									meta.Color = ui.colors.Text2
									return meta.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
								// Channel row: 24 dp avatar circle + channel name 12 sp text2
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return ui.layoutAvatar(gtx, v.ChannelID, v.Channel, 24)
										}),
										layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := ui.label(unit.Sp(12), v.Channel)
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

	// Signed in, the account Subs feed shows as card rows above the manage rows.
	var videos []yt.Video
	if ui.account != nil && ui.account.SignedIn() {
		videos = ui.state.FilteredVideos()
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

	numCardRows := (len(videos) + columns - 1) / columns

	// Thumbnail prefetching for visible card rows plus 1 row margin.
	firstRow := ui.subPageList.Position.First
	countRows := ui.subPageList.Position.Count
	if countRows == 0 {
		countRows = 4
	}
	startRow := firstRow - 1
	if startRow < 0 {
		startRow = 0
	}
	endRow := firstRow + countRows + 1
	if endRow > numCardRows {
		endRow = numCardRows
	}
	startIdx := startRow * columns
	endIdx := endRow * columns
	if endIdx > len(videos) {
		endIdx = len(videos)
	}
	if endIdx-startIdx > thumbs.MaxThumbnailsInHalfBudget {
		endIdx = startIdx + thumbs.MaxThumbnailsInHalfBudget
		if endIdx > len(videos) {
			endIdx = len(videos)
		}
	}
	for i := startIdx; i < endIdx; i++ {
		ui.requestThumbnail(videos[i].ID)
	}

	cardWidthPx := (availW - (columns-1)*gapX) / columns
	thumbHeightPx := cardWidthPx * 9 / 16

	manageStart := numCardRows
	totalItems := manageStart + 4
	if len(subs) > 0 {
		totalItems += len(subs)
	} else {
		totalItems += 1
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutSubsChannelsRow(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.subPageList.Layout(gtx, totalItems, func(gtx layout.Context, idx int) layout.Dimensions {
				if idx < manageStart {
					return layout.Inset{
						Left:   unit.Dp(16),
						Right:  unit.Dp(16),
						Top:    unit.Dp(8),
						Bottom: unit.Dp(36),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							ui.layoutGridCards(gtx, idx, columns, cardWidthPx, thumbHeightPx, videos)...,
						)
					})
				}

				offset := idx - manageStart
				switch offset {
				case 0:
					return layout.Inset{
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
						Top:    unit.Dp(24),
						Bottom: unit.Dp(16),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(18), "Manage subscriptions")
						lbl.Font.Weight = font.Bold
						lbl.Color = ui.colors.Text
						return lbl.Layout(gtx)
					})
				case 1:
					return layout.Inset{
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
						Bottom: unit.Dp(12),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
											ed := ui.editor(&ui.addChannelEditor, "Add channel: URL, @handle or UC id")
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
								btn := ui.button(&ui.addChannelBtn, "Subscribe")
								btn.Background = ui.colors.Action
								btn.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
								btn.CornerRadius = unit.Dp(18)
								btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
								return btn.Layout(gtx)
							}),
						)
					})
				case 2:
					return layout.Inset{
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
						Bottom: unit.Dp(24),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
											ed := ui.editor(&ui.csvPathEditor, "Path to subscriptions.csv…")
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
								btn := ui.button(&ui.importCSVBtn, "Import")
								btn.Background = ui.colors.Surface
								btn.Color = ui.colors.Text
								btn.CornerRadius = unit.Dp(8)
								btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
								return btn.Layout(gtx)
							}),
						)
					})
				case 3:
					return layout.Inset{
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
						Bottom: unit.Dp(12),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(16), fmt.Sprintf("Subscribed channels (%d)", len(subs)))
						lbl.Font.Weight = font.Bold
						lbl.Color = ui.colors.Text
						return lbl.Layout(gtx)
					})
				default:
					if len(subs) == 0 {
						return layout.Inset{
							Left:   unit.Dp(24),
							Right:  unit.Dp(24),
							Bottom: unit.Dp(16),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(14), "No channels subscribed yet.")
							lbl.Color = ui.colors.Text2
							return lbl.Layout(gtx)
						})
					}
					subIdx := offset - 4
					sub := subs[subIdx]
					click := &ui.unsubscribeClicks[subIdx]
					if click.Clicked(gtx) {
						ui.removeSubscription(sub.ChannelID)
					}

					return layout.Inset{
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
						Bottom: unit.Dp(8),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return ui.layoutAvatar(gtx, sub.ChannelID, sub.Title, 36)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								title := sub.Title
								if title == "" {
									title = sub.ChannelID
								}
								lbl := ui.label(unit.Sp(15), title)
								lbl.Font.Weight = font.Medium
								lbl.Color = ui.colors.Text
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
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
												lbl := ui.label(unit.Sp(13), "Subscribed")
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
				}
			})
		}),
	)
}

func (ui *UI) saveSettings() {
	if ui.store != nil {
		_ = ui.store.SaveSettings(ui.settings)
	}
}

func (ui *UI) layoutSettingChip(gtx layout.Context, click *widget.Clickable, title string, isSelected bool) layout.Dimensions {
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
					lbl := ui.label(unit.Sp(13), title)
					lbl.Color = chipFg
					if isSelected {
						lbl.Font.Weight = font.Medium
					}
					return lbl.Layout(gtx)
				})
			},
		)
	})
}

func formatGPUDecodes(caps embed.Caps, loaded bool, err error) string {
	if !loaded {
		return "GPU decodes: querying…"
	}
	if err != nil {
		return fmt.Sprintf("GPU decodes: unavailable (%v)", err)
	}

	var parts []string
	if caps.H264 {
		parts = append(parts, "H.264")
	}
	if caps.HEVC {
		parts = append(parts, "HEVC")
	}
	if caps.VP9 {
		parts = append(parts, "VP9")
	}
	if caps.AV1 {
		parts = append(parts, "AV1")
	}

	if len(parts) == 0 {
		if caps.Driver != "" {
			return fmt.Sprintf("GPU decodes: none (%s)", caps.Driver)
		}
		return "GPU decodes: none"
	}

	codecs := strings.Join(parts, ", ")
	if caps.Driver != "" {
		return fmt.Sprintf("GPU decodes: %s (%s)", codecs, caps.Driver)
	}
	return fmt.Sprintf("GPU decodes: %s", codecs)
}

func (ui *UI) layoutSuggestionsDropdown(gtx layout.Context) layout.Dimensions {
	if !ui.suggestVisible || len(ui.suggestItems) == 0 {
		return layout.Dimensions{}
	}

	n := len(ui.suggestItems)
	if n > 10 {
		n = 10
	}

	winW := gtx.Constraints.Max.X
	pillW := gtx.Dp(640)
	if pillW > winW-gtx.Dp(32) {
		pillW = winW - gtx.Dp(32)
	}
	left := (winW - pillW) / 2
	top := gtx.Dp(52)

	rowH := gtx.Dp(40)
	query := strings.TrimSpace(ui.searchEditor.Text())
	qLower := strings.ToLower(query)
	hasAllYT := query != ""
	totalH := rowH*n + gtx.Dp(16)
	if hasAllYT {
		totalH += rowH + gtx.Dp(1)
	}

	dropRect := image.Rect(left, top, left+pillW, top+totalH)
	bgRRect := clip.UniformRRect(dropRect, gtx.Dp(12))
	paint.FillShape(gtx.Ops, ui.colors.Surface, bgRRect.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{
		Path:  bgRRect.Path(gtx.Ops),
		Width: float32(gtx.Dp(1)),
	}.Op())

	for i := 0; i < n; i++ {
		item := ui.suggestItems[i]
		if ui.suggestClicks[i].Clicked(gtx) {
			ui.suggestVisible = false
			ui.suggestSelected = -1
			ui.searchEditor.SetText(item)
			ui.search(item)
			return layout.Dimensions{}
		}

		rowTop := top + gtx.Dp(8) + i*rowH
		rowRect := image.Rect(left+gtx.Dp(4), rowTop, left+pillW-gtx.Dp(4), rowTop+rowH)
		rowRRect := clip.UniformRRect(rowRect, gtx.Dp(8))

		isSel := ui.suggestSelected == i
		isHov := ui.suggestClicks[i].Hovered()
		if isSel || isHov {
			paint.FillShape(gtx.Ops, ui.colors.SurfaceHover, rowRRect.Op(gtx.Ops))
		}
		if isHov {
			pointer.CursorPointer.Add(gtx.Ops)
		}

		cArea := clip.Rect(rowRect).Push(gtx.Ops)
		ui.suggestClicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: rowRect.Size()}
		})
		cArea.Pop()

		iLower := strings.ToLower(item)
		var prefixStr, restStr string
		if strings.HasPrefix(iLower, qLower) && len(qLower) > 0 {
			prefixStr = item[:len(qLower)]
			restStr = item[len(qLower):]
		} else {
			restStr = item
		}

		rowArea := clip.Rect(rowRect).Push(gtx.Ops)
		offset := op.Offset(rowRect.Min).Push(gtx.Ops)
		rowGtx := gtx
		rowGtx.Constraints = layout.Exact(rowRect.Size())
		layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(rowGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Dp(18)
					drawMagnifier(gtx, image.Rect(0, 0, sz, sz), ui.colors.Text2)
					return layout.Dimensions{Size: image.Pt(sz, sz)}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if prefixStr == "" {
						lbl := ui.label(unit.Sp(14), restStr)
						lbl.Color = ui.colors.Text
						lbl.Font.Weight = font.Medium
						return lbl.Layout(gtx)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(14), prefixStr)
							lbl.Color = ui.colors.Text2
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(14), restStr)
							lbl.Color = ui.colors.Text
							lbl.Font.Weight = font.Medium
							return lbl.Layout(gtx)
						}),
					)
				}),
			)
		})
		offset.Pop()
		rowArea.Pop()
	}

	if hasAllYT {
		divY := top + gtx.Dp(8) + n*rowH
		divRect := image.Rect(left+gtx.Dp(8), divY, left+pillW-gtx.Dp(8), divY+gtx.Dp(1))
		paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(divRect).Op())

		allYTRowTop := divY + gtx.Dp(1)
		allYTRowRect := image.Rect(left+gtx.Dp(4), allYTRowTop, left+pillW-gtx.Dp(4), allYTRowTop+rowH)
		allYTRowRRect := clip.UniformRRect(allYTRowRect, gtx.Dp(8))

		if ui.allYouTubeClick.Clicked(gtx) {
			ui.suggestVisible = false
			ui.suggestSelected = -1
			if curr := ui.spaces.Active(); curr != nil {
				curr.Query = query
			}
			ui.switchSpace(SpaceWatch)
			ui.searchEditor.SetText(query)
			ui.lastSearchText = query
			ui.search(query)
			return layout.Dimensions{}
		}

		if ui.allYouTubeClick.Hovered() {
			paint.FillShape(gtx.Ops, ui.colors.SurfaceHover, allYTRowRRect.Op(gtx.Ops))
			pointer.CursorPointer.Add(gtx.Ops)
		}

		cArea := clip.Rect(allYTRowRect).Push(gtx.Ops)
		ui.allYouTubeClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: allYTRowRect.Size()}
		})
		cArea.Pop()

		offset := op.Offset(allYTRowRect.Min).Push(gtx.Ops)
		rowGtx := gtx
		rowGtx.Constraints = layout.Exact(allYTRowRect.Size())
		layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(rowGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					badgeW := gtx.Dp(78)
					badgeH := gtx.Dp(22)
					r := clip.UniformRRect(image.Rect(0, 0, badgeW, badgeH), gtx.Dp(4))
					paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
					badgeGtx := gtx
					badgeGtx.Constraints = layout.Exact(image.Pt(badgeW, badgeH))
					return layout.Center.Layout(badgeGtx, func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(10), "All YouTube")
						lbl.Color = ui.colors.Text2
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(13), fmt.Sprintf("Search all of YouTube for '%s'", query))
					lbl.Color = ui.colors.Text
					return lbl.Layout(gtx)
				}),
			)
		})
		offset.Pop()
	}

	return layout.Dimensions{}
}

func (ui *UI) layoutPlaylistView(gtx layout.Context) layout.Dimensions {
	if ui.playlistBackBtn.Clicked(gtx) {
		ui.switchTab(TabAccount)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left:   unit.Dp(24),
				Right:  unit.Dp(24),
				Top:    unit.Dp(16),
				Bottom: unit.Dp(16),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ui.playlistBackBtn.Hovered() {
							pointer.CursorPointer.Add(gtx.Ops)
						}
						btn := ui.button(&ui.playlistBackBtn, "← Back")
						btn.Background = ui.colors.Surface
						btn.Color = ui.colors.Text
						btn.CornerRadius = unit.Dp(16)
						btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(18), ui.state.CurrentPlaylist.Title)
								lbl.Font.Weight = font.Bold
								lbl.Color = ui.colors.Text
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								info := ""
								if ui.state.CurrentPlaylist.Channel != "" {
									info += ui.state.CurrentPlaylist.Channel
								}
								if ui.state.CurrentPlaylist.VideoCount > 0 {
									if info != "" {
										info += " • "
									}
									info += fmt.Sprintf("%d videos", ui.state.CurrentPlaylist.VideoCount)
								}
								if info == "" {
									return layout.Dimensions{}
								}
								lbl := ui.label(unit.Sp(13), info)
								lbl.Color = ui.colors.Text2
								return lbl.Layout(gtx)
							}),
						)
					}),
				)
			})
		}),
		layout.Flexed(1, ui.layoutCardGrid),
	)
}

func (ui *UI) layoutPlaylistsGrid(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(20), "Saved Playlists")
			lbl.Font.Weight = font.Bold
			lbl.Color = ui.colors.Text
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(ui.state.SavedPlaylists) == 0 {
				msg := "No saved playlists"
				if ui.playlistsLoading {
					msg = "Loading playlists…"
				} else if ui.account == nil || !ui.account.SignedIn() {
					msg = "Sign in to see saved playlists"
				}
				lbl := ui.label(unit.Sp(14), msg)
				lbl.Color = ui.colors.Text2
				return lbl.Layout(gtx)
			}

			availW := gtx.Constraints.Max.X
			cardW := gtx.Dp(220)
			gapX := gtx.Dp(16)
			cols := (availW + gapX) / (cardW + gapX)
			if cols < 1 {
				cols = 1
			}

			for len(ui.playlistClicks) < len(ui.state.SavedPlaylists) {
				ui.playlistClicks = append(ui.playlistClicks, widget.Clickable{})
			}

			numPlaylists := len(ui.state.SavedPlaylists)
			numRows := (numPlaylists + cols - 1) / cols

			var rowChildren []layout.FlexChild
			for r := 0; r < numRows; r++ {
				rowIdx := r
				rowChildren = append(rowChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						ui.layoutPlaylistRow(gtx, rowIdx, cols, cardW, gapX)...,
					)
				}))
				if r < numRows-1 {
					rowChildren = append(rowChildren, layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout))
				}
			}

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)
		}),
	)
}

func (ui *UI) layoutPlaylistRow(gtx layout.Context, rowIdx, cols, cardW, gapX int) []layout.FlexChild {
	var children []layout.FlexChild
	start := rowIdx * cols
	end := start + cols
	if end > len(ui.state.SavedPlaylists) {
		end = len(ui.state.SavedPlaylists)
	}

	for i := start; i < end; i++ {
		idx := i
		pl := ui.state.SavedPlaylists[idx]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.playlistClicks[idx].Clicked(gtx) {
				ui.openPlaylist(pl)
			}
			return ui.playlistClicks[idx].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if ui.playlistClicks[idx].Hovered() {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				gtx.Constraints.Min.X = cardW
				gtx.Constraints.Max.X = cardW
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						h := cardW * 9 / 16
						boxSize := image.Pt(cardW, h)
						gtx.Constraints.Min = boxSize
						gtx.Constraints.Max = boxSize

						vidID := extractThumbnailVideoID(pl.ThumbnailURL)
						var imgOp paint.ImageOp
						hasImg := false
						if vidID != "" {
							if img, ok := ui.thumbs.Get(vidID); ok && img != nil {
								op, cached := ui.imageOps[vidID]
								if !cached {
									op = paint.NewImageOp(img)
									ui.imageOps[vidID] = op
								}
								imgOp = op
								hasImg = true
							} else {
								ui.requestThumbnail(vidID)
							}
						}

						rrect := clip.UniformRRect(image.Rectangle{Max: boxSize}, gtx.Dp(8))
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
								if pl.VideoCount > 0 {
									return layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{
											Right:  unit.Dp(6),
											Bottom: unit.Dp(6),
										}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											countStr := fmt.Sprintf("%d videos", pl.VideoCount)
											return layout.Background{}.Layout(gtx,
												func(gtx layout.Context) layout.Dimensions {
													bg := color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xd0}
													br := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(4))
													paint.FillShape(gtx.Ops, bg, br.Op(gtx.Ops))
													return layout.Dimensions{Size: gtx.Constraints.Min}
												},
												func(gtx layout.Context) layout.Dimensions {
													return layout.Inset{
														Top:    unit.Dp(2),
														Bottom: unit.Dp(2),
														Left:   unit.Dp(6),
														Right:  unit.Dp(6),
													}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														lbl := ui.label(unit.Sp(11), countStr)
														lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
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
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(14), pl.Title)
						lbl.Font.Weight = font.Medium
						lbl.Color = ui.colors.Text
						lbl.MaxLines = 2
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if pl.Channel == "" {
							return layout.Dimensions{}
						}
						lbl := ui.label(unit.Sp(12), pl.Channel)
						lbl.Color = ui.colors.Text2
						return lbl.Layout(gtx)
					}),
				)
			})
		}))

		if i < end-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout))
		}
	}
	return children
}

func (ui *UI) layoutAccountView(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := ui.accountBrowserEditor.Update(gtx)
		if !ok {
			break
		}
		spec := strings.TrimSpace(ui.accountBrowserEditor.Text())
		if spec != "" && spec != ui.settings.BrowserSpec {
			ui.settings.BrowserSpec = spec
			if ui.account != nil {
				ui.account.SetBrowser(spec)
			}
			ui.saveSettings()
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			if ui.account != nil && !ui.account.SignedIn() && !ui.accountSigningIn {
				ui.signInAccount()
			}
		}
	}

	if ui.accountSignInBtn.Clicked(gtx) {
		ui.signInAccount()
	}
	if ui.accountSignOutBtn.Clicked(gtx) {
		ui.signOutAccount()
	}
	if ui.addAccountBtn.Clicked(gtx) {
		ui.showAddAccount = !ui.showAddAccount
		if ui.showAddAccount && len(ui.discoveredProfiles) == 0 {
			go func() {
				profs := yt.DiscoverBrowserProfiles()
				ui.sendAction(func() {
					ui.discoveredProfiles = profs
				})
				ui.invalidate()
			}()
		}
	}
	if ui.cancelAddAccountBtn.Clicked(gtx) {
		ui.showAddAccount = false
		ui.signInGoogleStatus = ""
	}

	if ui.signInGoogleBtn.Clicked(gtx) {
		ui.signInGoogleStatus = "Finish signing in in the browser window, then close it"
		go func() {
			ctx := context.Background()
			accID := fmt.Sprintf("acc_%d", time.Now().Unix())
			bSpec, uDir, err := ui.googleSignInLauncher.Launch(ctx, ui.store.Dir(), accID)
			if err != nil {
				ui.sendAction(func() {
					ui.signInGoogleStatus = "Error: " + yt.SanitizeErrorOutput(err.Error())
				})
				ui.invalidate()
				return
			}

			ui.sendAction(func() {
				ui.signInGoogleStatus = "Extracting cookies…"
			})
			ui.invalidate()

			cookieFile := filepath.Join(ui.store.Dir(), "accounts", accID, "cookies.txt")
			cacheDir := filepath.Join(platform.CacheDir(), "accounts", accID)
			newAcc := yt.NewAccount()
			newAcc.CookieFile = cookieFile
			newAcc.CacheDir = cacheDir
			newAcc.SetBrowser(bSpec)

			signErr := newAcc.SignIn(ctx)
			ui.sendAction(func() {
				if signErr != nil {
					ui.signInGoogleStatus = "Error extracting cookies: " + yt.SanitizeErrorOutput(signErr.Error())
					return
				}
				accConfig := store.AccountConfig{
					ID:          accID,
					Label:       "Google Account",
					BrowserSpec: bSpec,
					UserDataDir: uDir,
					CookieFile:  cookieFile,
					CacheDir:    cacheDir,
				}
				_ = ui.store.AddAccount(accConfig)
				ui.switchAccount(accConfig)
				ui.signInGoogleStatus = ""
				ui.showAddAccount = false
			})
			ui.invalidate()
		}()
	}

	for len(ui.addProfileClicks) < len(ui.discoveredProfiles) {
		ui.addProfileClicks = append(ui.addProfileClicks, widget.Clickable{})
	}
	for i, prof := range ui.discoveredProfiles {
		if ui.addProfileClicks[i].Clicked(gtx) {
			accID := fmt.Sprintf("acc_%d", time.Now().Unix())
			lbl := prof.Name
			if prof.UserName != "" {
				lbl += " (" + prof.UserName + ")"
			}
			cookieFile := filepath.Join(ui.store.Dir(), "accounts", accID, "cookies.txt")
			cacheDir := filepath.Join(platform.CacheDir(), "accounts", accID)
			newAcc := yt.NewAccount()
			newAcc.CookieFile = cookieFile
			newAcc.CacheDir = cacheDir
			newAcc.SetBrowser(prof.BrowserSpec)

			go func(id, label, spec, cFile, cdDir string) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				err := newAcc.SignIn(ctx)
				ui.sendAction(func() {
					if err == nil {
						accConfig := store.AccountConfig{
							ID:          id,
							Label:       label,
							BrowserSpec: spec,
							CookieFile:  cFile,
							CacheDir:    cdDir,
						}
						_ = ui.store.AddAccount(accConfig)
						ui.switchAccount(accConfig)
						ui.showAddAccount = false
					} else {
						ui.accountError = yt.SanitizeErrorOutput(err.Error())
					}
				})
				ui.invalidate()
			}(accID, lbl, prof.BrowserSpec, cookieFile, cacheDir)
		}
	}

	for _, acc := range ui.settings.Accounts {
		sw, so, rm := ui.getAccountClicks(acc.ID)
		if sw.Clicked(gtx) {
			ui.switchAccount(acc)
		}
		if so.Clicked(gtx) {
			if acc.ID == ui.settings.ActiveAccount {
				ui.signOutAccount()
			} else {
				_ = os.Remove(acc.CookieFile)
				_ = os.Remove(acc.CookieFile + ".play")
				ui.reloadAllFeeds()
			}
		}
		if rm.Clicked(gtx) {
			ui.removeAccount(acc.ID)
		}
	}

	return layout.Inset{
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
		Top:    unit.Dp(24),
		Bottom: unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return ui.accountList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(24), "You")
					lbl.Font.Weight = font.Bold
					lbl.Color = ui.colors.Text
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

				// Quick navigation row to Subscriptions and Library
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.accountNavSubsClick.Clicked(gtx) {
						ui.openUtility(TabSubscriptions)
					}
					if ui.accountNavLibClick.Clicked(gtx) {
						ui.openUtility(TabWatchLater)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.accountNavSubsClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								if ui.accountNavSubsClick.Hovered() {
									pointer.CursorPointer.Add(gtx.Ops)
								}
								return layout.Background{}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
										paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
										return layout.Dimensions{Size: gtx.Constraints.Min}
									},
									func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											lbl := ui.label(unit.Sp(13), "Subscriptions")
											lbl.Color = ui.colors.Text
											lbl.Font.Weight = font.Medium
											return lbl.Layout(gtx)
										})
									},
								)
							})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.accountNavLibClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								if ui.accountNavLibClick.Hovered() {
									pointer.CursorPointer.Add(gtx.Ops)
								}
								return layout.Background{}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
										paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
										return layout.Dimensions{Size: gtx.Constraints.Min}
									},
									func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											lbl := ui.label(unit.Sp(13), "Your Library")
											lbl.Color = ui.colors.Text
											lbl.Font.Weight = font.Medium
											return lbl.Layout(gtx)
										})
									},
								)
							})
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(20), "Accounts")
							lbl.Font.Weight = font.Bold
							lbl.Color = ui.colors.Text
							return lbl.Layout(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btnText := "+ Add account"
							if ui.showAddAccount {
								btnText = "Close Add Account"
							}
							if ui.addAccountBtn.Hovered() {
								pointer.CursorPointer.Add(gtx.Ops)
							}
							btn := ui.button(&ui.addAccountBtn, btnText)
							btn.Background = ui.colors.Surface
							btn.Color = ui.colors.Text
							btn.CornerRadius = unit.Dp(16)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
							return btn.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !ui.showAddAccount {
						return layout.Dimensions{}
					}
					return ui.layoutAddAccountPanel(gtx)
				}),

				layout.Rigid(ui.layoutAccountsList),
				layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout),

				layout.Rigid(ui.layoutPlaylistsGrid),
				layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout),

				layout.Rigid(ui.layoutSettingsSection),
			)
		})
	})
}

func (ui *UI) layoutAccountsList(gtx layout.Context) layout.Dimensions {
	accounts := ui.settings.Accounts
	if len(accounts) == 0 {
		return layout.Dimensions{}
	}

	var children []layout.FlexChild
	for _, a := range accounts {
		acc := a
		isActive := acc.ID == ui.settings.ActiveAccount
		sw, so, rm := ui.getAccountClicks(acc.ID)

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					rrect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
					bg := ui.colors.Surface
					if isActive {
						bg = ui.colors.SurfaceHover
					}
					paint.FillShape(gtx.Ops, bg, rrect.Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Top:    unit.Dp(12),
						Bottom: unit.Dp(12),
						Left:   unit.Dp(16),
						Right:  unit.Dp(16),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								sz := gtx.Dp(36)
								return layout.Background{}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										r := clip.UniformRRect(image.Rect(0, 0, sz, sz), gtx.Dp(18))
										col := ui.colors.SurfaceHover
										if isActive {
											col = ui.colors.Accent
										}
										paint.FillShape(gtx.Ops, col, r.Op(gtx.Ops))
										return layout.Dimensions{Size: image.Pt(sz, sz)}
									},
									func(gtx layout.Context) layout.Dimensions {
										drawPersonIcon(gtx, image.Rect(0, 0, sz, sz), ui.colors.Text)
										return layout.Dimensions{Size: image.Pt(sz, sz)}
									},
								)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := ui.label(unit.Sp(15), acc.Label)
										lbl.Font.Weight = font.Medium
										lbl.Color = ui.colors.Text
										return lbl.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										subText := acc.ID
										if isActive {
											if ui.account != nil && ui.account.SignedIn() {
												subText = "Active • Signed in"
											} else {
												subText = "Active • Signed out"
											}
										}
										lbl := ui.label(unit.Sp(12), subText)
										lbl.Color = ui.colors.Text2
										return lbl.Layout(gtx)
									}),
								)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: gtx.Constraints.Min}
							}),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if isActive {
											return layout.Background{}.Layout(gtx,
												func(gtx layout.Context) layout.Dimensions {
													r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
													paint.FillShape(gtx.Ops, color.NRGBA{R: 0x1f, G: 0x6e, B: 0x2e, A: 0xff}, r.Op(gtx.Ops))
													return layout.Dimensions{Size: gtx.Constraints.Min}
												},
												func(gtx layout.Context) layout.Dimensions {
													return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														lbl := ui.label(unit.Sp(12), "Active")
														lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
														return lbl.Layout(gtx)
													})
												},
											)
										}
										if sw.Hovered() {
											pointer.CursorPointer.Add(gtx.Ops)
										}
										btn := ui.button(sw, "Switch")
										btn.Background = ui.colors.SurfaceHover
										btn.Color = ui.colors.Text
										btn.CornerRadius = unit.Dp(14)
										btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}
										return btn.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),

									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if so.Hovered() {
											pointer.CursorPointer.Add(gtx.Ops)
										}
										btn := ui.button(so, "Sign out")
										btn.Background = ui.colors.SurfaceHover
										btn.Color = ui.colors.Text2
										btn.CornerRadius = unit.Dp(14)
										btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}
										return btn.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),

									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if rm.Hovered() {
											pointer.CursorPointer.Add(gtx.Ops)
										}
										btn := ui.button(rm, "Remove")
										btn.Background = ui.colors.SurfaceHover
										btn.Color = ui.colors.Action
										btn.CornerRadius = unit.Dp(14)
										btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}
										return btn.Layout(gtx)
									}),
								)
							}),
						)
					})
				},
			)
		}))
		children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *UI) layoutAddAccountPanel(gtx layout.Context) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			rrect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(10))
			paint.FillShape(gtx.Ops, ui.colors.Surface, rrect.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: rrect.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(16),
				Bottom: unit.Dp(16),
				Left:   unit.Dp(16),
				Right:  unit.Dp(16),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(16), "1. Use a browser profile")
						lbl.Font.Weight = font.Medium
						lbl.Color = ui.colors.Text
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(ui.discoveredProfiles) == 0 {
							lbl := ui.label(unit.Sp(13), "No browser profiles found (Chrome, Chromium, Brave, Edge, Firefox).")
							lbl.Color = ui.colors.Text2
							return lbl.Layout(gtx)
						}
						var profChildren []layout.FlexChild
						for i, p := range ui.discoveredProfiles {
							idx := i
							prof := p
							profChildren = append(profChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								title := fmt.Sprintf("[%s] %s", strings.Title(prof.Browser), prof.Name)
								if prof.UserName != "" {
									title += fmt.Sprintf(" (%s)", prof.UserName)
								}
								if ui.addProfileClicks[idx].Hovered() {
									pointer.CursorPointer.Add(gtx.Ops)
								}
								btn := ui.button(&ui.addProfileClicks[idx], title)
								btn.Background = ui.colors.SurfaceHover
								btn.Color = ui.colors.Text
								btn.CornerRadius = unit.Dp(14)
								btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
								return btn.Layout(gtx)
							}))
							profChildren = append(profChildren, layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout))
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, profChildren...)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(16), "2. Sign in with Google")
						lbl.Font.Weight = font.Medium
						lbl.Color = ui.colors.Text
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						desc := ui.label(unit.Sp(13), "Opens Chrome or Chromium in an isolated profile to sign in. When finished, close the browser.")
						desc.Color = ui.colors.Text2
						return desc.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if ui.signInGoogleBtn.Hovered() {
									pointer.CursorPointer.Add(gtx.Ops)
								}
								btn := ui.button(&ui.signInGoogleBtn, "Sign in with Google")
								btn.Background = ui.colors.Action
								btn.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
								btn.CornerRadius = unit.Dp(16)
								btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
								return btn.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if ui.signInGoogleStatus == "" {
									return layout.Dimensions{}
								}
								lbl := ui.label(unit.Sp(13), ui.signInGoogleStatus)
								lbl.Color = ui.colors.Text2
								if strings.HasPrefix(ui.signInGoogleStatus, "Error") {
									lbl.Color = ui.colors.Accent
								}
								return lbl.Layout(gtx)
							}),
						)
					}),
				)
			})
		},
	)
}

func (ui *UI) layoutSettingsSection(gtx layout.Context) layout.Dimensions {
	if ui.codecAutoChip.Clicked(gtx) {
		if ui.settings.CodecPolicy != string(player.CodecPolicyAuto) {
			ui.settings.CodecPolicy = string(player.CodecPolicyAuto)
			ui.saveSettings()
		}
	}
	if ui.codecOldGPUChip.Clicked(gtx) {
		if ui.settings.CodecPolicy != string(player.CodecPolicyOldGPU) {
			ui.settings.CodecPolicy = string(player.CodecPolicyOldGPU)
			ui.saveSettings()
		}
	}
	if ui.codecAnyChip.Clicked(gtx) {
		if ui.settings.CodecPolicy != string(player.CodecPolicyAny) {
			ui.settings.CodecPolicy = string(player.CodecPolicyAny)
			ui.saveSettings()
		}
	}

	if ui.playerInplaceChip.Clicked(gtx) {
		if ui.settings.Player != store.PlayerInplace {
			ui.settings.Player = store.PlayerInplace
			if !ui.cliExternalPlayer {
				ui.externalPlayer = false
			}
			ui.saveSettings()
		}
	}
	if ui.playerExternalChip.Clicked(gtx) {
		if ui.settings.Player != store.PlayerExternal {
			ui.settings.Player = store.PlayerExternal
			ui.externalPlayer = true
			ui.saveSettings()
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(20), "Settings")
			lbl.Font.Weight = font.Bold
			lbl.Color = ui.colors.Text
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(14), "Codec")
			lbl.Font.Weight = font.Medium
			lbl.Color = ui.colors.Text
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutSettingChip(gtx, &ui.codecAutoChip, "Auto", ui.settings.CodecPolicy == string(player.CodecPolicyAuto))
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutSettingChip(gtx, &ui.codecOldGPUChip, "Old GPU (H.264)", ui.settings.CodecPolicy == string(player.CodecPolicyOldGPU))
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutSettingChip(gtx, &ui.codecAnyChip, "Any", ui.settings.CodecPolicy == string(player.CodecPolicyAny))
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gpuText := formatGPUDecodes(ui.cachedCaps, ui.capsLoaded, ui.capsErr)
			lbl := ui.label(unit.Sp(13), gpuText)
			lbl.Color = ui.colors.Text2
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(14), "Player")
			lbl.Font.Weight = font.Medium
			lbl.Color = ui.colors.Text
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutSettingChip(gtx, &ui.playerInplaceChip, "In window", ui.settings.Player == store.PlayerInplace)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutSettingChip(gtx, &ui.playerExternalChip, "External mpv", ui.settings.Player == store.PlayerExternal)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(14), "Login browser")
			lbl.Font.Weight = font.Medium
			lbl.Color = ui.colors.Text
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
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
						ed := ui.editor(&ui.accountBrowserEditor, yt.DefaultBrowserSpec())
						ed.Color = ui.colors.Text
						ed.HintColor = ui.colors.Text2
						ed.TextSize = unit.Sp(13)
						return ed.Layout(gtx)
					})
				},
			)
		}),
	)
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
		lbl := ui.label(unit.Sp(14), msg)
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
				lbl := ui.label(unit.Sp(12), ui.state.Status)
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
	ui.saveCurrentPosition()
	ui.dropThumbnails()
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

	if ui.demo {
		return
	}

	_ = ui.store.AddHistory(v)
	ui.state.Status = ""

	if !ui.view.Valid() && ui.embedPlayer == nil {
		ui.state.CurrentTab = ui.previousTab
		ui.playVideoExternal(v, false)
		ui.state.Status = "Embedded player unavailable on this display; playing with external mpv"
		return
	}

	if ui.embedPlayer == nil {
		p, err := ui.createEmbedPlayer()
		if err != nil {
			ui.state.CurrentTab = ui.previousTab
			ui.playVideoExternal(v, false)
			ui.state.Status = fmt.Sprintf("Embedded player unavailable (%v); playing with external mpv", err)
			return
		}
		ui.embedPlayer = p
	}

	cookiePath := ""
	if ui.account != nil && ui.account.SignedIn() {
		cookiePath, _ = ui.account.PlaybackCookieFile()
	}

	scale := ui.currentScale
	if scale <= 0 {
		scale = 2
	}

	codecPol := player.CodecPolicy(ui.settings.CodecPolicy)
	formatStr := player.FormatFor(ui.power, player.ModeVideo, codecPol, ui.cachedCaps)
	hwdec := player.HWDec(codecPol, ui.cachedCaps)
	err := ui.embedPlayer.Play(embed.PlayOptions{
		URL:         v.WatchURL(),
		Title:       v.Title,
		YTDLFormat:  formatStr,
		CookiesFile: cookiePath,
		HWDec:       hwdec,
		WidthPx:     1280,
		HeightPx:    720,
		Scale:       scale,
		Debug:       ui.debug,
	})

	if err != nil {
		ui.state.Status = "Playback error: " + err.Error()
		return
	}

	ui.watchPaused = false
	ui.watchCurrentTime = 0
	ui.watchDuration = float64(v.Duration)
	ui.watchLastMouseMove = time.Now()
	ui.hwdecLogged = false
	ui.lastTimePoll = time.Now()
	ui.playerOwnerSpace = SpaceWatch
	ui.playerOwnerID = v.ID
	ui.applyResumePosition(v)
	ui.invalidate()
}

func (ui *UI) closeWatchPage() {
	ui.saveCurrentPosition()
	if ui.embedPlayer != nil {
		ui.embedPlayer.Stop()
	}
	ui.playerOwnerID = ""
	if ui.watchFullscreen {
		ui.setFullscreen(false)
	}
	ui.state.CurrentTab = ui.previousTab
	if ui.state.CurrentTab == TabWatch {
		ui.state.CurrentTab = TabHome
	}
	ui.upNextVideos = nil
	ui.watchVideo = yt.Video{}
	ui.dropThumbnails()
	runtime.GC()
	if ui.debug {
		fmt.Fprintf(os.Stderr, "watch page closed\n")
	}
	ui.invalidate()
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

	if ui.spaces != nil {
		switch ui.spaces.ActiveSpace {
		case SpaceShorts:
			ui.spaces.State(SpaceShorts).ResumeItem = &v
			ui.spaces.State(SpaceShorts).Shorts = []yt.Video{v}
			ui.playVideo(v, false)
			return
		case SpacePodcasts:
			ui.spaces.State(SpacePodcasts).ResumeItem = &v
			ui.spaces.State(SpacePodcasts).Episodes = []yt.Video{v}
			ui.playVideo(v, false)
			return
		case SpaceMusic:
			ui.spaces.State(SpaceMusic).ResumeItem = &v
			ui.spaces.State(SpaceMusic).Tracks = []yt.Video{v}
			ui.playVideo(v, false)
			return
		}
	}

	ui.openWatchPage(v)
}

func (ui *UI) maybeTriggerDebugPlayFirst() {
	if !ui.debugPlayFirst || ui.debugPlayTriggered || !ui.firstFramePresented {
		return
	}
	if ui.spaces == nil {
		return
	}
	switch ui.spaces.ActiveSpace {
	case SpaceMusic:
		state := ui.spaces.State(SpaceMusic)
		if len(state.Tracks) > 0 {
			ui.debugPlayTriggered = true
			ui.debugPlayFirst = false
			if ui.debug {
				fmt.Fprintln(os.Stderr, "debug-selected-first=music")
			}
			item := state.Tracks[0]
			tracks := append([]yt.Video(nil), state.Tracks...)
			ui.sendAction(func() { ui.selectMusicTrack(item, tracks) })
			ui.invalidate()
		}
	case SpacePodcasts:
		state := ui.spaces.State(SpacePodcasts)
		if len(state.Episodes) > 0 {
			ui.debugPlayTriggered = true
			ui.debugPlayFirst = false
			if ui.debug {
				fmt.Fprintln(os.Stderr, "debug-selected-first=podcasts")
			}
			item := state.Episodes[0]
			episodes := append([]yt.Video(nil), state.Episodes...)
			ui.sendAction(func() { ui.selectPodcastEpisode(item, episodes) })
			ui.invalidate()
		}
	case SpaceShorts:
		state := ui.spaces.State(SpaceShorts)
		if len(state.Shorts) > 0 {
			ui.debugPlayTriggered = true
			ui.debugPlayFirst = false
			if ui.debug {
				fmt.Fprintln(os.Stderr, "debug-selected-first=shorts")
			}
			item := state.Shorts[0]
			shorts := append([]yt.Video(nil), state.Shorts...)
			ui.sendAction(func() { ui.selectShort(item, shorts) })
			ui.invalidate()
		}
	case SpaceWatch:
		state := ui.spaces.State(SpaceWatch)
		if len(state.Videos) > 0 {
			ui.debugPlayTriggered = true
			ui.debugPlayFirst = false
			item := state.Videos[0]
			state.ResumeItem = &item
			ui.playVideo(item, false)
		}
	}
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

		ui.registerVideoBox(image.Rect(0, 0, winW, videoH))

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
	videoX := ui.spaceOrigin.X + gtx.Dp(16)
	videoY := ui.spaceOrigin.Y + gtx.Dp(16)
	if !isWide {
		videoY -= ui.watchList.Position.Offset
	}

	videoRect := image.Rect(videoX, videoY, videoX+videoW, videoY+videoH)
	if !isWide && (ui.watchList.Position.First > 0 || ui.watchList.Position.Offset > 0 || videoY+videoH <= ui.spaceOrigin.Y) {
		videoRect = image.Rectangle{}
	}
	ui.registerVideoBox(videoRect)

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

	if ui.debugVideoMarker && !ui.activeVideoBox.Empty() {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 0x00, G: 0xff, B: 0x01, A: 0xff}, clip.Rect(image.Rect(0, 0, w, h)).Op())
		return layout.Dimensions{Size: box}
	}

	if ui.demo {
		paint.FillShape(gtx.Ops, ui.colors.Surface, clip.Rect(image.Rect(0, 0, w, h)).Op())
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return ui.layoutPillBadge(gtx, "Playback disabled in demo", false)
		})
	}

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
					ui.togglePlaybackPause()
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
		paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(image.Rect(0, progY, playedW, progY+progLineH)).Op())
	}
	handleR := gtx.Dp(5)
	handleRect := image.Rect(playedW-handleR, progY+progLineH/2-handleR, playedW+handleR, progY+progLineH/2+handleR)
	paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Ellipse(handleRect).Op(gtx.Ops))

	if ui.watchPlayPauseClick.Clicked(gtx) {
		if ui.embedPlayer != nil {
			ui.togglePlaybackPause()
		}
	}
	if ui.watchMuteClick.Clicked(gtx) {
		if ui.embedPlayer != nil {
			ui.togglePlayerMute()
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
						timeStr := formatPlaybackTime(ui.watchCurrentTime, ui.watchDuration)
						lbl := ui.label(unit.Sp(13), timeStr)
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
			lbl := ui.label(unit.Sp(20), v.Title)
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
					return ui.layoutAvatar(gtx, v.ChannelID, v.Channel, 36)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(16), v.Channel)
					lbl.Font.Weight = font.Medium
					lbl.Color = ui.colors.Text
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					isSub := ui.isSubscribed(v.ChannelID)
					label := "Subscribe"
					btnBg := ui.colors.Action
					btnFg := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
					if isSub {
						label = "Subscribed"
						btnBg = ui.colors.Surface
						btnFg = ui.colors.Text
					}
					btn := ui.button(&ui.watchSubscribeClick, label)
					btn.Background = btnBg
					btn.Color = btnFg
					btn.CornerRadius = unit.Dp(18)
					btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(16), Right: unit.Dp(16)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := ui.button(&ui.watchLaterClick, "Watch later")
					btn.Background = ui.colors.Surface
					btn.Color = ui.colors.Text
					btn.CornerRadius = unit.Dp(18)
					btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := ui.button(&ui.watchExternalMPVClick, "mpv")
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
								lbl := ui.label(unit.Sp(14), metaText)
								lbl.Font.Weight = font.Medium
								lbl.Color = ui.colors.Text
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if v.Description == "" {
									return layout.Dimensions{}
								}
								return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									desc := ui.label(unit.Sp(13), v.Description)
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

	// Request thumbnails for visible rows plus 1 row margin, clamped to half budget.
	first := ui.upNextList.Position.First
	count := ui.upNextList.Position.Count
	if count == 0 {
		count = 6
	}
	start := first - 1
	if start < 0 {
		start = 0
	}
	end := first + count + 1
	if end > len(videos) {
		end = len(videos)
	}
	if end-start > thumbs.MaxThumbnailsInHalfBudget {
		end = start + thumbs.MaxThumbnailsInHalfBudget
		if end > len(videos) {
			end = len(videos)
		}
	}
	for i := start; i < end; i++ {
		ui.requestThumbnail(videos[i].ID)
	}

	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(16), "Up next")
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
										lbl := ui.label(unit.Sp(14), v.Title)
										lbl.Font.Weight = font.Medium
										lbl.Color = ui.colors.Text
										lbl.MaxLines = 2
										lbl.Truncator = "…"
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := ui.label(unit.Sp(12), v.Channel)
										lbl.Color = ui.colors.Text2
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										metaText := FormatMetaLine(v.ViewCount, v.UploadDate, v.PublishedAt, time.Now())
										lbl := ui.label(unit.Sp(12), metaText)
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

// layoutSubsChannelsRow renders the horizontal row of account channel avatars at the top of Subscriptions.
func (ui *UI) layoutSubsChannelsRow(gtx layout.Context) layout.Dimensions {
	type channelItem struct {
		id        string
		title     string
		avatarURL string
	}
	var items []channelItem

	if ui.account != nil && ui.account.SignedIn() && len(ui.state.AccountChannels) > 0 {
		for _, ch := range ui.state.AccountChannels {
			items = append(items, channelItem{id: ch.ID, title: ch.Title, avatarURL: ch.AvatarURL})
		}
	} else {
		for _, sub := range ui.state.Subscriptions {
			title := sub.Title
			if title == "" {
				title = sub.ChannelID
			}
			items = append(items, channelItem{id: sub.ChannelID, title: title, avatarURL: ""})
		}
	}

	if len(items) == 0 {
		return layout.Dimensions{}
	}

	for len(ui.subsChannelClicks) < len(items) {
		ui.subsChannelClicks = append(ui.subsChannelClicks, widget.Clickable{})
	}

	ui.subsChannelsList.Axis = layout.Horizontal

	return layout.Inset{
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
		Top:    unit.Dp(12),
		Bottom: unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := ui.label(unit.Sp(16), "Channels")
				lbl.Font.Weight = font.Bold
				lbl.Color = ui.colors.Text
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, lbl.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.subsChannelsList.Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
					item := items[i]
					click := &ui.subsChannelClicks[i]
					if click.Clicked(gtx) {
						ui.selectChannel(item.id, item.title)
					}
					if click.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}

					itemWidth := gtx.Dp(76)
					gtx.Constraints.Min.X = itemWidth
					gtx.Constraints.Max.X = itemWidth

					return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
								// 64 dp avatar circle
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if item.avatarURL != "" && ui.thumbs != nil {
										if img, ok := ui.thumbs.GetAvatar(item.avatarURL); ok && img != nil {
											sizePx := gtx.Dp(64)
											box := image.Pt(sizePx, sizePx)
											gtx.Constraints.Min = box
											gtx.Constraints.Max = box
											circle := clip.Ellipse(image.Rect(0, 0, sizePx, sizePx))
											cl := circle.Push(gtx.Ops)
											imgOp := paint.NewImageOp(img)
											sz := imgOp.Size()
											scaleX := float32(sizePx) / float32(sz.X)
											scaleY := float32(sizePx) / float32(sz.Y)
											aff := f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scaleX, scaleY))
											affOp := op.Affine(aff).Push(gtx.Ops)
											imgOp.Add(gtx.Ops)
											paint.PaintOp{}.Add(gtx.Ops)
											affOp.Pop()
											cl.Pop()
											return layout.Dimensions{Size: box}
										}
										ui.thumbs.RequestAvatar(item.avatarURL)
									}
									return ui.layoutAvatar(gtx, item.id, item.title, 64)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
								// Channel name: 12 sp, 1 line, ellipsis
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := ui.label(unit.Sp(12), item.title)
									lbl.Color = ui.colors.Text
									lbl.MaxLines = 1
									lbl.Truncator = "…"
									lbl.Alignment = text.Middle
									return lbl.Layout(gtx)
								}),
							)
						})
					})
				})
			}),
		)
	})
}

// selectChannel filters Home feed to the given channel or loads its RSS feed.
func (ui *UI) selectChannel(channelID, channelTitle string) {
	ui.state.ActiveChannelFilter = channelID
	ui.state.ActiveChannelTitle = channelTitle
	ui.state.SelectedIndex = 0
	if ui.state.CurrentTab != TabHome {
		ui.switchTab(TabHome)
		ui.state.ActiveChannelFilter = channelID
		ui.state.ActiveChannelTitle = channelTitle
	}

	request := ui.beginSpaceRequest(SpaceWatch, loadContent)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		videos, err := ui.ytClient.Feed(ctx, channelID)
		if err != nil {
			return
		}
		ui.sendAction(func() {
			if ui.requestCurrent(request) {
				ui.setWatchVideos(videos)
			}
		})
		ui.invalidate()
	}()
}

// refreshAccountChannels fetches account subscribed channels once per Subs page open.
func (ui *UI) refreshAccountChannels() {
	if ui.account == nil || !ui.account.SignedIn() || ui.accountChannelsFetching {
		return
	}
	ui.accountChannelsFetching = true
	request := ui.beginSpaceRequest(ui.spaces.ActiveSpace, loadChannels)
	source := ui.account
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		channels, err := source.Channels(ctx)
		ui.sendAction(func() {
			if !ui.requestCurrent(request) {
				return
			}
			ui.accountChannelsFetching = false
			if err == nil && len(channels) > 0 {
				ui.state.AccountChannels = channels
			}
		})
		ui.invalidate()
	}()
}

func (ui *UI) startHover(videoID string, cardIdx int) {
	if ui.hoverVideoID == videoID {
		return
	}
	ui.clearHover()
	ui.hoverVideoID = videoID
	ui.hoverCardIdx = cardIdx
	ui.hoverStartTime = time.Now()
	ui.hoverFetchStarted = false
}

func (ui *UI) clearHover() {
	if ui.hoverCancel != nil {
		ui.hoverCancel()
		ui.hoverCancel = nil
	}
	ui.hoverVideoID = ""
	ui.hoverCardIdx = -1
	ui.hoverStartTime = time.Time{}
	ui.hoverFetchStarted = false
	ui.hoverFrames = nil
	ui.hoverOps = nil
	ui.hoverFrameIdx = 0
	ui.hoverLastFrameTime = time.Time{}
}

func (ui *UI) loadStoryboardPreview(ctx context.Context, videoID string) {
	if ui.account == nil {
		return
	}
	sb, err := ui.account.Storyboard(ctx, videoID)
	if err != nil || sb == nil || len(sb.Sheets) == 0 {
		return
	}
	if ctx.Err() != nil {
		return
	}
	frames, err := ui.fetchStoryboardFrames(ctx, sb)
	if err != nil || len(frames) == 0 {
		return
	}
	if ctx.Err() != nil {
		return
	}
	ui.sendAction(func() {
		if ui.hoverVideoID == videoID {
			ui.hoverFrames = frames
			ui.hoverOps = make([]paint.ImageOp, len(frames))
			for i, f := range frames {
				ui.hoverOps[i] = paint.NewImageOp(f)
			}
			ui.hoverFrameIdx = 0
			ui.hoverLastFrameTime = time.Now()
		}
	})
	ui.invalidate()
}

func (ui *UI) fetchStoryboardFrames(ctx context.Context, sb *yt.Storyboard) ([]image.Image, error) {
	if sb == nil || len(sb.Sheets) == 0 {
		return nil, nil
	}
	sheetsToFetch := sb.Sheets
	if len(sheetsToFetch) > 6 {
		urls := make([]string, 6)
		for i := 0; i < 6; i++ {
			idx := i * (len(sb.Sheets) - 1) / 5
			urls[i] = sb.Sheets[idx]
		}
		sheetsToFetch = urls
	}

	var allFrames []image.Image
	client := &http.Client{Timeout: 10 * time.Second}

	for _, sheetURL := range sheetsToFetch {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sheetURL, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		sheetData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		sheetImg, _, err := image.Decode(bytes.NewReader(sheetData))
		if err != nil {
			continue
		}

		rows := sb.Rows
		cols := sb.Columns
		if rows <= 0 {
			rows = 3
		}
		if cols <= 0 {
			cols = 3
		}
		frameW := sb.Width
		frameH := sb.Height
		if frameW <= 0 {
			frameW = sheetImg.Bounds().Dx() / cols
		}
		if frameH <= 0 {
			frameH = sheetImg.Bounds().Dy() / rows
		}
		if frameW <= 0 || frameH <= 0 {
			continue
		}

		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				minX := sheetImg.Bounds().Min.X + c*frameW
				minY := sheetImg.Bounds().Min.Y + r*frameH
				frameRect := image.Rect(minX, minY, minX+frameW, minY+frameH)

				dst := image.NewRGBA(image.Rect(0, 0, frameW, frameH))
				draw.Draw(dst, dst.Bounds(), sheetImg, frameRect.Min, draw.Src)
				allFrames = append(allFrames, dst)
			}
		}
	}
	return allFrames, nil
}

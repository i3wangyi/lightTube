package ui

import (
	"fmt"
	"image"
	"image/color"

	"lighttube/internal/store"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// layoutRail renders the left navigation rail:
// Width: 216 dp on desktop (>= 1050 dp window width), collapses to 72 dp icon guide below 1050 dp.
// Top: small "LT" red mark + "LightTube".
// "Your spaces": Watch, Shorts, Podcasts, Music (48 dp rows; active = raised bg and accent icon).
// Divider.
// Utilities: Library (Watch later, History, saved playlists), Subscriptions (manage), You (accounts, settings).
func (ui *UI) layoutRail(gtx layout.Context) layout.Dimensions {
	winW := gtx.Constraints.Max.X
	collapsed := winW < gtx.Dp(1050)
	railW := gtx.Dp(216)
	if collapsed {
		railW = gtx.Dp(72)
	}

	gtx.Constraints.Min.X = railW
	gtx.Constraints.Max.X = railW

	spacesList := []struct {
		id    SpaceID
		label string
		icon  func(layout.Context, image.Rectangle, color.NRGBA)
	}{
		{SpaceWatch, "Watch", drawWatchIcon},
		{SpaceShorts, "Shorts", drawShortsIcon},
		{SpacePodcasts, "Podcasts", drawPodcastsIcon},
		{SpaceMusic, "Music", drawMusicIcon},
	}

	return layout.Inset{
		Top:    unit.Dp(16),
		Bottom: unit.Dp(16),
		Left:   unit.Dp(collapsedPad(collapsed)),
		Right:  unit.Dp(collapsedPad(collapsed)),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild

		// 1. Brand: "LT" mark + LightTube
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.logoClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if ui.logoClick.Hovered() {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				if collapsed {
					// Centered 31x31 brand mark
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return ui.drawBrandMark(gtx)
					})
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.drawBrandMark(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(20), "lightTube")
						lbl.Color = ui.colors.Text
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					}),
				)
			})
		}))

		if !collapsed {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(16), Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(10), "Independent client")
					lbl.Color = ui.colors.Text2
					return lbl.Layout(gtx)
				})
			}))
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(11), "Your spaces")
					lbl.Color = ui.colors.Text2
					lbl.Font.Weight = font.Medium
					return lbl.Layout(gtx)
				})
			}))
		} else {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout))
		}

		// 2. Spaces navigation (Watch, Shorts, Podcasts, Music)
		for i, sp := range spacesList {
			idx := i
			spaceItem := sp
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				isActive := !ui.showingUtility && ui.spaces.ActiveSpace == spaceItem.id
				click := &ui.spaceNavClicks[idx]
				if click.Clicked(gtx) {
					ui.switchSpace(spaceItem.id)
				}
				return ui.layoutRailRow(gtx, click, spaceItem.label, spaceItem.icon, isActive, collapsed)
			}))
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
		}

		// 3. Divider
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(1)
				w := gtx.Constraints.Max.X
				rect := image.Rect(0, 0, w, h)
				paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(rect).Op())
				return layout.Dimensions{Size: image.Pt(w, h)}
			})
		}))

		// 4. Utilities: Library, Subscriptions, You
		utilities := []struct {
			tab   Tab
			label string
			click *widget.Clickable
			icon  func(layout.Context, image.Rectangle, color.NRGBA)
		}{
			{TabWatchLater, "Your library", &ui.railLibraryClick, drawLibraryIcon},
			{TabSubscriptions, "Subscriptions", &ui.railSubsClick, drawStackedRectsIcon},
			{TabAccount, "You", &ui.railYouClick, drawPersonIcon},
		}

		for _, u := range utilities {
			util := u
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				isActive := false
				if ui.showingUtility {
					if util.tab == TabWatchLater && (ui.utilityTab == TabWatchLater || ui.utilityTab == TabHistory) {
						isActive = true
					} else if util.tab == ui.utilityTab {
						isActive = true
					}
				}
				if util.click.Clicked(gtx) {
					ui.openUtility(util.tab)
				}
				return ui.layoutRailRow(gtx, util.click, util.label, util.icon, isActive, collapsed)
			}))
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
		}

		// Push footer to bottom
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}))

		// 5. Bottom note (uncollapsed only)
		if !collapsed {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(10), "Independent client\nNot affiliated with YouTube.")
					lbl.Color = ui.colors.Text2
					return lbl.Layout(gtx)
				})
			}))
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func collapsedPad(collapsed bool) int {
	if collapsed {
		return 6
	}
	return 12
}

func (ui *UI) drawBrandMark(gtx layout.Context) layout.Dimensions {
	sz := gtx.Dp(31)
	gtx.Constraints = layout.Exact(image.Pt(sz, sz))
	r := clip.UniformRRect(image.Rect(0, 0, sz, sz), gtx.Dp(9))
	paint.FillShape(gtx.Ops, ui.colors.Accent, r.Op(gtx.Ops))
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := ui.label(unit.Sp(13), "LT")
		lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		lbl.Font.Weight = font.Bold
		return lbl.Layout(gtx)
	})
	return layout.Dimensions{Size: image.Pt(sz, sz)}
}

func (ui *UI) layoutRailRow(gtx layout.Context, click *widget.Clickable, label string, drawIcon func(layout.Context, image.Rectangle, color.NRGBA), isActive, collapsed bool) layout.Dimensions {
	h := gtx.Dp(48)
	w := gtx.Constraints.Max.X
	rect := image.Rect(0, 0, w, h)
	rrect := clip.UniformRRect(rect, gtx.Dp(10))

	if click.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}

	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if isActive {
			paint.FillShape(gtx.Ops, ui.colors.Raised, rrect.Op(gtx.Ops))
		} else if click.Hovered() {
			paint.FillShape(gtx.Ops, ui.colors.Hover, rrect.Op(gtx.Ops))
		}

		iconColor := ui.colors.Text2
		if isActive {
			iconColor = ui.colors.Accent
		}

		if collapsed {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				iconBox := image.Rect(0, 0, gtx.Dp(22), gtx.Dp(22))
				drawIcon(gtx, iconBox, iconColor)
				return layout.Dimensions{Size: iconBox.Size()}
			})
		}

		return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					iconBox := image.Rect(0, 0, gtx.Dp(22), gtx.Dp(22))
					drawIcon(gtx, iconBox, iconColor)
					return layout.Dimensions{Size: iconBox.Size()}
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(14), label)
					lbl.Color = ui.colors.Text
					if isActive {
						lbl.Font.Weight = font.Medium
					}
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

// SearchScopeLabel returns the scope label shown in the top bar.
func SearchScopeLabel(space SpaceID) string {
	switch space {
	case SpaceWatch:
		return "Watch space"
	case SpaceShorts:
		return "Shorts space"
	case SpacePodcasts:
		return "Podcasts space"
	case SpaceMusic:
		return "Music space"
	default:
		return "Watch space"
	}
}

// SearchPlaceholder returns the placeholder text for the search input.
func SearchPlaceholder(space SpaceID) string {
	switch space {
	case SpaceWatch:
		return "Search Watch"
	case SpaceShorts:
		return "Search Shorts"
	case SpacePodcasts:
		return "Search Podcasts"
	case SpaceMusic:
		return "Search Music"
	default:
		return "Search Watch"
	}
}

// switchSpace switches to the requested space, pauses playback, preserves/restores state,
// and never starts playback.
func (ui *UI) switchSpace(space SpaceID) {
	ui.pausePlayback()

	if ui.spaces == nil {
		return
	}

	// 1. Save state of current active space
	curr := ui.spaces.Active()
	if curr != nil {
		curr.Query = ui.searchEditor.Text()
	}

	// 2. Switch space
	ui.spaces.ActiveSpace = space
	ui.showingUtility = false
	ui.state.CurrentTab = TabHome

	// 3. Restore state of target space
	target := ui.spaces.Active()
	if target != nil {
		ui.searchEditor.SetText(target.Query)
		ui.lastSearchText = target.Query
		ui.state.Status = target.Status
		needsLoad := target.ContentGeneration != ui.currentGen() || !spaceHasContent(target)
		if needsLoad {
			switch space {
			case SpaceWatch:
				ui.loadWatchSpaceContent(target.Chip)
			case SpaceShorts:
				ui.loadShortsSpaceContent(target.Chip)
			case SpacePodcasts:
				ui.loadPodcastsSpaceContent(target.Chip)
			case SpaceMusic:
				ui.loadMusicSpaceContent(target.Chip)
			}
		} else if space == SpaceWatch {
			ui.state.SetVideos(target.Videos)
		}
	}

	ui.invalidate()
}

func (ui *UI) clearAccountSpaceState(stop, save bool) {
	ui.bumpGen()
	if save {
		ui.saveCurrentPosition()
	}
	if stop && ui.embedPlayer != nil {
		ui.embedPlayer.Stop()
	}
	if stop && ui.player != nil {
		_ = ui.player.Stop()
	}
	ui.playerOwnerID = ""
	ui.playerMuteKnown = false
	ui.pendingResumeID = ""
	ui.pendingResumeSeconds = 0
	ui.watchPaused = true
	ui.spaces.ResetContent()
	ui.shortsLoading, ui.podcastsLoading, ui.musicLoading = false, false, false
	ui.playlistsLoading, ui.playlistsLoaded, ui.accountChannelsFetching = false, false, false
	ui.refreshMu.Lock()
	clear(ui.refreshingFeeds)
	ui.refreshMu.Unlock()
	ui.searchEditor.SetText("")
	ui.state.SetVideos(nil)
}

// pausePlayback pauses video playback without launching or playing anything.
func (ui *UI) pausePlayback() {
	ui.saveCurrentPosition()
	if ui.embedPlayer != nil {
		ui.embedPlayer.SetPause(true)
	}
	ui.watchPaused = true
}

func (ui *UI) togglePlaybackPause() {
	if ui.embedPlayer == nil {
		return
	}
	if !ui.watchPaused {
		ui.saveCurrentPosition()
	}
	ui.embedPlayer.TogglePause()
	ui.watchPaused = !ui.watchPaused
}

func spaceHasContent(state *SpaceState) bool {
	if state == nil {
		return false
	}
	return state.ContentLoaded
}

func (ui *UI) ownsPlayer(space SpaceID, mediaID string) bool {
	return ui.embedPlayer != nil && ui.playerOwnerSpace == space && mediaID != "" && ui.playerOwnerID == mediaID
}

func (ui *UI) spaceItemPosition(space SpaceID, itemID string) float64 {
	if ui.ownsPlayer(space, itemID) {
		return ui.watchCurrentTime
	}
	if pos, ok := ui.getVideoPosition(itemID); ok {
		return pos.Seconds
	}
	return 0
}

func (ui *UI) saveCurrentPosition() {
	if ui.embedPlayer == nil || ui.playerOwnerID == "" || ui.store == nil {
		return
	}
	st := ui.embedPlayer.Status()
	seconds, duration := st.TimePos, st.Duration
	if seconds <= 0 {
		seconds = ui.watchCurrentTime
	}
	if ui.pendingResumeID == ui.playerOwnerID && ui.pendingResumeSeconds > seconds {
		seconds = ui.pendingResumeSeconds
	}
	if duration <= 0 {
		duration = ui.watchDuration
	}
	if duration > 0 {
		kind := "video"
		switch ui.playerOwnerSpace {
		case SpacePodcasts:
			kind = "episode"
		case SpaceMusic:
			kind = "track"
		case SpaceShorts:
			kind = "short"
		}
		_ = ui.store.SaveMediaPosition(ui.currentAccountID(), ui.playerOwnerID, store.Position{
			Seconds: seconds, Duration: duration, Space: ui.playerOwnerSpace.String(), MediaKind: kind,
			Title: ui.watchVideo.Title, Show: ui.watchVideo.Channel,
		})
	}
}

func (ui *UI) currentGen() int64 {
	ui.generationMu.Lock()
	defer ui.generationMu.Unlock()
	return ui.generation
}

func (ui *UI) bumpGen() int64 {
	ui.generationMu.Lock()
	defer ui.generationMu.Unlock()
	ui.generation++
	return ui.generation
}

func (ui *UI) currentAccountID() string {
	return ui.settings.ActiveAccount
}

func (ui *UI) collectionAccountID() string {
	if ui.settings.ActiveAccount == "" || ui.settings.ActiveAccount == "default" {
		return ""
	}
	return ui.settings.ActiveAccount
}

// openUtility switches to a utility view (Library, Subscriptions, You).
func (ui *UI) openUtility(t Tab) {
	ui.pausePlayback()
	ui.showingUtility = true
	ui.utilityTab = t
	ui.switchTab(t)
	ui.invalidate()
}

// layoutPerSpaceHeading renders the per-space header:
// Title, subtitle, status badge (e.g. "Autoplay off"), and "Ambient on/off" badge-button.
func (ui *UI) layoutPerSpaceHeading(gtx layout.Context) layout.Dimensions {
	cfg := GetSpaceConfig(ui.spaces.ActiveSpace)
	if ui.spaces.ActiveSpace == SpaceShorts {
		cfg.Badge, _ = ui.shortsSoundLabels()
	}

	if ui.ambientToggleClick.Clicked(gtx) {
		newVal := !ui.settings.AmbientEnabled()
		_ = ui.store.SetAmbient(newVal)
		ui.settings = ui.store.Settings()
		ui.invalidate()
	}

	ambientText := "Ambient off"
	if ui.settings.AmbientEnabled() {
		ambientText = "Ambient on"
	}

	winW := gtx.Constraints.Max.X
	isNarrow := winW < gtx.Dp(600)

	padX := unit.Dp(24)
	if isNarrow {
		padX = unit.Dp(12)
	}

	return layout.Inset{
		Top:    unit.Dp(14),
		Bottom: unit.Dp(14),
		Left:   padX,
		Right:  padX,
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		titleBlock := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					titleSp := unit.Sp(26)
					if isNarrow {
						titleSp = unit.Sp(22)
					}
					lbl := ui.label(titleSp, cfg.Title)
					lbl.Color = ui.colors.Text
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(12), cfg.Subtitle)
					lbl.Color = ui.colors.Text2
					return lbl.Layout(gtx)
				}),
			)
		}

		badgesBlock := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				// Status badge (e.g. "Autoplay off")
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutPillBadge(gtx, cfg.Badge, false)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				// Ambient toggle badge-button
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.ambientToggleClick.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					return ui.ambientToggleClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return ui.layoutPillBadge(gtx, ambientText, true)
					})
				}),
			)
		}

		if isNarrow {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(titleBlock),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(badgesBlock),
			)
		}

		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(titleBlock),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Rigid(badgesBlock),
		)
	})
}

func (ui *UI) layoutPillBadge(gtx layout.Context, text string, isButton bool) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			minDim := gtx.Constraints.Min.X
			if gtx.Constraints.Min.Y < minDim {
				minDim = gtx.Constraints.Min.Y
			}
			rad := gtx.Dp(14)
			if rad > minDim/2 {
				rad = minDim / 2
			}
			if rad < 0 {
				rad = 0
			}
			r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, rad)
			paint.FillShape(gtx.Ops, ui.colors.Bg, r.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(6),
				Bottom: unit.Dp(6),
				Left:   unit.Dp(12),
				Right:  unit.Dp(12),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := ui.label(unit.Sp(11), text)
				if isButton {
					lbl.Color = ui.colors.Text
				} else {
					lbl.Color = ui.colors.Text2
				}
				lbl.Font.Weight = font.Medium
				return lbl.Layout(gtx)
			})
		},
	)
}

// layoutChips renders the space-specific filter chips in a horizontal scrolling list.
func (ui *UI) layoutChips(gtx layout.Context) layout.Dimensions {
	cfg := GetSpaceConfig(ui.spaces.ActiveSpace)
	activeChip := ui.spaces.Active().Chip

	padX := unit.Dp(24)
	if gtx.Constraints.Max.X < gtx.Dp(600) {
		padX = unit.Dp(12)
	}

	return layout.Inset{
		Bottom: unit.Dp(14),
		Left:   padX,
		Right:  padX,
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return ui.chipsList.Layout(gtx, len(cfg.Chips), func(gtx layout.Context, idx int) layout.Dimensions {
			chipName := cfg.Chips[idx]
			isSelected := activeChip == idx

			click := &ui.spaces.ChipClicks[idx]
			if click.Clicked(gtx) {
				ui.spaces.Active().Chip = idx
				switch ui.spaces.ActiveSpace {
				case SpaceWatch:
					ui.loadWatchSpaceContent(idx)
				case SpaceShorts:
					ui.loadShortsSpaceContent(idx)
				case SpacePodcasts:
					ui.loadPodcastsSpaceContent(idx)
				case SpaceMusic:
					ui.loadMusicSpaceContent(idx)
				}
				ui.invalidate()
			}
			if click.Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}

			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							bgCol := ui.colors.Raised
							if isSelected {
								bgCol = ui.colors.Text // Active: text bg (#f1f1f1)
							} else if click.Hovered() {
								bgCol = ui.colors.Hover
							}
							r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
							paint.FillShape(gtx.Ops, bgCol, r.Op(gtx.Ops))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{
								Top:    unit.Dp(8),
								Bottom: unit.Dp(8),
								Left:   unit.Dp(14),
								Right:  unit.Dp(14),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(12), chipName)
								if isSelected {
									lbl.Color = ui.colors.Bg // Active: bg text (#0f0f0f)
									lbl.Font.Weight = font.Bold
								} else {
									lbl.Color = ui.colors.Text
									lbl.Font.Weight = font.Medium
								}
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							})
						},
					)
				})
			})
		})
	})
}

// layoutBottomBar renders the bottom spaces navigation bar for mobile viewports (< 600 dp).
func (ui *UI) layoutBottomBar(gtx layout.Context) layout.Dimensions {
	spacesList := []struct {
		id    SpaceID
		label string
		icon  func(layout.Context, image.Rectangle, color.NRGBA)
	}{
		{SpaceWatch, "Watch", drawWatchIcon},
		{SpaceShorts, "Shorts", drawShortsIcon},
		{SpacePodcasts, "Podcasts", drawPodcastsIcon},
		{SpaceMusic, "Music", drawMusicIcon},
	}

	h := gtx.Dp(56)
	w := gtx.Constraints.Max.X

	paint.FillShape(gtx.Ops, ui.colors.Bg, clip.Rect(image.Rect(0, 0, w, h)).Op())
	paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, 0, w, gtx.Dp(1))).Op())

	var children []layout.FlexChild
	for i, sp := range spacesList {
		idx := i
		spaceItem := sp
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			isActive := !ui.showingUtility && ui.spaces.ActiveSpace == spaceItem.id
			click := &ui.spaceNavClicks[idx]
			if click.Clicked(gtx) {
				ui.switchSpace(spaceItem.id)
			}
			if click.Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				iconCol := ui.colors.Text2
				textCol := ui.colors.Text2
				if isActive {
					iconCol = ui.colors.Accent
					textCol = ui.colors.Text
				}
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							iconBox := image.Rect(0, 0, gtx.Dp(20), gtx.Dp(20))
							pillW := gtx.Dp(48)
							pillH := gtx.Dp(26)
							pillGtx := gtx
							pillGtx.Constraints = layout.Exact(image.Pt(pillW, pillH))

							if isActive {
								pillR := clip.UniformRRect(image.Rect(0, 0, pillW, pillH), gtx.Dp(13))
								return layout.Background{}.Layout(pillGtx,
									func(gtx layout.Context) layout.Dimensions {
										paint.FillShape(gtx.Ops, ui.colors.Raised, pillR.Op(gtx.Ops))
										return layout.Dimensions{Size: image.Pt(pillW, pillH)}
									},
									func(gtx layout.Context) layout.Dimensions {
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											spaceItem.icon(gtx, iconBox, iconCol)
											return layout.Dimensions{Size: iconBox.Size()}
										})
									},
								)
							}

							return layout.Center.Layout(pillGtx, func(gtx layout.Context) layout.Dimensions {
								spaceItem.icon(gtx, iconBox, iconCol)
								return layout.Dimensions{Size: iconBox.Size()}
							})
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(10), spaceItem.label)
							lbl.Color = textCol
							if isActive {
								lbl.Font.Weight = font.Bold
							}
							return lbl.Layout(gtx)
						}),
					)
				})
			})
		}))
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// layoutPlaceholderScreen renders placeholder screens for Shorts, Podcasts, and Music.
// Says they arrive in the next step (no fake data).
func (ui *UI) layoutPlaceholderScreen(gtx layout.Context, spaceName string) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(24),
		Bottom: unit.Dp(24),
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
				paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
				paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top:    unit.Dp(48),
					Bottom: unit.Dp(48),
					Left:   unit.Dp(36),
					Right:  unit.Dp(36),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(22), fmt.Sprintf("%s arrives in the next step.", spaceName))
							lbl.Color = ui.colors.Text
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(13), "This space will feature native content and its own ordered queue without fake data.")
							lbl.Color = ui.colors.Text2
							return lbl.Layout(gtx)
						}),
					)
				})
			},
		)
	})
}

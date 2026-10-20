package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"lighttube/internal/yt"
)

// layoutPodcastsSpace renders the Podcasts space with a 4-column square show grid,
// ordered episode queue rows with mini-progress bars, and its dedicated show queue.
func (ui *UI) layoutPodcastsSpace(gtx layout.Context) layout.Dimensions {
	state := ui.spaces.State(SpacePodcasts)
	chip := state.Chip

	// Trigger live load if needed
	if !ui.demo && !state.ContentLoaded && !ui.podcastsLoading {
		ui.loadPodcastsSpaceContent(chip)
	}

	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	padX := unit.Dp(24)
	if winW < gtx.Dp(600) {
		padX = unit.Dp(12)
	}
	padXPx := gtx.Dp(padX)
	dockPadBottom := gtx.Dp(12)
	dockW := winW - padXPx*2

	vidW := gtx.Dp(240)
	if dockW-gtx.Dp(32) < vidW {
		vidW = dockW - gtx.Dp(32)
	}
	vidH := vidW * 9 / 16

	// A dock click can load a different owner; compute visibility afterward.
	if ui.spaces.PodcastsDockPlay.Clicked(gtx) {
		ui.playPodcastsDock()
	}

	isPlaying := state.ResumeItem != nil && ui.ownsPlayer(SpacePodcasts, state.ResumeItem.ID)
	dockGtx := gtx
	dockGtx.Constraints.Min.X = dockW
	dockGtx.Constraints.Max.X = dockW
	dockGtx.Constraints.Min.Y = 0
	dockRecord := op.Record(gtx.Ops)
	dockDims := ui.layoutPodcastsDockSection(dockGtx, vidW, vidH, isPlaying)
	dockCall := dockRecord.Stop()
	dockRelY := winH - dockDims.Size.Y - dockPadBottom
	if isPlaying {
		videoOrigin := ui.spaceOrigin.Add(image.Pt(padXPx+gtx.Dp(16), dockRelY+gtx.Dp(12)))
		ui.registerVideoBox(image.Rectangle{Min: videoOrigin, Max: videoOrigin.Add(image.Pt(vidW, vidH))})
	} else {
		ui.registerVideoBox(image.Rectangle{})
	}

	scrollListH := dockRelY - gtx.Dp(12)
	if scrollListH < 0 {
		scrollListH = 0
	}

	return layout.Stack{}.Layout(gtx,
		// 1. Scrollable show grid & episodes list
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtxList := gtx
			gtxList.Constraints = layout.Exact(image.Pt(winW, scrollListH))
			return layout.Inset{
				Left:  padX,
				Right: padX,
			}.Layout(gtxList, func(gtx layout.Context) layout.Dimensions {
				return state.ScrollList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// 1. Shows square grid section
						layout.Rigid(ui.layoutPodcastsShowsSection),
						layout.Rigid(layout.Spacer{Height: unit.Dp(28)}.Layout),
						// 2. Episodes list section
						layout.Rigid(ui.layoutPodcastsEpisodesSection),
					)
				})
			})
		}),
		// 2. Compact player dock pinned at bottom of viewport outside scroll list
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			off := op.Offset(image.Pt(padXPx, dockRelY)).Push(gtx.Ops)
			dockCall.Add(gtx.Ops)
			off.Pop()
			return layout.Dimensions{Size: image.Pt(dockW, dockDims.Size.Y)}
		}),
	)
}

// layoutPodcastsShowsSection renders the 4-column (2-column on narrow) square shows grid.
func (ui *UI) layoutPodcastsShowsSection(gtx layout.Context) layout.Dimensions {
	state := ui.spaces.State(SpacePodcasts)
	chip := state.Chip

	coverHeading := "Shows you follow"
	if chip == 1 {
		coverHeading = "Shows with new episodes"
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Section header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutSectionHeader(gtx, coverHeading, "Follow a show. Finish an episode.")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		// Grid of shows
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.demo && ui.demoData != nil {
				return ui.layoutDemoShowsGrid(gtx, ui.demoData.Shows)
			}
			if len(state.Shows) > 0 {
				return ui.layoutLiveShowsGrid(gtx, state.Shows)
			}
			if ui.podcastsLoading {
				return ui.layoutEmptyMessage(gtx, "Loading shows from YouTube…")
			}
			if state.PodcastsError != "" {
				return ui.layoutEmptyMessage(gtx, state.PodcastsError)
			}
			return ui.layoutEmptyMessage(gtx, "No podcast shows found. Follow shows on YouTube to see them here.")
		}),
		layout.Rigid(ui.layoutPodcastPlaylistResults),
	)
}

func (ui *UI) layoutPodcastPlaylistResults(gtx layout.Context) layout.Dimensions {
	state := ui.spaces.State(SpacePodcasts)
	if len(state.PodcastPlaylists) == 0 {
		return layout.Dimensions{}
	}
	if len(ui.spaces.PodcastPlaylistClicks) < len(state.PodcastPlaylists) {
		ui.spaces.PodcastPlaylistClicks = make([]widget.Clickable, len(state.PodcastPlaylists))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutSectionHeader(gtx, "Podcast playlists", "YouTube playlist search results.")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var rows []layout.FlexChild
			for i := range state.PodcastPlaylists {
				idx := i
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					pl := state.PodcastPlaylists[idx]
					if ui.spaces.PodcastPlaylistClicks[idx].Clicked(gtx) && ui.account != nil {
						state.SelectedPlaylist = &pl
						request := ui.beginSpaceRequest(SpacePodcasts, loadContent)
						source := ui.account
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
							defer cancel()
							_, videos, err := source.PlaylistVideos(ctx, pl.URL)
							ui.sendAction(func() {
								if ui.requestCurrent(request) && err == nil {
									state.Episodes = videos
									state.Queue = append([]yt.Video(nil), videos...)
								}
							})
							ui.invalidate()
						}()
					}
					return ui.spaces.PodcastPlaylistClicks[idx].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = gtx.Dp(44)
						lbl := ui.label(unit.Sp(14), pl.Title)
						lbl.Color = ui.colors.Text
						return layout.Inset{Left: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, lbl.Layout)
					})
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		}),
	)
}

func (ui *UI) layoutDemoShowsGrid(gtx layout.Context, shows []DemoShow) layout.Dimensions {
	cols := 4
	if gtx.Constraints.Max.X < gtx.Dp(750) {
		cols = 2
	}
	gap := gtx.Dp(16)
	totalGap := gap * (cols - 1)
	cardW := (gtx.Constraints.Max.X - totalGap) / cols
	if cardW < gtx.Dp(120) {
		cardW = gtx.Dp(120)
	}

	var rows []layout.FlexChild
	for r := 0; r < len(shows); r += cols {
		end := r + cols
		if end > len(shows) {
			end = len(shows)
		}
		rowShows := shows[r:end]
		rowIdx := r

		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var colChildren []layout.FlexChild
			for cIdx, s := range rowShows {
				showItem := s
				itemIdx := rowIdx + cIdx
				if cIdx > 0 {
					colChildren = append(colChildren, layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout))
				}
				colChildren = append(colChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(cardW, cardW+gtx.Dp(56)))
					if itemIdx < len(ui.spaces.ShowClicks) {
						if ui.spaces.ShowClicks[itemIdx].Clicked(gtx) {
							ui.state.Status = fmt.Sprintf("Opened show: %s", showItem.Title)
							ui.invalidate()
						}
						if ui.spaces.ShowClicks[itemIdx].Hovered() {
							pointer.CursorPointer.Add(gtx.Ops)
						}
						return ui.spaces.ShowClicks[itemIdx].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return ui.layoutDemoShowCard(gtx, showItem, cardW)
						})
					}
					return ui.layoutDemoShowCard(gtx, showItem, cardW)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, colChildren...)
		}))
		if end < len(shows) {
			rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout))
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (ui *UI) layoutDemoShowCard(gtx layout.Context, s DemoShow, width int) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		// Square cover art, center-cropped, text NEVER drawn on top
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var img image.Image
			if s.ImageFile != "" {
				img, _ = ui.getDemoImage(s.ImageFile)
			}
			return ui.drawCoverImage(gtx, img, image.Pt(width, width), unit.Dp(8))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		// Show Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(13), s.Title)
			lbl.Color = ui.colors.Text
			lbl.Font.Weight = font.Bold
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
		// Creator & sample show label
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(11), fmt.Sprintf("%s · Sample show", s.Creator))
			lbl.Color = ui.colors.Text2
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
	)
}

func (ui *UI) layoutLiveShowsGrid(gtx layout.Context, shows []yt.PodcastShow) layout.Dimensions {
	cols := 4
	if gtx.Constraints.Max.X < gtx.Dp(750) {
		cols = 2
	}
	gap := gtx.Dp(16)
	totalGap := gap * (cols - 1)
	cardW := (gtx.Constraints.Max.X - totalGap) / cols
	if cardW < gtx.Dp(120) {
		cardW = gtx.Dp(120)
	}

	if len(ui.spaces.ShowClicks) < len(shows) {
		ui.spaces.ShowClicks = make([]widget.Clickable, len(shows))
	}

	var rows []layout.FlexChild
	for r := 0; r < len(shows); r += cols {
		end := r + cols
		if end > len(shows) {
			end = len(shows)
		}
		rowShows := shows[r:end]
		rowIdx := r

		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var colChildren []layout.FlexChild
			for cIdx, s := range rowShows {
				showItem := s
				itemIdx := rowIdx + cIdx
				if cIdx > 0 {
					colChildren = append(colChildren, layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout))
				}
				colChildren = append(colChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(cardW, cardW+gtx.Dp(56)))
					click := &ui.spaces.ShowClicks[itemIdx]
					if click.Clicked(gtx) {
						if showItem.URL != "" && ui.account != nil {
							ui.loadPodcastShowEpisodes(showItem)
						}
					}
					if click.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return ui.layoutLiveShowCard(gtx, showItem, cardW)
					})
				}))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, colChildren...)
		}))
		if end < len(shows) {
			rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout))
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (ui *UI) layoutLiveShowCard(gtx layout.Context, s yt.PodcastShow, width int) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
		// Square cover art, center-cropped, text NEVER drawn on top
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var img image.Image
			if s.ID != "" && ui.thumbs != nil {
				ui.thumbs.Request(s.ID, s.ThumbnailURL)
				img, _ = ui.thumbs.Get(s.ID)
			}
			return ui.drawCoverImage(gtx, img, image.Pt(width, width), unit.Dp(8))
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(13), s.Title)
			lbl.Color = ui.colors.Text
			lbl.Font.Weight = font.Bold
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
		// Channel / Genre
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			desc := s.Channel
			if s.Genre != "" {
				desc = fmt.Sprintf("%s · %s", s.Channel, s.Genre)
			}
			lbl := ui.label(unit.Sp(11), desc)
			lbl.Color = ui.colors.Text2
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
	)
}

// layoutPodcastsEpisodesSection renders ordered episode rows.
func (ui *UI) layoutPodcastsEpisodesSection(gtx layout.Context) layout.Dimensions {
	state := ui.spaces.State(SpacePodcasts)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Section header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutSectionHeader(gtx, "Your episode queue", "Episodes play in order. Positions are saved automatically.")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		// Episode rows
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.demo && ui.demoData != nil {
				return ui.layoutDemoEpisodesList(gtx, ui.demoData.Episodes)
			}
			if len(state.Episodes) > 0 {
				return ui.layoutLiveEpisodesList(gtx, state.Episodes)
			}
			if ui.podcastsLoading {
				return ui.layoutEmptyMessage(gtx, "Loading episodes…")
			}
			return ui.layoutEmptyMessage(gtx, "Select a show above to view its episode queue.")
		}),
	)
}

func (ui *UI) layoutDemoEpisodesList(gtx layout.Context, episodes []DemoEpisode) layout.Dimensions {
	var children []layout.FlexChild
	for i, ep := range episodes {
		idx := i
		item := ep
		if idx < len(ui.spaces.EpisodeClicks) {
			click := &ui.spaces.EpisodeClicks[idx]
			if click.Clicked(gtx) {
				ui.state.Status = "Playback disabled in demo"
				ui.invalidate()
			}
			if click.Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.layoutDemoEpisodeRow(gtx, item)
				})
			}))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.layoutDemoEpisodeRow(gtx, item)
			}))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *UI) layoutDemoEpisodeRow(gtx layout.Context, ep DemoEpisode) layout.Dimensions {
	thumbW := gtx.Dp(88)
	thumbH := gtx.Dp(50)

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			// Bottom border divider
			bLine := image.Rect(0, gtx.Constraints.Min.Y-gtx.Dp(1), gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(bLine).Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(10),
				Bottom: unit.Dp(10),
				Left:   unit.Dp(4),
				Right:  unit.Dp(4),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
				}.Layout(gtx,
					// 16:9 thumbnail on left, center-cropped, text NEVER on image
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						var img image.Image
						if ep.ImageFile != "" {
							img, _ = ui.getDemoImage(ep.ImageFile)
						}
						return ui.drawCoverImage(gtx, img, image.Pt(thumbW, thumbH), unit.Dp(6))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					// Title, show name & episode number in the center
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(13), ep.Title)
								lbl.Color = ui.colors.Text
								lbl.Font.Weight = font.Bold
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(11), fmt.Sprintf("%s · %s", ep.Show, ep.Number))
								lbl.Color = ui.colors.Text2
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
							// Mini progress bar
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								barW := gtx.Dp(80)
								barH := gtx.Dp(2)
								progW := 0
								if ep.Duration > 0 && ep.Seconds > 0 {
									progW = int(float64(barW) * ep.Seconds / float64(ep.Duration))
								}
								if progW > barW {
									progW = barW
								}
								paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, 0, barW, barH)).Op())
								if progW > 0 {
									paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(image.Rect(0, 0, progW, barH)).Op())
								}
								return layout.Dimensions{Size: image.Pt(barW, barH)}
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
					// Time tail
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := ui.oneLineLabel(unit.Sp(12), ep.ProgressText)
						lbl.Color = ui.colors.Text2
						return lbl.Layout(gtx)
					}),
				)
			})
		},
	)
}

func (ui *UI) layoutLiveEpisodesList(gtx layout.Context, episodes []yt.Video) layout.Dimensions {
	if len(ui.spaces.EpisodeClicks) < len(episodes) {
		ui.spaces.EpisodeClicks = make([]widget.Clickable, len(episodes))
	}

	var children []layout.FlexChild
	for i, ep := range episodes {
		idx := i
		item := ep
		click := &ui.spaces.EpisodeClicks[idx]
		if click.Clicked(gtx) {
			ui.sendAction(func() { ui.selectPodcastEpisode(item, episodes) })
			ui.invalidate()
		}
		if click.Hovered() {
			pointer.CursorPointer.Add(gtx.Ops)
		}

		// Calculate resume / played position
		var posText string
		var posPct float32
		if p, ok := ui.getVideoPosition(item.ID); ok {
			if p.IsPlayed() {
				posText = "Played"
				posPct = 1.0
			} else if p.Seconds > 0 && p.Duration > 0 {
				rem := (p.Duration - p.Seconds) / 60
				if rem < 1 {
					rem = 1
				}
				posText = fmt.Sprintf("%d min left", int(rem))
				posPct = float32(p.Seconds) / float32(p.Duration)
			}
		}
		if posText == "" && item.Duration > 0 {
			posText = yt.FormatDuration(item.Duration)
		}

		thumbW := gtx.Dp(88)
		thumbH := gtx.Dp(50)

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						bLine := image.Rect(0, gtx.Constraints.Min.Y-gtx.Dp(1), gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
						paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(bLine).Op())
						return layout.Dimensions{Size: gtx.Constraints.Min}
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Top:    unit.Dp(10),
							Bottom: unit.Dp(10),
							Left:   unit.Dp(4),
							Right:  unit.Dp(4),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{
								Axis:      layout.Horizontal,
								Alignment: layout.Middle,
							}.Layout(gtx,
								// 16:9 thumbnail on left, center-cropped, text NEVER on image
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									var img image.Image
									if item.ID != "" && ui.thumbs != nil {
										ui.thumbs.Request(item.ID, item.ThumbnailURL)
										img, _ = ui.thumbs.Get(item.ID)
									}
									return ui.drawCoverImage(gtx, img, image.Pt(thumbW, thumbH), unit.Dp(6))
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
								// Title & channel
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := ui.label(unit.Sp(13), item.Title)
											lbl.Color = ui.colors.Text
											lbl.Font.Weight = font.Bold
											lbl.MaxLines = 1
											return lbl.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := ui.label(unit.Sp(11), item.Channel)
											lbl.Color = ui.colors.Text2
											lbl.MaxLines = 1
											return lbl.Layout(gtx)
										}),
										layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											barW := gtx.Dp(80)
											barH := gtx.Dp(2)
											progW := int(float32(barW) * posPct)
											paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, 0, barW, barH)).Op())
											if progW > 0 {
												paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(image.Rect(0, 0, progW, barH)).Op())
											}
											return layout.Dimensions{Size: image.Pt(barW, barH)}
										}),
									)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := ui.label(unit.Sp(12), posText)
									lbl.Color = ui.colors.Text2
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
							)
						})
					},
				)
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *UI) selectPodcastEpisode(item yt.Video, episodes []yt.Video) {
	state := ui.spaces.State(SpacePodcasts)
	state.Queue = append([]yt.Video(nil), episodes...)
	state.ResumeItem = &item
	ui.playVideo(item, false)
	ui.invalidate()
}

func (ui *UI) playPodcastsDock() {
	state := ui.spaces.State(SpacePodcasts)
	if state.ResumeItem == nil {
		return
	}
	if ui.ownsPlayer(SpacePodcasts, state.ResumeItem.ID) {
		ui.togglePlaybackPause()
	} else {
		ui.selectPodcastEpisode(*state.ResumeItem, state.Queue)
	}
	ui.invalidate()
}

// layoutPodcastsDockSection renders the visible player dock in Podcasts space.
// Controls go below the video subsurface!
func (ui *UI) layoutPodcastsDockSection(gtx layout.Context, vidW, vidH int, isPlaying bool) layout.Dimensions {
	state := ui.spaces.State(SpacePodcasts)

	var title, sub, tail string
	var imgFile string

	if ui.demo && ui.demoData != nil && len(ui.demoData.Episodes) > 0 {
		ep := ui.demoData.Episodes[0]
		title = ep.Title
		sub = ep.Show + " · Visible player preview"
		tail = "7:00 / 30:00"
		imgFile = ep.ImageFile
	} else if state.ResumeItem != nil {
		title = state.ResumeItem.Title
		sub = state.ResumeItem.Channel + " · Visible player"
		tail = formatPlaybackTime(ui.spaceItemPosition(SpacePodcasts, state.ResumeItem.ID), float64(state.ResumeItem.Duration))
	} else {
		title = "No episode playing"
		sub = "Select an episode to play"
		tail = "--:--"
	}

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
			paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(12),
				Bottom: unit.Dp(12),
				Left:   unit.Dp(16),
				Right:  unit.Dp(16),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Start,
				}.Layout(gtx,
					// 1. Video area on top
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(vidW, vidH))
						if ui.debugVideoMarker && isPlaying {
							paint.FillShape(gtx.Ops, color.NRGBA{R: 0x00, G: 0xff, B: 0x01, A: 0xff}, clip.Rect(image.Rect(0, 0, vidW, vidH)).Op())
							return layout.Dimensions{Size: image.Pt(vidW, vidH)}
						}
						if isPlaying {
							// Subsurface video is visible; keep empty Gio area
							return layout.Dimensions{Size: image.Pt(vidW, vidH)}
						}
						var img image.Image
						if ui.demo && imgFile != "" {
							img, _ = ui.getDemoImage(imgFile)
						} else if state.ResumeItem != nil && ui.thumbs != nil {
							ui.thumbs.Request(state.ResumeItem.ID, state.ResumeItem.ThumbnailURL)
							img, _ = ui.thumbs.Get(state.ResumeItem.ID)
						}
						return ui.drawCoverImage(gtx, img, image.Pt(vidW, vidH), unit.Dp(8))
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutPodcastDockControls(gtx, title, sub, tail)
					}),
				)
			})
		},
	)
}

func (ui *UI) layoutSectionHeader(gtx layout.Context, heading, sub string) layout.Dimensions {
	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(16), heading)
			lbl.Color = ui.colors.Text
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(12), sub)
			lbl.Color = ui.colors.Text2
			return lbl.Layout(gtx)
		}),
	)
}

func (ui *UI) layoutEmptyMessage(gtx layout.Context, msg string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(24), Bottom: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := ui.label(unit.Sp(13), msg)
		lbl.Color = ui.colors.Text2
		return lbl.Layout(gtx)
	})
}

func (ui *UI) recordPodcastTime(gtx layout.Context, tail string) (op.CallOp, layout.Dimensions) {
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max.X = 1 << 20
	measure := op.Record(gtx.Ops)
	label := ui.oneLineLabel(unit.Sp(10), tail)
	label.Color = ui.colors.Text2
	dims := label.Layout(gtx)
	measure.Stop()
	// Record with the measured bounds so the text and its accessible area agree.
	gtx.Constraints = layout.Exact(dims.Size)
	record := op.Record(gtx.Ops)
	label.Layout(gtx)
	return record.Stop(), dims
}

func (ui *UI) layoutPodcastTimeline(gtx layout.Context, tail string) layout.Dimensions {
	timeCall, timeDims := ui.recordPodcastTime(gtx, tail)
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		// Gio measures rigid children before giving the remaining width to the bar.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Constraints.Max.X, gtx.Dp(3)
			bounds := image.Rect(0, 0, width, height)
			defer clip.Rect(bounds).Push(gtx.Ops).Pop()
			semantic.LabelOp("Podcast progress").Add(gtx.Ops)
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(bounds).Op())
			played := width * 23 / 100
			state := ui.spaces.State(SpacePodcasts)
			if state.ResumeItem != nil && state.ResumeItem.Duration > 0 {
				played = int(float64(width) * ui.spaceItemPosition(SpacePodcasts, state.ResumeItem.ID) / float64(state.ResumeItem.Duration))
			}
			played = max(0, min(width, played))
			paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(image.Rect(0, 0, played, height)).Op())
			return layout.Dimensions{Size: image.Pt(width, height)}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: timeDims.Size}.Push(gtx.Ops).Pop()
			timeCall.Add(gtx.Ops)
			return timeDims
		}),
	)
}

func (ui *UI) layoutPodcastDockControls(gtx layout.Context, title, sub, tail string) layout.Dimensions {
	_, timeDims := ui.recordPodcastTime(gtx, tail)
	narrow := gtx.Constraints.Max.X < gtx.Dp(600)
	play := func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(44)
		gtx.Constraints = layout.Exact(image.Pt(size, size))
		return ui.spaces.PodcastsDockPlay.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			drawPlayPauseIcon(gtx, image.Rect(0, 0, size, size), ui.colors.Text, ui.watchPaused || ui.embedPlayer == nil)
			return layout.Dimensions{Size: image.Pt(size, size)}
		})
	}
	info := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := ui.oneLineLabel(unit.Sp(12), title)
				label.Font.Weight = font.Bold
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := ui.oneLineLabel(unit.Sp(10), sub)
				label.Color = ui.colors.Text2
				return label.Layout(gtx)
			}),
		)
	}
	if narrow {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, info), layout.Rigid(play),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.layoutPodcastTimeline(gtx, tail) }),
		)
	}
	// Limit the title before it can consume the bar's minimum width or the time.
	titleWidth := max(0, gtx.Constraints.Max.X-timeDims.Size.X-gtx.Dp(80+44+12+12+12))
	titleWidth = min(titleWidth, gtx.Dp(280))
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = titleWidth
			gtx.Constraints.Min.X = 0
			return info(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(play),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return ui.layoutPodcastTimeline(gtx, tail) }),
	)
}

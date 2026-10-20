package ui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"lighttube/internal/yt"
)

// layoutMusicSpace renders the Music space with an ambient glow behind the feature album,
// numbered track rows with artwork and duration, and a usable compact player dock with controls below the video.
func (ui *UI) layoutMusicSpace(gtx layout.Context) layout.Dimensions {
	state := ui.spaces.State(SpaceMusic)
	chip := state.Chip

	// Trigger live load if needed
	if !ui.demo && !state.ContentLoaded && !ui.musicLoading {
		ui.loadMusicSpaceContent(chip)
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
	if ui.spaces.MusicDockPlay.Clicked(gtx) {
		ui.playMusicDock()
	}

	isPlaying := state.ResumeItem != nil && ui.ownsPlayer(SpaceMusic, state.ResumeItem.ID)
	dockGtx := gtx
	dockGtx.Constraints.Min.X = dockW
	dockGtx.Constraints.Max.X = dockW
	dockGtx.Constraints.Min.Y = 0
	dockRecord := op.Record(gtx.Ops)
	dockDims := ui.layoutMusicDockSection(dockGtx, vidW, vidH, isPlaying)
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
		// 1. Scrollable feature album card & tracks list
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtxList := gtx
			gtxList.Constraints = layout.Exact(image.Pt(winW, scrollListH))
			return layout.Inset{
				Left:  padX,
				Right: padX,
			}.Layout(gtxList, func(gtx layout.Context) layout.Dimensions {
				return state.ScrollList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// 1. Feature album card with ambient glow
						layout.Rigid(ui.layoutMusicFeatureSection),
						layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
						// 2. Numbered tracks list
						layout.Rigid(ui.layoutMusicTracksSection),
					)
				})
			})
		}),
		// 2. Compact player / dock pinned at bottom of viewport outside scroll list
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			off := op.Offset(image.Pt(padXPx, dockRelY)).Push(gtx.Ops)
			dockCall.Add(gtx.Ops)
			off.Pop()
			return layout.Dimensions{Size: image.Pt(dockW, dockDims.Size.Y)}
		}),
	)
}

// layoutMusicFeatureSection renders the album feature block with purple ambient glow.
func (ui *UI) layoutMusicFeatureSection(gtx layout.Context) layout.Dimensions {
	ambientEnabled := ui.settings.AmbientEnabled()
	winW := gtx.Constraints.Max.X
	isNarrow := winW < gtx.Dp(750)

	var eyebrow, title, desc string
	var coverImg image.Image

	if ui.demo && ui.demoData != nil {
		alb := ui.demoData.Album
		eyebrow = "MADE FOR THE LATE HOURS · SAMPLE COLLECTION"
		title = alb.Title
		desc = alb.Description
		if alb.ImageFile != "" {
			coverImg, _ = ui.getDemoImage(alb.ImageFile)
		}
	} else {
		state := ui.spaces.State(SpaceMusic)
		if state.Album != nil {
			title = state.Album.Title
			desc = fmt.Sprintf("%s · Collection", state.Album.Artist)
			if state.Album.ThumbnailURL != "" && ui.thumbs != nil {
				ui.thumbs.Request(state.Album.ID, state.Album.ThumbnailURL)
				coverImg, _ = ui.thumbs.Get(state.Album.ID)
			}
		} else {
			title = "Liked music"
			if len(state.Tracks) == 1 {
				desc = "1 track"
			} else {
				desc = fmt.Sprintf("%d tracks", len(state.Tracks))
			}
			if len(state.Tracks) > 0 && ui.thumbs != nil {
				t0 := state.Tracks[0]
				if t0.ThumbnailURL != "" {
					ui.thumbs.Request(t0.ID, t0.ThumbnailURL)
				}
				coverImg, _ = ui.thumbs.Get(t0.ID)
			}
		}
	}

	// Action buttons
	if ui.spaces.MusicOpenColBtn.Clicked(gtx) {
		state := ui.spaces.State(SpaceMusic)
		state.Chip = 1
		ui.loadMusicSpaceContent(1)
		ui.state.Status = fmt.Sprintf("Opened collection: %s", title)
		ui.invalidate()
	}
	if ui.spaces.MusicSaveColBtn.Clicked(gtx) {
		state := ui.spaces.State(SpaceMusic)
		album := state.Album
		if album == nil && len(state.Tracks) > 0 {
			album = &yt.Album{ID: state.Tracks[0].Description, Title: title, Artist: state.Tracks[0].Channel, Tracks: append([]yt.Video(nil), state.Tracks...)}
		}
		if album == nil || ui.store == nil {
			ui.state.Status = "No collection is available to save."
		} else if err := ui.store.SaveCollection(ui.collectionAccountID(), *album); err != nil {
			ui.state.Status = "Could not save collection."
		} else {
			ui.state.Status = fmt.Sprintf("Saved collection: %s", album.Title)
		}
		ui.invalidate()
	}

	coverSize := gtx.Dp(196)
	if isNarrow {
		coverSize = gtx.Dp(160)
	}

	return layout.Stack{}.Layout(gtx,
		// Background ambient glow (rgba(80,57,134,.20))
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			cardBounds := image.Rect(0, 0, gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
			ui.ambientShape.DrawMusicGlow(gtx, cardBounds, ambientEnabled)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(14))
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
					paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Top:    unit.Dp(24),
						Bottom: unit.Dp(24),
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if isNarrow {
							return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return ui.drawCoverImage(gtx, coverImg, image.Pt(coverSize, coverSize), unit.Dp(8))
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return ui.layoutAlbumCopy(gtx, eyebrow, title, desc)
								}),
							)
						}

						return layout.Flex{
							Axis:      layout.Horizontal,
							Alignment: layout.Middle,
						}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return ui.drawCoverImage(gtx, coverImg, image.Pt(coverSize, coverSize), unit.Dp(8))
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(28)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return ui.layoutAlbumCopy(gtx, eyebrow, title, desc)
							}),
						)
					})
				},
			)
		}),
	)
}

func (ui *UI) layoutAlbumCopy(gtx layout.Context, eyebrow, title, desc string) layout.Dimensions {
	var children []layout.FlexChild

	if eyebrow != "" {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := ui.label(unit.Sp(10), eyebrow)
				lbl.Color = ui.colors.Text2
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		)
	}

	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(30), title)
			lbl.Color = ui.colors.Text
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(12), desc)
			lbl.Color = ui.colors.Text2
			lbl.MaxLines = 2
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutPillButton(gtx, &ui.spaces.MusicOpenColBtn, "Open collection", true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutPillButton(gtx, &ui.spaces.MusicSaveColBtn, "Save collection", false)
				}),
			)
		}),
	)

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
}

// layoutMusicTracksSection renders the numbered tracks list.
func (ui *UI) layoutMusicTracksSection(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.layoutSectionHeader(gtx, "Tracks, albums, and your queue", "Ordered tracks")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.demo && ui.demoData != nil {
				return ui.layoutDemoTracksList(gtx, ui.demoData.Tracks)
			}
			state := ui.spaces.State(SpaceMusic)
			if len(state.Tracks) > 0 {
				return ui.layoutLiveTracksList(gtx, state.Tracks)
			}
			if ui.musicLoading {
				return ui.layoutEmptyMessage(gtx, "Loading music tracks…")
			}
			switch state.Chip {
			case 0:
				return ui.layoutEmptyMessage(gtx, "Sign in to load your Liked Music playlist.")
			case 1:
				return ui.layoutEmptyMessage(gtx, "Search Music to find albums and tracks.")
			case 2:
				return ui.layoutEmptyMessage(gtx, "No saved music collections.")
			default:
				return ui.layoutEmptyMessage(gtx, "No music tracks found.")
			}
		}),
	)
}

func (ui *UI) layoutDemoTracksList(gtx layout.Context, tracks []DemoTrack) layout.Dimensions {
	if len(ui.spaces.TrackClicks) < len(tracks) {
		ui.spaces.TrackClicks = make([]widget.Clickable, len(tracks))
	}

	var children []layout.FlexChild
	for i, trk := range tracks {
		idx := i
		item := trk
		click := &ui.spaces.TrackClicks[idx]
		if click.Clicked(gtx) {
			ui.state.Status = "Playback disabled in demo"
			ui.invalidate()
		}
		if click.Hovered() {
			pointer.CursorPointer.Add(gtx.Ops)
		}

		numStr := fmt.Sprintf("%02d", idx+1)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.layoutTrackRow(gtx, numStr, item.Title, item.Artist, item.Album, item.DurationStr, item.ImageFile, click.Hovered())
			})
		}))
		if i < len(tracks)-1 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(1)
				paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X, h)).Op())
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
			}))
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *UI) layoutLiveTracksList(gtx layout.Context, tracks []yt.Video) layout.Dimensions {
	if len(ui.spaces.TrackClicks) < len(tracks) {
		ui.spaces.TrackClicks = make([]widget.Clickable, len(tracks))
	}

	var children []layout.FlexChild
	for i, trk := range tracks {
		idx := i
		item := trk
		click := &ui.spaces.TrackClicks[idx]
		if click.Clicked(gtx) {
			ui.sendAction(func() { ui.selectMusicTrack(item, tracks) })
			ui.invalidate()
		}
		if click.Hovered() {
			pointer.CursorPointer.Add(gtx.Ops)
		}

		numStr := fmt.Sprintf("%02d", idx+1)
		durStr := ""
		if item.Duration > 0 {
			durStr = yt.FormatDuration(item.Duration)
		}
		collection := item.Description

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.layoutLiveTrackRow(gtx, numStr, item.ID, item.Title, item.Channel, collection, durStr, item.ThumbnailURL, click.Hovered())
			})
		}))
		if i < len(tracks)-1 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(1)
				paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X, h)).Op())
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
			}))
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *UI) selectMusicTrack(item yt.Video, tracks []yt.Video) {
	state := ui.spaces.State(SpaceMusic)
	state.Queue = append([]yt.Video(nil), tracks...)
	state.ResumeItem = &item
	ui.playVideo(item, false)
	ui.invalidate()
}

func (ui *UI) layoutTrackRow(gtx layout.Context, num, title, artist, collection, dur, imgFile string, hovered bool) layout.Dimensions {
	bgCol := ui.colors.Bg
	if hovered {
		bgCol = ui.colors.Raised
	}
	winW := gtx.Constraints.Max.X
	hideCollection := winW < gtx.Dp(650)

	var img image.Image
	if imgFile != "" {
		img, _ = ui.getDemoImage(imgFile)
	}

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, bgCol, clip.Rect(image.Rectangle{Max: gtx.Constraints.Min}).Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(10),
				Bottom: unit.Dp(10),
				Left:   unit.Dp(8),
				Right:  unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// Track number (24 dp width)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(24), gtx.Dp(20)))
						lbl := ui.label(unit.Sp(12), num)
						lbl.Color = ui.colors.Text2
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					// 44x44 dp art
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						artSize := gtx.Dp(44)
						return ui.drawCoverImage(gtx, img, image.Pt(artSize, artSize), unit.Dp(4))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
					// Title & Artist
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(13), title)
								lbl.Color = ui.colors.Text
								lbl.Font.Weight = font.Bold
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(11), artist)
								lbl.Color = ui.colors.Text2
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
						)
					}),
					// Collection name (desktop only)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if hideCollection {
							return layout.Dimensions{}
						}
						return layout.Inset{Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(12), collection)
							lbl.Color = ui.colors.Text2
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						})
					}),
					// Duration
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(12), dur)
						lbl.Color = ui.colors.Text2
						return lbl.Layout(gtx)
					}),
				)
			})
		},
	)
}

func (ui *UI) layoutLiveTrackRow(gtx layout.Context, num, id, title, artist, collection, dur, thumbURL string, hovered bool) layout.Dimensions {
	bgCol := ui.colors.Bg
	if hovered {
		bgCol = ui.colors.Raised
	}
	winW := gtx.Constraints.Max.X
	hideCollection := winW < gtx.Dp(650)

	var img image.Image
	if ui.thumbs != nil {
		if thumbURL != "" {
			ui.thumbs.Request(id, thumbURL)
		}
		img, _ = ui.thumbs.Get(id)
	}

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, bgCol, clip.Rect(image.Rectangle{Max: gtx.Constraints.Min}).Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(10),
				Bottom: unit.Dp(10),
				Left:   unit.Dp(8),
				Right:  unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(24), gtx.Dp(20)))
						lbl := ui.label(unit.Sp(12), num)
						lbl.Color = ui.colors.Text2
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						artSize := gtx.Dp(44)
						return ui.drawCoverImage(gtx, img, image.Pt(artSize, artSize), unit.Dp(4))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(13), title)
								lbl.Color = ui.colors.Text
								lbl.Font.Weight = font.Bold
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(11), artist)
								lbl.Color = ui.colors.Text2
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if hideCollection {
							return layout.Dimensions{}
						}
						return layout.Inset{Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := ui.label(unit.Sp(12), collection)
							lbl.Color = ui.colors.Text2
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(12), dur)
						lbl.Color = ui.colors.Text2
						return lbl.Layout(gtx)
					}),
				)
			})
		},
	)
}

func (ui *UI) playMusicDock() {
	state := ui.spaces.State(SpaceMusic)
	if state.ResumeItem == nil {
		return
	}
	if ui.demo {
		ui.state.Status = "Playback disabled in demo"
	} else if ui.ownsPlayer(SpaceMusic, state.ResumeItem.ID) {
		ui.togglePlaybackPause()
	} else {
		ui.selectMusicTrack(*state.ResumeItem, state.Queue)
	}
	ui.invalidate()
}

// layoutMusicDockSection renders the compact player dock pinned at bottom of viewport.
// Crucial Wayland rule: the video is a subsurface; never draw Gio content over it; controls go BELOW the video!
func (ui *UI) layoutMusicDockSection(gtx layout.Context, vidW, vidH int, isPlaying bool) layout.Dimensions {
	state := ui.spaces.State(SpaceMusic)

	var trackTitle, trackSub, trackTime string
	var imgFile string

	if ui.demo && ui.demoData != nil {
		d := ui.demoData.DockTrack
		trackTitle = d.Title
		trackSub = "Visible player preview · No audio"
		trackTime = d.DurationStr
		imgFile = d.ImageFile
	} else {
		if state.ResumeItem != nil {
			trackTitle = fmt.Sprintf("%s · %s", state.ResumeItem.Title, state.ResumeItem.Channel)
			trackSub = "YouTube playback · Visible player"
			trackTime = formatPlaybackTime(ui.spaceItemPosition(SpaceMusic, state.ResumeItem.ID), float64(state.ResumeItem.Duration))
		} else {
			trackTitle = "No track playing"
			trackSub = "Select a track to play"
			trackTime = "--:--"
		}
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
					// 1. TOP: Video box
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
					// 2. BOTTOM: Player controls row STRICTLY BELOW the video!
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{
							Axis:      layout.Horizontal,
							Alignment: layout.Middle,
						}.Layout(gtx,
							// Track info
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := ui.label(unit.Sp(12), trackTitle)
										lbl.Color = ui.colors.Text
										lbl.Font.Weight = font.Bold
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := ui.label(unit.Sp(10), trackSub)
										lbl.Color = ui.colors.Text2
										lbl.MaxLines = 1
										return lbl.Layout(gtx)
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
							// Play/pause control button
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if ui.spaces.MusicDockPlay.Hovered() {
									pointer.CursorPointer.Add(gtx.Ops)
								}
								btnSize := gtx.Dp(36)
								gtx.Constraints = layout.Exact(image.Pt(btnSize, btnSize))
								return ui.spaces.MusicDockPlay.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									r := clip.UniformRRect(image.Rect(0, 0, btnSize, btnSize), btnSize/2)
									paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
									drawPlayPauseIcon(gtx, image.Rect(0, 0, btnSize, btnSize), ui.colors.Text, ui.watchPaused || ui.embedPlayer == nil)
									return layout.Dimensions{Size: image.Pt(btnSize, btnSize)}
								})
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
							// Scrub bar (3 dp height, accent fill)
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								barW := gtx.Constraints.Max.X
								barH := gtx.Dp(3)
								progW := barW * 36 / 100
								if state.ResumeItem != nil && state.ResumeItem.Duration > 0 {
									progW = int(float64(barW) * (ui.spaceItemPosition(SpaceMusic, state.ResumeItem.ID) / float64(state.ResumeItem.Duration)))
								}
								if progW > barW {
									progW = barW
								}
								paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, 0, barW, barH)).Op())
								paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(image.Rect(0, 0, progW, barH)).Op())
								return layout.Dimensions{Size: image.Pt(barW, barH)}
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							// Time display
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.oneLineLabel(unit.Sp(10), trackTime)
								lbl.Color = ui.colors.Text2
								return lbl.Layout(gtx)
							}),
						)
					}),
				)
			})
		},
	)
}

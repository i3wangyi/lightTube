package ui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"lighttube/internal/yt"
)

// layoutShortsSpace renders the Shorts space with a 9:16 portrait stage, action buttons,
// side exploration panel, and explicit unsupported state for Explore in live mode.
func (ui *UI) layoutShortsSpace(gtx layout.Context) layout.Dimensions {
	state := ui.spaces.State(SpaceShorts)
	chip := state.Chip

	// Live mode: Explore chip (2) shows the explicit unsupported state
	if !ui.demo && chip == 2 {
		return ui.layoutShortsExploreUnsupported(gtx)
	}

	// Live mode: Following chip (0) or Search results
	if !ui.demo {
		if chip == 0 && len(state.Shorts) == 0 && !ui.shortsLoading {
			ui.loadShortsSpaceContent(0)
		}
		if ui.shortsLoading && len(state.Shorts) == 0 {
			return ui.layoutShortsStatusCard(gtx, "Searching Shorts…", "Loading Shorts from followed channels without blocking.")
		}
		if len(state.Shorts) == 0 {
			if chip == 1 {
				return ui.layoutShortsStatusCard(gtx, "No saved Shorts", "Save a Short to access it here.")
			}
			return ui.layoutShortsStatusCard(gtx, "No Shorts available", "Follow channels that produce Shorts or search for Shorts above.")
		}
	}

	// In demo mode or live with items: render portrait stage
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	isNarrow := winW < gtx.Dp(800)

	portW := gtx.Dp(320)
	portH := gtx.Dp(565)
	padY := gtx.Dp(16)
	if winH > padY*2 && portH > winH-padY*2 {
		portH = winH - padY*2
		portW = portH * 9 / 16
	}

	var stageRelX, stageRelY int
	if isNarrow {
		wrapW := portW + gtx.Dp(16) + gtx.Dp(44)
		stageRelX = (winW - wrapW) / 2
		if stageRelX < gtx.Dp(12) {
			stageRelX = gtx.Dp(12)
		}
		stageRelY = padY
	} else {
		totalW := portW + gtx.Dp(16) + gtx.Dp(44) + gtx.Dp(48) + gtx.Dp(310)
		stageRelX = (winW - totalW) / 2
		if stageRelX < gtx.Dp(24) {
			stageRelX = gtx.Dp(24)
		}
		stageRelY = padY
	}

	// Register exact window-pixel bounds for embedded player subsurface every frame
	stageRect := image.Rect(
		ui.spaceOrigin.X+stageRelX,
		ui.spaceOrigin.Y+stageRelY,
		ui.spaceOrigin.X+stageRelX+portW,
		ui.spaceOrigin.Y+stageRelY+portH,
	)
	if ui.embedPlayer != nil && state.ResumeItem != nil && chip == 0 {
		ui.registerVideoBox(stageRect)
	} else {
		ui.registerVideoBox(image.Rectangle{})
	}

	if isNarrow {
		return layout.Inset{
			Left:   unit.Dp(float32(stageRelX) / float32(gtx.Metric.PxPerDp)),
			Top:    unit.Dp(float32(stageRelY) / float32(gtx.Metric.PxPerDp)),
			Bottom: unit.Dp(16),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtxWrap := gtx
					gtxWrap.Constraints = layout.Exact(image.Pt(portW+gtx.Dp(16)+gtx.Dp(44), portH))
					return ui.layoutShortsPortraitWrap(gtxWrap, portW, portH)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Rigid(ui.layoutShortsAside),
			)
		})
	}

	asideX := stageRelX + portW + gtx.Dp(16) + gtx.Dp(44) + gtx.Dp(48)
	asideW := gtx.Dp(310)
	if asideX+asideW > winW-gtx.Dp(24) {
		asideW = winW - gtx.Dp(24) - asideX
	}

	return layout.Inset{
		Left:   unit.Dp(float32(stageRelX) / float32(gtx.Metric.PxPerDp)),
		Top:    unit.Dp(float32(stageRelY) / float32(gtx.Metric.PxPerDp)),
		Bottom: unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtxWrap := gtx
				gtxWrap.Constraints = layout.Exact(image.Pt(portW+gtx.Dp(16)+gtx.Dp(44), portH))
				return ui.layoutShortsPortraitWrap(gtxWrap, portW, portH)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(48)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = asideW
				gtx.Constraints.Max.X = asideW
				return ui.layoutShortsAside(gtx)
			}),
		)
	})
}

// layoutShortsPortraitWrap renders the 9:16 portrait container plus right-side action buttons.
func (ui *UI) layoutShortsPortraitWrap(gtx layout.Context, portW, portH int) layout.Dimensions {
	state := ui.spaces.State(SpaceShorts)

	var currentShort yt.Video
	var demoShort *DemoShort
	var shortNum, shortTotal int

	if ui.demo && ui.demoData != nil && len(ui.demoData.Shorts) > 0 {
		shortTotal = len(ui.demoData.Shorts)
		idx := state.ShortIndex % shortTotal
		if idx < 0 {
			idx += shortTotal
		}
		demoShort = &ui.demoData.Shorts[idx]
		shortNum = idx + 1
		currentShort = yt.Video{
			ID:          demoShort.ID,
			Title:       demoShort.Title,
			Channel:     demoShort.Channel,
			Description: demoShort.Caption,
			URL:         demoShort.SourceLink,
		}
	} else if len(state.Shorts) > 0 {
		shortTotal = len(state.Shorts)
		idx := state.ShortIndex % shortTotal
		if idx < 0 {
			idx += shortTotal
		}
		currentShort = state.Shorts[idx]
		shortNum = idx + 1
	}

	// Previous / Next button handlers
	if ui.spaces.ShortsPrevClick.Clicked(gtx) {
		if shortTotal > 0 {
			state.ShortIndex = (state.ShortIndex - 1 + shortTotal) % shortTotal
			ui.invalidate()
		}
	}
	if ui.spaces.ShortsNextClick.Clicked(gtx) {
		if shortTotal > 0 {
			state.ShortIndex = (state.ShortIndex + 1) % shortTotal
			ui.invalidate()
		}
	}
	if ui.spaces.ShortsSaveClick.Clicked(gtx) {
		if ui.demo {
			ui.state.Status = "Short saved to this preview session."
		} else {
			if currentShort.ID != "" && ui.store != nil {
				_ = ui.store.AddWatchLater(currentShort)
				ui.state.Status = fmt.Sprintf("Saved %q to Library", currentShort.Title)
			}
		}
		ui.invalidate()
	}
	if ui.spaces.ShortsStageClick.Clicked(gtx) && currentShort.ID != "" {
		item := currentShort
		shorts := append([]yt.Video(nil), state.Shorts...)
		ui.sendAction(func() { ui.selectShort(item, shorts) })
		ui.invalidate()
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		// Portrait stage (320x565 dp or scaled)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(portW, portH))
			return ui.spaces.ShortsStageClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.layoutPortraitCard(gtx, currentShort, demoShort, shortNum, shortTotal)
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
		// Vertical action buttons (↑, ↓, Save)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{
				Axis:      layout.Vertical,
				Alignment: layout.Middle,
			}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutRoundIconButton(gtx, &ui.spaces.ShortsPrevClick, drawUpArrowIcon, "Previous Short")
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutRoundIconButton(gtx, &ui.spaces.ShortsNextClick, drawDownArrowIcon, "Next Short")
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutRoundIconButton(gtx, &ui.spaces.ShortsSaveClick, drawStarIcon, "Save Short")
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := ui.label(unit.Sp(10), "Save")
					lbl.Color = ui.colors.Text2
					return lbl.Layout(gtx)
				}),
			)
		}),
	)
}

func (ui *UI) selectShort(item yt.Video, shorts []yt.Video) {
	state := ui.spaces.State(SpaceShorts)
	state.Queue = append([]yt.Video(nil), shorts...)
	state.ResumeItem = &item
	ui.playVideo(item, false)
	ui.invalidate()
}

// layoutPortraitCard renders the card inside the 9:16 portrait stage.
func (ui *UI) layoutPortraitCard(gtx layout.Context, v yt.Video, demoShort *DemoShort, num, total int) layout.Dimensions {
	w := gtx.Constraints.Max.X
	h := gtx.Constraints.Max.Y

	if ui.debugVideoMarker && ui.embedPlayer != nil && ui.spaces.State(SpaceShorts).ResumeItem != nil {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 0x00, G: 0xff, B: 0x01, A: 0xff}, clip.Rect(image.Rect(0, 0, w, h)).Op())
		return layout.Dimensions{Size: image.Pt(w, h)}
	}

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			// Video / image background scaled to cover and center-cropped
			var img image.Image
			if demoShort != nil {
				img, _ = ui.getDemoImage(demoShort.ImageFile)
			} else if v.ID != "" {
				if ui.thumbs != nil {
					ui.thumbs.Request(v.ID, v.ThumbnailURL)
					img, _ = ui.thumbs.Get(v.ID)
				}
			}
			return ui.drawCoverImage(gtx, img, image.Pt(w, h), unit.Dp(12))
		},
		func(gtx layout.Context) layout.Dimensions {
			// Dark gradient overlay for typography readability (scrim)
			gradH := gtx.Dp(180)
			if gradH > h/2 {
				gradH = h / 2
			}
			paint.FillShape(gtx.Ops, ui.colors.Overlay, clip.Rect(image.Rect(0, 0, w, gtx.Dp(50))).Op())
			paint.FillShape(gtx.Ops, ui.colors.Overlay, clip.Rect(image.Rect(0, h-gradH, w, h)).Op())

			return layout.Flex{
				Axis:      layout.Vertical,
				Spacing:   layout.SpaceBetween,
				Alignment: layout.Start,
			}.Layout(gtx,
				// Top badge: Muted · Captions on
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(16), Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						rBadge := clip.UniformRRect(image.Rect(0, 0, gtx.Dp(140), gtx.Dp(26)), gtx.Dp(13))
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, ui.colors.Overlay, rBadge.Op(gtx.Ops))
								return layout.Dimensions{Size: image.Pt(gtx.Dp(140), gtx.Dp(26))}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := ui.label(unit.Sp(10), "Muted · Captions on")
									lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
									lbl.Font.Weight = font.Medium
									return lbl.Layout(gtx)
								})
							},
						)
					})
				}),
				// Center title
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := ui.label(unit.Sp(18), v.Title)
						lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					})
				}),
				// Bottom caption and creator info
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Left:   unit.Dp(16),
						Right:  unit.Dp(16),
						Bottom: unit.Dp(16),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(12), v.Channel)
								lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
								lbl.Font.Weight = font.Bold
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								desc := v.Description
								if desc == "" && ui.demo {
									desc = "Sample short preview"
								}
								if desc == "" {
									return layout.Dimensions{}
								}
								lbl := ui.label(unit.Sp(11), desc)
								lbl.Color = ui.colors.Text2
								lbl.MaxLines = 2
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !ui.demo {
									return layout.Dimensions{}
								}
								note := fmt.Sprintf("Sample %d of %d · Still-image preview", num, total)
								lbl := ui.label(unit.Sp(9), note)
								lbl.Color = ui.colors.Text2
								return lbl.Layout(gtx)
							}),
						)
					})
				}),
			)
		},
	)
}

// layoutShortsAside renders the right-side "More to explore" panel.
func (ui *UI) layoutShortsAside(gtx layout.Context) layout.Dimensions {
	state := ui.spaces.State(SpaceShorts)

	var currentShort yt.Video
	if ui.demo && ui.demoData != nil && len(ui.demoData.Shorts) > 0 {
		idx := state.ShortIndex % len(ui.demoData.Shorts)
		if idx < 0 {
			idx += len(ui.demoData.Shorts)
		}
		s := ui.demoData.Shorts[idx]
		currentShort = yt.Video{
			ID:          s.ID,
			Title:       s.Title,
			Channel:     s.Channel,
			Description: s.Caption,
			URL:         s.SourceLink,
		}
	} else if len(state.Shorts) > 0 {
		idx := state.ShortIndex % len(state.Shorts)
		if idx < 0 {
			idx += len(state.Shorts)
		}
		currentShort = state.Shorts[idx]
	}

	// Check if a real source link exists
	realSourceLink := yt.ExtractFullVideoSourceLink(currentShort.Description, "")
	if ui.demo && currentShort.URL != "" {
		realSourceLink = currentShort.URL
	}

	// Action: Next short from related card
	if ui.spaces.ShortsRelatedClick.Clicked(gtx) {
		if ui.demo && ui.demoData != nil {
			state.ShortIndex = (state.ShortIndex + 1) % len(ui.demoData.Shorts)
		} else if len(state.Shorts) > 0 {
			state.ShortIndex = (state.ShortIndex + 1) % len(state.Shorts)
		}
		ui.invalidate()
	}

	// Action: Open in Watch
	if ui.spaces.ShortsOpenWatchBtn.Clicked(gtx) {
		ui.switchSpace(SpaceWatch)
		watchVideo := currentShort
		if realSourceLink != "" {
			if id, err := yt.ExtractVideoID(realSourceLink); err == nil && id != "" {
				watchVideo.ID = id
				watchVideo.URL = realSourceLink
			}
		}
		// Selects video in Watch space, does NOT autoplay
		ui.spaces.State(SpaceWatch).ResumeItem = &watchVideo
		ui.state.Status = fmt.Sprintf("Opened in Watch space: %s (playback paused)", watchVideo.Title)
		ui.invalidate()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(18), "More to explore")
			lbl.Color = ui.colors.Text
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := ui.label(unit.Sp(12), "Stay in Shorts, or choose to open the full story in Watch.")
			lbl.Color = ui.colors.Text2
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),

		// 1. Related Short card
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.demo && ui.demoData != nil {
				if ui.spaces.ShortsRelatedClick.Hovered() {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				return ui.spaces.ShortsRelatedClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.layoutAsideCard(gtx, "A minute away from everything", "Field Notes · Sample short", "forest.jpg", nil)
				})
			}
			if len(state.Shorts) > 1 {
				nextIdx := (state.ShortIndex + 1) % len(state.Shorts)
				nextShort := state.Shorts[nextIdx]
				var img image.Image
				if ui.thumbs != nil && nextShort.ID != "" {
					ui.thumbs.Request(nextShort.ID, nextShort.ThumbnailURL)
					img, _ = ui.thumbs.Get(nextShort.ID)
				}
				if ui.spaces.ShortsRelatedClick.Hovered() {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				return ui.spaces.ShortsRelatedClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.layoutAsideCard(gtx, nextShort.Title, nextShort.Channel, "", img)
				})
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !ui.demo && len(state.Shorts) <= 1 {
				return layout.Dimensions{}
			}
			return layout.Spacer{Height: unit.Dp(12)}.Layout(gtx)
		}),

		// 2. Open full video / Open this Short in Watch
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if currentShort.ID == "" && !ui.demo {
				return layout.Dimensions{}
			}
			if ui.spaces.ShortsOpenWatchBtn.Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			openLabel := "Open this Short in Watch ↗"
			cardTitle := currentShort.Title
			imgFile := ""
			var liveImg image.Image
			if ui.demo {
				cardTitle = "The full story behind the frame"
				imgFile = "portrait-city.jpg"
			} else {
				if ui.thumbs != nil && currentShort.ID != "" {
					ui.thumbs.Request(currentShort.ID, currentShort.ThumbnailURL)
					liveImg, _ = ui.thumbs.Get(currentShort.ID)
				}
			}
			if realSourceLink != "" {
				openLabel = "Open full video in Watch ↗"
			}
			return ui.spaces.ShortsOpenWatchBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.layoutAsideCard(gtx, cardTitle, openLabel, imgFile, liveImg)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),

		// Scope note (demo only)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !ui.demo {
				return layout.Dimensions{}
			}
			lbl := ui.label(unit.Sp(11), "Explore · three sample shorts")
			lbl.Color = ui.colors.Text2
			return lbl.Layout(gtx)
		}),
	)
}

func (ui *UI) layoutAsideCard(gtx layout.Context, title, subtitle, imgFile string, liveImg image.Image) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
			paint.FillShape(gtx.Ops, ui.colors.Raised, r.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(10),
				Bottom: unit.Dp(10),
				Left:   unit.Dp(12),
				Right:  unit.Dp(12),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
				}.Layout(gtx,
					// Thumbnail 88x50 dp (16:9), center-cropped, text strictly to the right
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						thumbW := gtx.Dp(88)
						thumbH := gtx.Dp(50)
						var img image.Image
						if ui.demo && imgFile != "" {
							img, _ = ui.getDemoImage(imgFile)
						} else if liveImg != nil {
							img = liveImg
						}
						return ui.drawCoverImage(gtx, img, image.Pt(thumbW, thumbH), unit.Dp(6))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(12), title)
								lbl.Color = ui.colors.Text
								lbl.Font.Weight = font.Bold
								lbl.MaxLines = 2
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(11), subtitle)
								lbl.Color = ui.colors.Text2
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							}),
						)
					}),
				)
			})
		},
	)
}

func (ui *UI) layoutRoundIconButton(gtx layout.Context, click *widget.Clickable, drawIcon func(gtx layout.Context, bounds image.Rectangle, col color.NRGBA), tip string) layout.Dimensions {
	if click.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	size := gtx.Dp(44)
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		r := clip.UniformRRect(image.Rect(0, 0, size, size), size/2)
		bgCol := ui.colors.Raised
		if click.Hovered() {
			bgCol = ui.colors.Hover
		}
		paint.FillShape(gtx.Ops, bgCol, r.Op(gtx.Ops))
		paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())

		if drawIcon != nil {
			iconBox := image.Rect(0, 0, size, size)
			drawIcon(gtx, iconBox, ui.colors.Text)
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

// layoutShortsExploreUnsupported displays the explicit unsupported message required for Explore in live mode,
// wrapping text cleanly within the card column at all window widths.
func (ui *UI) layoutShortsExploreUnsupported(gtx layout.Context) layout.Dimensions {
	maxW := gtx.Constraints.Max.X - gtx.Dp(48)
	if maxW > gtx.Dp(800) {
		maxW = gtx.Dp(800)
	}
	if maxW < gtx.Dp(280) {
		maxW = gtx.Constraints.Max.X
	}

	return layout.Inset{
		Top:    unit.Dp(24),
		Bottom: unit.Dp(24),
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			innerW := maxW - gtx.Dp(48)
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
					paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
					paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				},
				func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = innerW
					gtx.Constraints.Min.X = innerW
					return layout.Inset{
						Top:    unit.Dp(36),
						Bottom: unit.Dp(36),
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(18), "YouTube does not offer a Shorts feed to this app")
								lbl.Color = ui.colors.Text
								lbl.Font.Weight = font.Bold
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(13), "YouTube's public endpoints do not expose an unauthenticated global Shorts algorithmic explore stream.")
								lbl.Color = ui.colors.Text2
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(13), "Select 'Following' to view Shorts from channels you subscribe to, or search for Shorts using the search box above.")
								lbl.Color = ui.colors.Text2
								return lbl.Layout(gtx)
							}),
						)
					})
				},
			)
		})
	})
}

func (ui *UI) layoutShortsStatusCard(gtx layout.Context, title, body string) layout.Dimensions {
	maxW := gtx.Constraints.Max.X - gtx.Dp(48)
	if maxW > gtx.Dp(800) {
		maxW = gtx.Dp(800)
	}

	return layout.Inset{
		Top:    unit.Dp(24),
		Bottom: unit.Dp(24),
		Left:   unit.Dp(24),
		Right:  unit.Dp(24),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			innerW := maxW - gtx.Dp(48)
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
					paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
					paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				},
				func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = innerW
					gtx.Constraints.Min.X = innerW
					return layout.Inset{
						Top:    unit.Dp(36),
						Bottom: unit.Dp(36),
						Left:   unit.Dp(24),
						Right:  unit.Dp(24),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(18), title)
								lbl.Color = ui.colors.Text
								lbl.Font.Weight = font.Bold
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := ui.label(unit.Sp(13), body)
								lbl.Color = ui.colors.Text2
								return lbl.Layout(gtx)
							}),
						)
					})
				},
			)
		})
	})
}

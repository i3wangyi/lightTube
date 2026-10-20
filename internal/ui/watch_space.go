package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

// layoutWatchSpace renders the Watch space matching Phase 10:
// Chips: For you, Following, Continue, Saved.
// Hero on For you without a query: latest unfinished video (paused, real thumbnail, "Continue watching", resume time), else first feed item.
// Then 3-column landscape grid (2 at medium, 1 narrow).
func (ui *UI) layoutWatchSpace(gtx layout.Context) layout.Dimensions {
	activeChip := ui.spaces.Active().Chip
	query := strings.TrimSpace(ui.searchEditor.Text())
	showHero := (activeChip == 0 && query == "")

	hero, heroPos, hasHero := ui.getWatchHero()

	// Filter videos for the grid (excluding the hero video on For you tab to avoid repetition)
	var gridVideos []yt.Video
	if showHero && hasHero {
		for _, v := range ui.state.Videos {
			if v.ID != hero.ID {
				gridVideos = append(gridVideos, v)
			}
		}
	} else {
		gridVideos = ui.state.Videos
	}

	sectionHeading := "Recommended for you"
	if query != "" {
		sectionHeading = fmt.Sprintf("Search results for %q", query)
	} else {
		switch activeChip {
		case 1:
			sectionHeading = "From your creators"
		case 2:
			sectionHeading = "Continue watching"
		case 3:
			sectionHeading = "Saved videos"
		}
	}

	countStr := fmt.Sprintf("%d videos", len(gridVideos))
	if ui.demo {
		countStr = fmt.Sprintf("%d sample videos", len(gridVideos))
	}

	padX := unit.Dp(24)
	if gtx.Constraints.Max.X < gtx.Dp(600) {
		padX = unit.Dp(12)
	}

	return ui.spaces.Active().ScrollList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		var children []layout.FlexChild

		// 1. Hero section on For you tab
		if showHero && hasHero {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: padX, Right: padX, Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.layoutWatchHero(gtx, hero, heroPos)
				})
			}))
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(28)}.Layout))
		}

		// 2. Section heading & count
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: padX, Right: padX, Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.th, unit.Sp(18), sectionHeading)
						lbl.Color = ui.colors.Text
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.th, unit.Sp(12), countStr)
						lbl.Color = ui.colors.Text2
						return lbl.Layout(gtx)
					}),
				)
			})
		}))

		// 3. Responsive video grid
		if len(gridVideos) == 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: padX, Right: padX, Top: unit.Dp(24), Bottom: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					msg := "No videos match this filter."
					if activeChip == 2 {
						msg = "No unfinished videos to continue watching."
					} else if activeChip == 3 {
						msg = "No saved videos yet."
					}
					lbl := material.Label(ui.th, unit.Sp(13), msg)
					lbl.Color = ui.colors.Text2
					return lbl.Layout(gtx)
				})
			}))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: padX, Right: padX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return ui.layoutResponsiveGrid(gtx, gridVideos)
				})
			}))
		}

		// 4. Bottom spacing inside scroll view
		children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout))

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// layoutWatchHero renders the hero unit with paused player preview and resume context.
func (ui *UI) layoutWatchHero(gtx layout.Context, hero yt.Video, pos store.Position) layout.Dimensions {
	w := gtx.Constraints.Max.X
	isWide := w >= gtx.Dp(900)

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			cardBounds := image.Rectangle{Max: gtx.Constraints.Min}
			// Draw static cached ambient glow behind hero card matching exact dimensions
			ui.ambientShape.DrawWatchGlow(gtx, cardBounds, ui.settings.AmbientEnabled())

			r := clip.UniformRRect(cardBounds, gtx.Dp(14))
			paint.FillShape(gtx.Ops, ui.colors.Surface, r.Op(gtx.Ops))
			return layout.Dimensions{Size: cardBounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:    unit.Dp(16),
				Bottom: unit.Dp(16),
				Left:   unit.Dp(16),
				Right:  unit.Dp(16),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if isWide {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// Left: 16:9 player preview
						layout.Flexed(1.55, func(gtx layout.Context) layout.Dimensions {
							return ui.layoutHeroPlayerPreview(gtx, hero, pos)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
						// Right: Hero copy & actions
						layout.Flexed(1.0, func(gtx layout.Context) layout.Dimensions {
							return ui.layoutHeroCopy(gtx, hero, pos)
						}),
					)
				}

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutHeroPlayerPreview(gtx, hero, pos)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutHeroCopy(gtx, hero, pos)
					}),
				)
			})
		},
	)
}

// layoutHeroPlayerPreview renders the 16:9 paused player preview with controls and scrub bar.
func (ui *UI) layoutHeroPlayerPreview(gtx layout.Context, hero yt.Video, pos store.Position) layout.Dimensions {
	w := gtx.Constraints.Max.X
	h := w * 9 / 16
	if h > gtx.Dp(308) {
		h = gtx.Dp(308)
		w = h * 16 / 9
	}
	boxSize := image.Pt(w, h)
	gtx.Constraints.Min = boxSize
	gtx.Constraints.Max = boxSize

	rrect := clip.UniformRRect(image.Rectangle{Max: boxSize}, gtx.Dp(11))

	var imgOp paint.ImageOp
	hasImg := false

	if ui.demo {
		if img, ok := ui.getDemoImage("city.jpg"); ok && img != nil {
			op, cached := ui.imageOps["hero_city"]
			if !cached {
				op = paint.NewImageOp(img)
				ui.imageOps["hero_city"] = op
			}
			imgOp = op
			hasImg = true
		}
	} else if ui.thumbs != nil {
		if img, ok := ui.thumbs.Get(hero.ID); ok && img != nil {
			op, cached := ui.imageOps[hero.ID]
			if !cached {
				op = paint.NewImageOp(img)
				ui.imageOps[hero.ID] = op
			}
			imgOp = op
			hasImg = true
		}
	}

	if ui.heroPreviewClick.Clicked(gtx) {
		if ui.demo {
			ui.state.Status = "Playback disabled in demo"
			ui.invalidate()
		} else {
			ui.playVideo(hero, false)
		}
	}

	if ui.heroPreviewClick.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
		if ui.hoverVideoID != hero.ID {
			ui.startHover(hero.ID, -1)
		}
		if ui.hoverVideoID == hero.ID {
			elapsed := gtx.Now.Sub(ui.hoverStartTime)
			if elapsed < 500*time.Millisecond {
				gtx.Execute(op.InvalidateCmd{At: ui.hoverStartTime.Add(500 * time.Millisecond)})
			}
		}
	} else if ui.hoverVideoID == hero.ID {
		ui.clearHover()
	}

	return ui.heroPreviewClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, ui.colors.Raised, rrect.Op(gtx.Ops))
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

				hasResume := pos.Duration > 0 && pos.Seconds > 0

				// Bottom player controls bar with scrub line only if position & duration known
				if hasResume {
					ctlH := gtx.Dp(46)
					ctlRect := image.Rect(0, h-ctlH, w, h)
					paint.FillShape(gtx.Ops, color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xaa}, clip.Rect(ctlRect).Op())

					// Scrubber line: background text2, played portion in Accent (#ff0033)
					scrubH := gtx.Dp(3)
					scrubY := h - ctlH
					paint.FillShape(gtx.Ops, ui.colors.Border, clip.Rect(image.Rect(0, scrubY, w, scrubY+scrubH)).Op())
					pct := pos.Seconds / pos.Duration
					if pct > 1.0 {
						pct = 1.0
					}
					playedW := int(float64(w) * pct)
					paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(image.Rect(0, scrubY, playedW, scrubY+scrubH)).Op())

					// Control buttons and time label
					offset := op.Offset(image.Pt(0, h-ctlH+scrubH)).Push(gtx.Ops)
					rowGtx := gtx
					rowGtx.Constraints = layout.Exact(image.Pt(w, ctlH-scrubH))
					layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(rowGtx, func(gtx layout.Context) layout.Dimensions {
						timeStr := fmt.Sprintf("%s / %s", formatSeconds(int(pos.Seconds)), formatSeconds(int(pos.Duration)))
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								sz := gtx.Dp(20)
								drawPlayTriangle(gtx, image.Rect(0, 0, sz, sz), color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
								return layout.Dimensions{Size: image.Pt(sz, sz)}
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.th, unit.Sp(11), timeStr)
								lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
								return lbl.Layout(gtx)
							}),
						)
					})
					offset.Pop()
				}

				return layout.Dimensions{Size: boxSize}
			},
		)
	})
}

// layoutHeroCopy renders the right column of the hero unit:
// Eyebrow, Title, Creator row, Description, and Action buttons.
func (ui *UI) layoutHeroCopy(gtx layout.Context, hero yt.Video, pos store.Position) layout.Dimensions {
	if ui.heroPrimaryClick.Clicked(gtx) {
		if ui.demo {
			ui.state.Status = "Playback disabled in demo"
			ui.invalidate()
		} else {
			ui.playVideo(hero, false)
		}
	}
	if ui.heroQuietClick.Clicked(gtx) {
		if ui.demo {
			ui.state.Status = "Saved in preview"
			ui.invalidate()
		} else {
			ui.toggleWatchLater(hero)
		}
	}

	eyebrow := "FEATURED VIDEO"
	if pos.IsUnfinished() && pos.Seconds > 0 {
		eyebrow = "CONTINUE WATCHING"
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Eyebrow
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.th, unit.Sp(11), eyebrow)
			lbl.Color = ui.colors.Text2
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.th, unit.Sp(22), hero.Title)
			lbl.Color = ui.colors.Text
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

		// Creator row: Avatar circle + creator name
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutAvatar(gtx, ui.th, hero.ChannelID, hero.Channel, 34)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.th, unit.Sp(13), hero.Channel)
							lbl.Color = ui.colors.Text
							lbl.Font.Weight = font.Medium
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							subText := "Architecture & city life"
							if !ui.demo && hero.ViewCount > 0 {
								subText = fmt.Sprintf("%d views", hero.ViewCount)
							}
							lbl := material.Label(ui.th, unit.Sp(11), subText)
							lbl.Color = ui.colors.Text2
							return lbl.Layout(gtx)
						}),
					)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

		// Description
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			desc := hero.Description
			if desc == "" {
				return layout.Dimensions{}
			}
			lbl := material.Label(ui.th, unit.Sp(12), desc)
			lbl.Color = ui.colors.Text2
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),

		// Action buttons
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				// Primary button (Action color #c90028)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.heroPrimaryClick.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					btnText := "Play"
					if pos.IsUnfinished() && pos.Seconds > 0 {
						btnText = "Resume"
					}
					return ui.heroPrimaryClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(22))
								paint.FillShape(gtx.Ops, ui.colors.Action, r.Op(gtx.Ops))
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.th, unit.Sp(13), btnText)
									lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
									lbl.Font.Weight = font.Bold
									return lbl.Layout(gtx)
								})
							},
						)
					})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				// Quiet button
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.heroQuietClick.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					return ui.heroQuietClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Background{}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(22))
								paint.FillShape(gtx.Ops, ui.colors.Border, clip.Stroke{Path: r.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
								return layout.Dimensions{Size: gtx.Constraints.Min}
							},
							func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.th, unit.Sp(12), "Save for later")
									lbl.Color = ui.colors.Text
									return lbl.Layout(gtx)
								})
							},
						)
					})
				}),
			)
		}),
	)
}

// layoutResponsiveGrid renders video cards in a 3-column landscape grid
// (collapses to 2 at medium width < 1050 dp, 1 at narrow width < 650 dp).
func (ui *UI) layoutResponsiveGrid(gtx layout.Context, videos []yt.Video) layout.Dimensions {
	w := gtx.Constraints.Max.X
	cols := 3
	if w < gtx.Dp(1050) {
		cols = 2
	}
	if w < gtx.Dp(650) {
		cols = 1
	}

	gap := gtx.Dp(18)
	cardW := (w - gap*(cols-1)) / cols
	if cardW < gtx.Dp(180) {
		cardW = gtx.Dp(180)
	}
	if cardW > w {
		cardW = w
	}

	// Group into rows
	var rows [][]yt.Video
	for i := 0; i < len(videos); i += cols {
		end := i + cols
		if end > len(videos) {
			end = len(videos)
		}
		rows = append(rows, videos[i:end])
	}

	for len(ui.cardClicks) < len(videos) {
		ui.cardClicks = append(ui.cardClicks, widget.Clickable{})
		ui.cardClockClicks = append(ui.cardClockClicks, widget.Clickable{})
	}

	var rowChildren []layout.FlexChild
	for rowIdx, row := range rows {
		rIdx := rowIdx
		rowVideos := row
		rowChildren = append(rowChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var colChildren []layout.FlexChild
			for colIdx, v := range rowVideos {
				cIdx := colIdx
				video := v
				globalIdx := rIdx*cols + cIdx

				colChildren = append(colChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					cgtx := gtx
					cgtx.Constraints = layout.Exact(image.Pt(cardW, cardW*9/16+gtx.Dp(90)))
					return ui.layoutVideoCard(cgtx, video, globalIdx, cardW)
				}))
				if cIdx < len(rowVideos)-1 {
					colChildren = append(colChildren, layout.Rigid(layout.Spacer{Width: unit.Dp(18)}.Layout))
				}
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, colChildren...)
		}))
		if rIdx < len(rows)-1 {
			rowChildren = append(rowChildren, layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout))
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)
}

// layoutVideoCard renders an individual card with 11 dp radius thumbnail, duration badge,
// resume progress bar (if unfinished), played state (if >= 95%), and creator metadata.
func (ui *UI) layoutVideoCard(gtx layout.Context, v yt.Video, idx int, width int) layout.Dimensions {
	click := &ui.cardClicks[idx]
	if click.Clicked(gtx) {
		if ui.demo {
			ui.state.Status = "Playback disabled in demo"
			ui.invalidate()
		} else {
			ui.playVideo(v, false)
		}
	}
	if click.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
		if ui.hoverVideoID != v.ID {
			ui.startHover(v.ID, idx)
		}
		if ui.hoverVideoID == v.ID {
			elapsed := gtx.Now.Sub(ui.hoverStartTime)
			if elapsed < 500*time.Millisecond {
				gtx.Execute(op.InvalidateCmd{At: ui.hoverStartTime.Add(500 * time.Millisecond)})
			}
		}
	} else if ui.hoverVideoID == v.ID {
		ui.clearHover()
	}

	thumbH := width * 9 / 16
	pos, hasPos := ui.getVideoPosition(v.ID)

	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// 1. Thumbnail box (16:9, radius 11 dp)
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				boxSize := image.Pt(width, thumbH)
				rrect := clip.UniformRRect(image.Rectangle{Max: boxSize}, gtx.Dp(11))

				var imgOp paint.ImageOp
				hasImg := false

				if ui.demo {
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
				}

				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, ui.colors.Raised, rrect.Op(gtx.Ops))
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

						// Duration badge at bottom-right
						durText := yt.FormatDuration(v.Duration)
						if durText != "" {
							offset := op.Offset(image.Pt(width-gtx.Dp(48), thumbH-gtx.Dp(24))).Push(gtx.Ops)
							badgeGtx := gtx
							badgeGtx.Constraints = layout.Exact(image.Pt(gtx.Dp(42), gtx.Dp(18)))
							layout.Background{}.Layout(badgeGtx,
								func(gtx layout.Context) layout.Dimensions {
									r := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(4))
									paint.FillShape(gtx.Ops, ui.colors.Overlay, r.Op(gtx.Ops))
									return layout.Dimensions{Size: gtx.Constraints.Min}
								},
								func(gtx layout.Context) layout.Dimensions {
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(ui.th, unit.Sp(10), durText)
										lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
										lbl.Font.Weight = font.Medium
										return lbl.Layout(gtx)
									})
								},
							)
							offset.Pop()
						}

						// Stored position progress line at bottom (Accent color #ff0033)
						if hasPos && pos.IsUnfinished() {
							progH := gtx.Dp(3)
							progW := int(float64(width) * (pos.Percent() / 100.0))
							paint.FillShape(gtx.Ops, ui.colors.Accent, clip.Rect(image.Rect(0, thumbH-progH, progW, thumbH)).Op())
						}

						return layout.Dimensions{Size: boxSize}
					},
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

			// 2. Metadata: Avatar + Title + Channel
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutAvatar(gtx, ui.th, v.ChannelID, v.Channel, 32)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.th, unit.Sp(13), v.Title)
								lbl.Color = ui.colors.Text
								lbl.Font.Weight = font.Medium
								lbl.MaxLines = 2
								return lbl.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								meta := v.Channel
								if hasPos && pos.IsPlayed() {
									meta += " • Played"
								} else if hasPos && pos.IsUnfinished() {
									meta += fmt.Sprintf(" • Resumes at %s", formatSeconds(int(pos.Seconds)))
								}
								lbl := material.Label(ui.th, unit.Sp(11), meta)
								lbl.Color = ui.colors.Text2
								return lbl.Layout(gtx)
							}),
						)
					}),
				)
			}),
		)
	})
}

func demoFilenameForID(id string) string {
	switch id {
	case "demo_city":
		return "city.jpg"
	case "demo_workshop":
		return "workshop.jpg"
	case "demo_camera":
		return "camera.jpg"
	case "demo_performance":
		return "performance.jpg"
	case "demo_forest":
		return "forest.jpg"
	case "demo_architecture":
		return "architecture.jpg"
	case "demo_vinyl":
		return "vinyl.jpg"
	default:
		return "workshop.jpg"
	}
}

func formatSeconds(sec int) string {
	m := sec / 60
	s := sec % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func (ui *UI) getVideoPosition(id string) (store.Position, bool) {
	if ui.demo && ui.demoData != nil {
		pos, ok := ui.demoData.Positions[id]
		return pos, ok
	}
	if ui.store != nil {
		return ui.store.Position(ui.currentAccountID(), id)
	}
	return store.Position{}, false
}

func (ui *UI) getWatchHero() (yt.Video, store.Position, bool) {
	if ui.demo && ui.demoData != nil {
		return ui.demoData.Hero, ui.demoData.HeroPosition, true
	}

	// 1. Check for latest unfinished video in positions
	if ui.store != nil {
		unfin := ui.store.UnfinishedPositions(ui.currentAccountID())
		if len(unfin) > 0 {
			latest := unfin[0]
			for _, v := range ui.state.Videos {
				if v.ID == latest.VideoID {
					return v, latest.Position, true
				}
			}
			return yt.Video{
				ID:       latest.VideoID,
				Title:    latest.VideoID,
				Duration: int(latest.Position.Duration),
			}, latest.Position, true
		}
	}

	// 2. Otherwise return first video from feed
	if len(ui.state.Videos) > 0 {
		first := ui.state.Videos[0]
		pos, _ := ui.getVideoPosition(first.ID)
		return first, pos, true
	}

	return yt.Video{}, store.Position{}, false
}

func (ui *UI) loadWatchSpaceContent(chip int) {
	space := ui.spaces.State(SpaceWatch)
	ui.beginSpaceRequest(SpaceWatch, loadContent)
	space.ContentLoaded = true
	space.ContentGeneration = ui.currentGen()
	if ui.demo && ui.demoData != nil {
		switch chip {
		case 0: // For you
			ui.setWatchVideos(ui.demoData.Videos)
		case 1: // Following
			var vids []yt.Video
			for _, v := range ui.demoData.Videos {
				if v.Channel == "Object Studio · Demo" || v.Channel == "Field Notes · Demo" {
					vids = append(vids, v)
				}
			}
			ui.setWatchVideos(vids)
		case 2: // Continue
			var vids []yt.Video
			for _, v := range ui.demoData.Videos {
				if pos, ok := ui.demoData.Positions[v.ID]; ok && pos.IsUnfinished() {
					vids = append(vids, v)
				}
			}
			ui.setWatchVideos(vids)
		case 3: // Saved
			var vids []yt.Video
			for _, v := range ui.demoData.Videos {
				if ui.demoData.SavedTitles[v.Title] {
					vids = append(vids, v)
				}
			}
			ui.setWatchVideos(vids)
		}
		return
	}

	// Live mode
	switch chip {
	case 0: // For you
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedHome)
			ui.setWatchVideos(cached)
			ui.refreshAccountFeed(yt.FeedHome)
		} else {
			ui.loadMergedFeed()
		}
	case 1: // Following
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedSubs)
			ui.setWatchVideos(cached)
			ui.refreshAccountFeed(yt.FeedSubs)
		} else {
			ui.loadMergedFeed()
		}
	case 2: // Continue
		if ui.store != nil {
			unfin := ui.store.UnfinishedPositions(ui.currentAccountID())
			var vids []yt.Video
			for _, p := range unfin {
				// Watch Continue holds Watch items and legacy records
				// without provenance; other spaces keep their own lists.
				if p.Position.Space != "" && p.Position.Space != SpaceWatch.String() {
					continue
				}
				title := p.Position.Title
				if title == "" {
					title = p.VideoID
				}
				vids = append(vids, yt.Video{
					ID:       p.VideoID,
					Title:    title,
					Channel:  p.Position.Show,
					Duration: int(p.Position.Duration),
				})
			}
			ui.setWatchVideos(vids)
		}
	case 3: // Saved
		local := ui.store.WatchLater()
		if ui.account != nil && ui.account.SignedIn() {
			cached, _ := ui.account.Cached(yt.FeedWatchLater)
			ui.setWatchVideos(mergeVideos(cached, local))
			ui.refreshAccountFeed(yt.FeedWatchLater)
		} else {
			ui.setWatchVideos(local)
		}
	}
}

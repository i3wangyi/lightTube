# lightTube — visual design (YouTube-like, dark)

This file replaces the layout sketch in SPEC.md "Interface". Keys, data and
performance rules in SPEC.md still apply. All sizes are in dp (Gio `unit.Dp`).

## Colors (YouTube dark theme)

| Token        | Value     | Use                                         |
|--------------|-----------|---------------------------------------------|
| bg           | `#0f0f0f` | window background                           |
| surface      | `#272727` | chips, hover background, sidebar selected   |
| surfaceHover | `#3f3f3f` | chip hover                                  |
| border       | `#303030` | search box border                           |
| text         | `#f1f1f1` | titles, primary text                        |
| text2        | `#aaaaaa` | channel name, meta line, sidebar labels     |
| red          | `#ff0000` | logo badge, progress, focus accents         |
| badgeBg      | `#000000` at 80% alpha | duration badge                 |
| chipActive   | `#f1f1f1` bg, `#0f0f0f` text | selected chip          |

The window must paint `bg` itself (opaque), so it does not depend on the
compositor background.

## Layout

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [▶ lightTube]        ( Search                         )[🔍]   H.264≤1080p·AC │  top bar, 56 high
├──────┬──────────────────────────────────────────────────────────────────────┤
│ Home │ [All] [Linus Tech Tips] [No Boilerplate] …   (chips: filter by channel)│
│ Subs │ ┌────────────┐ ┌────────────┐ ┌────────────┐                            │
│ Later│ │ thumbnail  │ │            │ │            │   grid of cards             │
│ Hist │ │      12:34 │ │            │ │            │                            │
│      │ └────────────┘ └────────────┘ └────────────┘                            │
│ 72   │ (A) Title, bold, max 2 lines…                                          │
│ wide │     Channel name                                                       │
│      │     1.2M views · 3 days ago                                            │
├──────┴──────────────────────────────────────────────────────────────────────┤
│ status line (only when there is a message), 28 high, text2                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Top bar
- Left: logo — a red rounded rectangle (28x20, radius 6) with a white
  triangle, then "lightTube" in text color, 20 sp, medium weight.
- Center: pill search box, max width 640, height 40, radius 20, bg `#121212`,
  1 dp border `border`; focused border `#1c62b9`. A 64-wide button joined on
  the right (bg `surface`) with a magnifier glyph. Placeholder "Search".
- Paste: if the search box text is a YouTube URL, Enter plays it instead of
  searching. Keep `Ctrl+V` global paste-and-play. Remove the "Paste URL" button.
- Right: the policy label as a small chip (`surface` bg, text2, 12 sp).
- Search box fixes (phase 5; the phase 3 box is wrong):
  - The box is centered in the window width, not placed after the logo. It
    grows to 640 max and shrinks on narrow windows (min 200).
  - The text and placeholder are vertically centered in the 40 high box,
    16 sp, with 16 dp left inset and 12 dp right inset. One line only.
  - The border is the same 1 dp on all four sides of the input part; the
    button shares the right border and has a 1 dp `border` line on its left.
  - Height of the button = height of the box (40). The magnifier is 24 dp.
  - Optional like YouTube: an "x" clear button inside the box when it has text.

### Watch page (phase 5)
Opens on single click on a card or row. Layout like YouTube's watch page:

```
┌ top bar (same) ────────────────────────────────────────────────────────┐
│ ┌──────────────────────────────────────┐  Up next                      │
│ │                                      │  ┌──────┐ Title 14 sp, 2 lines│
│ │        video (subsurface), 16:9      │  │ 168w │ Channel 12 sp       │
│ │                                      │  └──────┘ views · age 12 sp   │
│ ├──────────────────────────────────────┤  ...                          │
│ │ ▶  ⏮ ⏭  🔊 ───  0:42 / 12:34    ⛶  │  (list = current feed or      │
│ └──────────────────────────────────────┘   search results, minus this) │
│ Title 20 sp medium, max 2 lines                                         │
│ (A) Channel name 16 sp medium   [Subscribe]       [Watch later] [mpv]   │
│ ┌ surface box radius 12: "1.2M views · 3 days ago" + description ────┐ │
└────────────────────────────────────────────────────────────────────────┘
```

- Video column width: window width − 24 − 402 (Up next 402 wide) on wide
  windows; on windows < 1000 wide, Up next goes below the description.
- Controls bar (Gio, below the video, never over it): play/pause, volume
  with mute, time "0:42 / 12:34", red progress bar (click and drag to seek,
  buffered part in `#717171`), fullscreen button. Poll mpv time at most 4
  times a second, and only while playing and the window is visible.
- Keys: Space/K play-pause, J/L −10/+10 s, ←/→ −5/+5 s, M mute,
  F fullscreen (Gio window fullscreen; video fills it, controls hidden until
  the mouse moves), Esc leaves fullscreen or goes back, O opens in external mpv.
- Click on the video area toggles pause; double click toggles fullscreen.
- Back: Esc or the guide item returns to the previous page and stops playback
  (YouTube's mini player is out of scope).

### Left mini guide (72 wide)
Four items, icon above label (icon 24, label 10 sp, text2; selected item has
`surface` rounded bg and text color): **Home** (merged subscription feed),
**Subscriptions** (manage channels), **Watch later**, **History**.
Search results replace the main area and set no guide item as selected.
Icons: draw simple vector shapes with Gio `clip`/`paint` (house, stacked
rectangles, clock, counter-clockwise arrow). No icon font, no image files.
`Ctrl+1..4` select these four items.

### Home, Watch later, History — card grid
- Columns = max(1, floor((width - 32) / (320 + 16))). Card width fills the
  column. Gap 16 horizontal, 40 vertical. Use one `layout.List` of rows so
  only visible rows are laid out.
- Thumbnail: 16:9, radius 12 (clip.RRect). While loading: `surface` rect.
- Duration badge: bottom-right, 4 dp inset, radius 4, `badgeBg`, white 12 sp
  medium. Hide when duration is unknown.
- Hover on a card: show a small "clock" button at the thumbnail top-right
  (bg `badgeBg`, radius 4). Click toggles watch later. Hover also sets a
  `surface` background behind the whole card with radius 12.
- Below the thumbnail, 12 dp gap: a 36 dp circle avatar on the left — a
  solid color derived from a hash of the channel id, with the channel's
  first letter in white. Then: title (16 sp, medium, text, max 2 lines with
  ellipsis), channel name (14 sp, text2), meta "1.2M views · 3 days ago"
  (14 sp, text2). Omit parts that are unknown.
- Chips row above the Home grid: "All" plus one chip per subscribed channel
  (horizontal scroll). Selecting a chip filters the grid. Chip: height 32,
  radius 8, `surface`; active chip uses chipActive colors.

### Search results — list (like YouTube search)
- One row per result: thumbnail 360 wide (16:9, radius 12, duration badge)
  on the left; on the right: title 18 sp medium max 2 lines; meta line
  "1.2M views · 3 days ago" 12 sp text2; channel row (24 dp avatar circle +
  channel name 12 sp text2). Row gap 16. Hover background like cards.

### Subscriptions (manage)
- A text field "Add channel: URL, @handle or UC id" + "Subscribe" button
  (red bg, white text, radius 18 — like YouTube's subscribe button).
- A text field for a Takeout CSV path + "Import" button (surface bg).
- List of channels: avatar circle, name, and a "Subscribed" chip that
  unsubscribes on click (surface bg).

### Interaction
- Single click on a card or row plays (YouTube behavior). `Enter` plays the
  selected item, `A` plays audio only, `W` toggles watch later,
  arrows move the selection. The selected item has a 2 dp `text` outline
  around the thumbnail.
- Pointer cursor over clickable things.
- Number formatting: 999, 1.2K, 34K, 1.2M, 3.4B. Age: "5 minutes ago",
  "3 hours ago", "2 days ago", "3 weeks ago", "4 months ago", "2 years ago".

### Fonts
- Use Noto Sans (Regular and Medium) for Latin text and Noto Sans CJK for
  Chinese, Japanese and Korean titles. All are installed under
  `/usr/share/fonts/noto` and `/usr/share/fonts/noto-cjk`.
- Do NOT let Gio scan every system font. Load only the needed font files
  into an explicit collection. Measure startup with both choices and keep
  the faster one that still renders CJK titles.

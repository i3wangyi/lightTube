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

### You / Account page (Settings)
- Account section: 48 dp avatar circle, name/email, signed in / signed out status,
  and a Sign in / Sign out button.
- Settings section (below account):
  - "Settings" section header (16 sp, bold).
  - **Codec** row: three chips (`Auto`, `Old GPU (H.264)`, `Any`; active chip uses
    `chipActive` colors). Beneath the chips: GPU decode summary in `text2` (12 sp),
    e.g. `GPU decodes: H.264 (iHD)`.
  - **Player** row: two chips (`In window`, `External mpv`).
  - **Login browser** row: text field for browser spec (`chrome+gnomekeyring` default)
    with a Save button.
  - Setting changes save immediately to `settings.json`.

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

## Phase 8: sharp thumbnails, account channels, moving thumbnails

Measured 2026-10-19: `i.ytimg.com/vi/<id>/hq720.jpg` is 1280x720 (~84 KB),
`hqdefault.jpg` 480x360 (4:3 with black bars), `mqdefault.jpg` 320x180 (the
phase 3 choice, too soft at scale 2). `yt-dlp --cookies <file> --flat-playlist
-J https://www.youtube.com/feed/channels` lists the account's channels in
about 1.4 s (id, title, follower count, avatar URL `//yt3.googleusercontent...`).
`yt-dlp -J <video>` (about 6 s) lists storyboard formats: sb0 = 320x180
frames, 3x3 per sheet, one frame per ~10 s, sheet URLs carry signed query
params (they cannot be built by hand).

### Thumbnails
- Fetch order: hq720.jpg, then hqdefault.jpg cropped to 16:9 (remove the
  bars), then mqdefault.jpg. Disk cache file `<id>-hq.jpg` (old `<id>.jpg`
  files are low-res; ignore them).
- Decode, then scale to the largest size shown: card width in px at the
  current scale, capped at 640x360. One RGBA copy, shared with the ImageOp.
- Memory LRU bounded by bytes (48 MB), not by count. The UI must never
  request more thumbnails in one frame than fit in half the budget: limit
  the prefetch margin (visible rows plus 1 row) and clamp the total.

### Account channels (signed in)
- `Account.Channels(ctx)` with a disk cache like the feeds; refresh once per
  Subs page open, at most one in flight.
- Subs page, top: a horizontal row "Channels" of avatar circles (64 dp,
  image from the avatar URL, fallback the letter circle) with the name
  below (12 sp, 1 line, ellipsis), horizontal scroll. Then the existing
  Subscriptions feed grid, then the manage section.
- Home chips row (signed in): "All" plus one chip per account channel.
- Click on a channel avatar or chip: show that channel's recent videos
  (the existing RSS channel feed) in the grid, with the channel name as a
  header and the chip active.
- Avatars use a small separate image cache (URL key, 128x128 px max,
  disk cache under the thumbs dir).

### Moving thumbnails (hover preview, like YouTube)
- After the pointer rests 500 ms on a card or search row thumbnail, fetch
  the video's storyboard in the background (yt-dlp -J with cookies, cached
  on disk per video for 24 h as the sheet URL list), then download up to 6
  sb0 sheets spread evenly over the video and play their frames in order
  in the thumbnail box, 400 ms per frame, looping. Show a thin red progress
  line at the thumbnail bottom while it plays.
- Only the hovered card animates. The frame timer (`op.InvalidateCmd` with
  `At`) runs only while hovered and the window is visible; leaving the card
  stops it and frees the frames. At most one storyboard fetch in flight;
  leaving the card cancels it.
- If there is no storyboard, keep the still thumbnail. Never block the UI.

## Phase 9: accounts, playlists, typeahead

Measured 2026-10-19 (yt-dlp 2026.08.19):
- `--cookies-from-browser BROWSER[+KEYRING][:PROFILE]` accepts a profile
  name or a profile directory path. Chrome lists its profiles in
  `~/.config/google-chrome/Local State` -> `profile.info_cache`
  (`name`, `user_name` = signed-in Google account).
- `https://www.youtube.com/feed/playlists` (with cookies, --flat-playlist -J):
  37 entries in 1.6 s, including LL (Liked videos) and WL (Watch later).
- Suggestions: `https://suggestqueries.google.com/complete/search?client=firefox&ds=yt&q=<q>`
  returns JSON `[q, [10 strings], ...]` in about 0.23 s, no login.
- `https://www.youtube.com/results?search_query=<q>&sp=<filter>` through
  yt-dlp: no filter = videos only; `EgIQAg==` channels (1.5 s);
  `EgIQAw==` playlists (1.5 s); `EgIQCQ==` Shorts (9.5 s, few results).

### Accounts (user-changeable, several accounts)
- An account = label + browser spec (`chrome+gnomekeyring`, plus a profile
  name or path) + its own cookie file and feed cache dir. Stored in
  settings.json. The existing single account becomes account "default"
  with its current cookie path, so nothing moves.
- You page: "Accounts" list (label, Google account name when known, active
  marker), "Switch", "Sign out", "Remove". Switching reloads all feeds.
- "Add account" offers:
  1. "Use a browser profile": list profiles found for Chrome, Chromium,
     Brave, Edge (Local State) and Firefox (profiles.ini), with the signed-in
     name; pick one, then sign in as today.
  2. "Sign in with Google": start Chrome (or Chromium/Brave, whichever is
     installed) with `--user-data-dir=<data>/browsers/<id> --no-first-run
     --no-default-browser-check` at
     `https://accounts.google.com/ServiceLogin?service=youtube&continue=https://www.youtube.com/`.
     When that browser process exits, extract cookies from that directory
     (`<browser>+<keyring>:<dir>/Default`). This works for any Google account
     and needs no OAuth client. Show "Finish signing in in the browser window,
     then close it". Removing the account deletes that directory.
- Never log cookie values; cookie files stay mode 0600.

### Saved playlists
- `Account.Playlists(ctx)` from feed/playlists, disk cache, one refresh per
  page open. You page: "Playlists" grid (title, video count when known,
  thumbnail of the first video if the entry has one, else surface box).
- Click opens the playlist (yt-dlp --flat-playlist -J of the playlist URL
  with cookies, private lists included) as a card grid with a header.

### Typeahead
- While the search box has focus and text, fetch suggestions 150 ms after
  the last keystroke; cancel the previous request; cache by prefix (LRU 200).
- Dropdown under the box: up to 10 rows, 40 dp, magnifier icon + text, the
  matching prefix in text2 and the rest in text (like YouTube). Up/Down move,
  Enter searches the selected row (or the typed text), Esc closes, click
  searches. The box keeps focus. No request for empty text or URLs.

### Mixed search results — dropped (2026-10-19)
The user chose to keep search to normal videos. Do not add channel,
playlist or Shorts search results.

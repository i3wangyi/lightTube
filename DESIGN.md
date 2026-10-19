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

## Phase 11: watch page like YouTube, comments, captions, chapters, downloads, channel pages

Runs after Phase 10 (four spaces), on the new shell: 11a player and watch
metadata, 11b comments and channel pages, 11c downloads.

Measured 2026-10-19 (yt-dlp 2026.08.19, mpv 0.41):
- `yt-dlp --cookies <f> -J --skip-download <video>` (about 6 s): `chapters`
  (start_time, end_time, title), `automatic_captions` (157 languages on the
  test video; `subtitles` may be empty), `like_count`, `comment_count`,
  `channel_follower_count`, description, formats ladder 144p-1080p for avc1
  (1440p/2160p only vp9/av01).
- Comments: `--write-comments --extractor-args
  "youtube:max_comments=20,20,5,2;comment_sort=top"` gives 20 comments with
  replies in about 6.6 s; fields author, author_thumbnail, text, like_count,
  _time_text, parent ("root" or the parent id), is_pinned,
  author_is_uploader. `comment_sort=new` gives newest.
- mpv with `ytdl-raw-options=sub-langs=<langs>,write-auto-subs=` adds the
  captions as external sub tracks (off until `sid` is set). mpv's
  `chapter-list` property has the chapters during playback.
- Channel: `<channel url>` -J --flat-playlist gives the header (title,
  channel_follower_count, description, thumbnails: avatar and banner up to
  2560 px wide); tabs `/videos` (30 in 1.3 s), `/shorts` (0.9 s),
  `/playlists` (0.9 s), `/streams` (returns null JSON when the channel has
  none: show "No live streams"). `?sort=p` does not change the order through
  yt-dlp: no Popular sort.

### Watch page metadata (background, never blocks playback)
- On open, start playback at once (as today) and in parallel run the -J
  metadata call (cache per video 1 h). When it arrives: like count,
  subscriber count, full description (collapsed to 3 lines with "...more"),
  chapters, caption languages, the format ladder.
- Description: timestamps (0:00, 1:02:03) and chapter titles are clickable
  and seek. URLs are shown as text (no click to open a browser in this
  phase).

### Player (in-window), as close to YouTube as practical
- Wayland limit: the video is a subsurface above the Gio window, so Gio
  cannot draw over it. The controls bar stays directly below the video
  (as in phase 5). In fullscreen it is a strip below the video area, shown
  on mouse move and while paused, hidden 3 s after the last move (the cursor
  hides too). Anything that must appear inside the video (bezel icons,
  buffering spinner, "Resumed at", autoplay countdown text, captions) is
  drawn by mpv itself: OSD messages / osd-overlay (ASS) and subtitles. Left: play/pause, next (plays the first Up next),
  volume button with a slider that opens on hover, time "0:42 / 12:34",
  chapter title "• Introduction >" (click opens the chapters list). Right:
  autoplay toggle, CC, settings gear, theater mode, fullscreen.
- Progress bar: red played part, #717171 buffered (mpv demuxer-cache-time),
  gaps between chapters, grows on hover; hover shows a tooltip with the
  time, the chapter title and the storyboard frame for that time (reuse
  phase 8 storyboard code, fetched once per video); click and drag seek.
- Big centered icon flash on play/pause, ±10 s, volume change (YouTube's
  "bezel"), 500 ms. Spinner while buffering (mpv paused-for-cache or
  seeking).
- Settings gear menu: Playback speed (0.25-2, default 1), Quality (Auto =
  the policy; then the avc1 heights 144p-1080p; vp9/av01 heights marked
  "software decode" and shown only when the codec policy is not old-gpu),
  Captions (Off, the languages: original first, then English, then the
  rest in a scrollable list; auto-translated ones work through sub-langs),
  Caption size (small/normal/large through sub-scale). Switching quality
  keeps the time (loadfile with start=<time>).
- Keys (YouTube): Space/K play-pause, J/L ∓10 s, ←/→ ∓5 s, ↑/↓ volume ±5,
  M mute, F fullscreen, T theater, C captions on/off, Shift+N next,
  Shift+P previous, < / > speed ∓0.25, 0-9 seek to 0-90 %, Home/End,
  , / . frame step while paused, Esc leaves fullscreen.
- Double-click on the video toggles fullscreen; single click toggles pause.
- Theater mode: video full width, Up next below. Persist volume, speed,
  captions on/off and language, theater mode in settings.json.
- Autoplay: at end, a 5 s countdown card over the video for the first Up
  next ("Up next in 5 · Cancel"); off when the autoplay toggle is off.
- Resume: start where the user stopped last time (mpv
  save-position-on-quit already; show "Resumed at 4:12").
- Mark watched on YouTube when signed in: pass yt-dlp `--mark-watched`
  through ytdl-raw-options (`mark-watched=`).

### Comments (below the description; Up next stays right)
- "N Comments" header, sort chips Top / Newest. Load 20 on first scroll into
  view (not on open), "Show more" loads 20 more (max_comments grows).
- Comment row: 40 dp avatar (author_thumbnail, letter fallback), author
  (pinned and uploader badges), relative time, text (5 lines, "Read more"),
  like count. "N replies" expands the replies already fetched.
- Text is plain; timestamps in comments seek like in the description.

### Downloads
- Watch page "Download" button and card context: choose quality (the avc1
  ladder; default the policy height) or "Audio only (m4a)". yt-dlp writes to
  the user's Videos dir + /lightTube (xdg-user-dir VIDEOS on Linux,
  ~/Movies on macOS, %USERPROFILE%\Videos on Windows), name
  "<title> [<id>].mp4", with --embed-subs (chosen caption language),
  --embed-chapters, --embed-thumbnail, cookies copy, --newline progress.
- At most 2 downloads at once; queue the rest. A "Downloads" item on the You
  page: list with progress bar, speed, ETA, cancel, "Show in folder" (xdg-open
  the folder), and play (in-window player with the local file; no network).
- Downloaded videos are marked with a check badge on cards. Survive restart:
  state in the data dir; partial downloads resume (yt-dlp --continue).

### Channel page (like YouTube)
- Open from: avatar or channel name on cards, the watch page, chips, the
  Subs channel row. Header: banner (widest thumbnail, cropped to 6:1),
  96 dp avatar, name, "@handle · N subscribers · N videos" when known,
  description first line, Subscribe (local) button.
- Tabs: Home (first 12 videos + the playlists shelf), Videos, Shorts
  (9:16 cards), Live, Playlists. Each tab loads on first open, 30 items,
  "Show more" loads 30 more (--playlist-start/--playlist-end), cache 1 h.
- Back (Esc, mouse back button) returns to the page before.

## Phase 10: four spaces redesign

Source of truth for the visual design: the published reference
(claude.ai artifact 6ufKKKFDcE9v4pmpYmyq68, Version 5) and its design
document (B1sXC7rsPUuSjjP6pEUpQp). The .dc.html files are design-editor
components: translate them to Gio; never add a browser UI. The design
session (the design review session) does the acceptance review. This section replaces
the older colors and the 72 dp guide as the default shell; keys, data,
playback and performance rules above still apply.

### Tokens (replace the phase 3 table)
bg #0f0f0f, surface #181818, raised #272727, hover #353535, border #383838,
text #f1f1f1, text2 #aaaaaa, accent #ff0033 (playback progress, active nav
icon, live dot), action #c90028 (red buttons with text: Subscribe, Sign in,
primary actions; replaces #ff0000), overlay rgba(0,0,0,.72). Thumbnail
radius 11 dp, search pill radius 28 dp, min interactive target 44 dp.
Ambient glow: watch rgba(88,126,161,.28), music rgba(80,57,134,.20). Fonts:
keep the explicit Noto collection (Roboto in the reference is not a reason
to scan or add fonts).

### Shell
- Left rail 216 dp: small "LT" red mark + "LightTube" (no heavy wordmark,
  no "official" claims). "Your spaces": Watch, Shorts, Podcasts, Music (icon
  + label, 48 dp rows; active = raised bg and accent icon). Divider. Then the
  utilities: Library (Watch later, History, saved playlists, downloads later),
  Subscriptions (manage), You (accounts, settings).
- Below 1050 dp window width the rail collapses to the 72 dp icon guide
  (same items). Report the real minimum width in the summary.
- Top bar: "<Space> space" label (hidden when narrow), the pill search with
  placeholder "Search <space>", library button, profile circle.
- Heading per space: title, subtitle, a status badge, "Ambient on/off"
  badge-button. Then filter chips (raised; active = text bg, bg text).

### Spaces (real data only; demo data only with --demo)
- Watch: chips For you (account Home feed), Following (subscriptions feed),
  Continue (videos with a stored position not finished), Saved (Watch later
  + saved playlists). Hero on For you without a query: the latest unfinished
  video (paused, real thumbnail, "Continue watching", resume time), else the
  first feed item. Then the 3-column landscape grid (2 at medium, 1 narrow).
- Shorts: chips Following (merge the /shorts tab of followed channels, cap
  15 channels, newest first, cache 1 h), Saved, Explore (explicit
  unsupported state: "YouTube does not offer a Shorts feed to this app").
  Stage: one 9:16 portrait (max 320x565 dp) with previous/next buttons,
  starts muted with captions on, plays in the visible player inside the
  portrait. Side panel "More to explore" and "Open full video in Watch"
  (switches to Watch, selects the video, does not autoplay). Classify as a
  Short only by source (Shorts tab, /shorts/ URL, Shorts search filter).
- Podcasts: chips Your shows (account feed/podcasts), New episodes (latest
  episode of each show), Continue (episodes with a stored position). Square
  show art grid (4 columns, 2 narrow), then ordered episode rows (68x50 art,
  title, show and episode number, resume bar, "N min left" or "Played").
  Its own queue = the show order.
- Music: chips Liked (list=LM), Albums (from the last Music search),
  Saved. Feature block (album art 196 dp, title, artist, Open/Save), numbered
  track rows (art 44 dp, title, artist, collection, duration), a dock with
  the current track (visible player). Its own queue = the track order.
  Note in README: yt-dlp marks YouTube Music as not directly supported.
- Search is scoped: Watch = video search; Shorts = Shorts filter search
  (slow; show "Searching Shorts…"); Podcasts = filter the followed shows and
  their episodes, plus YouTube search for "<q> podcast" playlists; Music =
  music.youtube.com search. The typeahead adds a last row "Search all of
  YouTube for '<q>'" that switches to Watch results.

### State and playback rules
- Each space keeps its own chip, query, scroll, selection, queue and resume
  item; switching restores them.
- One player. Switching spaces pauses the visible player and keeps its
  position; nothing starts on switch or on return. No hidden or background
  audio: Podcasts and Music play in the visible player/dock.
- Stored positions (new store file positions.json: id -> seconds, duration,
  updated) feed Continue, resume bars and "Played" (>= 95 %).
- Keys: Ctrl+1..5 keep their targets (Home -> Watch/For you, Subscriptions ->
  Watch/Following, Watch later -> Library, History -> Library, You). New
  Alt+1..4 select Watch, Shorts, Podcasts, Music. Player keys unchanged.

### Ambient
A static glow behind the hero (Watch) and the feature block (Music) only,
drawn as a blurred-looking shape from the fixed tokens (layered rounded
rects with falling alpha); no image sampling, no animation, no timer, no
extra redraws. Setting "Ambient" (default on) in Settings and the heading
badge; persisted.

### Demo state for visual acceptance
`--demo` loads testdata/demo (the 12 stock JPEGs from the design hand-off
with attribution in testdata/demo/ASSETS.md, fictional titles/creators from
the reference, labelled "Demo"); nothing personal, no network. Live mode
never shows demo records. vinyl.jpg shows archival photographs.

### Acceptance details from the design review (2026-10-19, Version 6)
- Podcasts and Music keep the real video visible while playing; never
  replace an active video with artwork. The compact player/dock must be a
  usable player (controls below the video) or playback pauses.
- `--demo` is isolated and deterministic: no account or cookie loading, no
  yt-dlp, suggestions, feeds or network, no media launch (playback disabled
  with a visible "Playback disabled in demo" state, or a stub). It never
  reads or writes the user's live data; use a separate XDG environment.
- Shorts identity only from provenance: a canonical watch URL returned by a
  Shorts tab or the Shorts search filter is a Short. Empty, invalid or
  changed upstream responses give honest empty/error states. Tests use
  fixtures, never current signed-in results.
- "Open full video" only when a real source link exists (for example the
  Short's description or YouTube's related-video field); never by matching
  title or topic. Opening the same Short in Watch is labelled "Open this
  Short in Watch".
- Broad search is labelled "All YouTube", is visibly separate from the
  space's own results, and never labels global results as Podcast, Music or
  Short. Returning to a space restores its own query, chip and scroll.
- Account switch and sign-out cancel or invalidate pending per-space loads
  (generation counter); queues, positions and results never cross accounts
  (positions.json per account). Tests with delayed responses.
- Measure idle CPU and RSS with ambient on and off, on the demo dataset,
  against the pre-redesign build on the same dataset. Ambient shapes are
  fixed and cached; no per-frame blur or image work. Report any unmet target.
- Grid columns follow the width (keep card width usable); report the real
  minimum width. Existing shortcuts stay; Alt+1..4 select spaces.

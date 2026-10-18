# lightTube — specification

A small, fast YouTube client for one machine: MacBookPro12,1 (Intel Broadwell,
Iris 6100), Arch Linux + Omarchy + Hyprland (Wayland). The goal is low CPU,
low heat and low memory. Video always plays in mpv with VA-API hardware decode.

## Facts measured on the target machine (2026-10-18)

- The GPU decodes H.264 in hardware. It does NOT decode VP9 or AV1 in hardware.
- `mpv --hwdec=vaapi` plays 1080p30 H.264 from YouTube at about 20% of one core.
  The same video in Chrome used about 95% of one core.
- A Gio window built with `-tags novulkan,nox11` idles at 0.3% CPU, 68 MB RSS.
- Vulkan headers are not installed. Always build with `-tags novulkan,nox11`.
- Tools present: mpv 0.41, yt-dlp 2026.08.19, ffmpeg. Go 1.27 via mise.

## Architecture

```
cmd/lighttube/main.go      entry point, flags (--version, --debug)
internal/player/           mpv launcher, format policy, power source check
internal/yt/               search (yt-dlp), channel resolve, RSS feeds, parsing
internal/store/            JSON persistence: subscriptions, watch later, history
internal/thumbs/           thumbnail fetch, disk cache, decode, memory LRU
internal/ui/               Gio interface
testdata/                  fixtures: yt-dlp JSON, RSS XML, Takeout CSV
```

Dependencies: Go standard library, `gioui.org`, and the `golang.org/x/...`
modules that Gio pulls in. No other dependency without approval.
No cgo except what Gio needs. No SQLite.

Data locations (XDG):
- `~/.local/share/lighttube/` — `subscriptions.json`, `watchlater.json`, `history.json`
- `~/.cache/lighttube/` — `thumbs/<id>.jpg`, `feeds/<channel_id>.xml`

Write files atomically (write temp file, then rename).

## Player (internal/player)

Start one mpv process per play request. Do not embed mpv (Wayland does not
support window embedding).

mpv arguments:
- `--hwdec=vaapi --vo=gpu`
- `--ytdl-format=<policy>` (below)
- `--save-position-on-quit`
- `--force-window=immediate`
- `--title=<video title>`
- `--input-ipc-server=$XDG_RUNTIME_DIR/lighttube-mpv.sock`
- audio-only mode: add `--no-video --force-window=no` and use format `ba`.
- Set a fixed Wayland app id so Hyprland rules can match the window. Verify
  the option name with `mpv --list-options | grep -i app` before using it.

Format policy (the core feature). Never pick AV1. Prefer H.264.
- On AC power:
  `bv*[vcodec^=avc1][height<=1080][fps<=30]+ba/bv*[vcodec^=avc1][height<=1080]+ba/bv*[vcodec^=vp9][height<=720]+ba/b[height<=480]`
- On battery: the same chain with `height<=720` for H.264 and `height<=480` for VP9.
- Power source: read `/sys/class/power_supply/*/online` for `type=Mains`.
  If no mains supply is found, treat it as AC.
- The policy is a pure function `Format(power Power, mode Mode) string` with tests.

Only one video plays at a time: before a new play, send `quit` through the IPC
socket to the old mpv, or kill it if the socket does not answer within 500 ms.

## Sources (internal/yt)

No API key. No sign-in.

- Search: `yt-dlp --flat-playlist -J --no-warnings "ytsearch20:<query>"`.
  Parse `entries[]`: id, title, channel, channel_id, duration, view_count,
  upload date if present. Time limit 15 s; cancel with context.
- Channel resolve: accept a channel URL, `@handle`, or a raw `UC…` id.
  Resolve to `channel_id` with
  `yt-dlp --flat-playlist --playlist-items 1 --print channel_id <url>`.
- Feed: `https://www.youtube.com/feeds/videos.xml?channel_id=<id>`
  (Atom XML, about 15 newest videos). Parse with `encoding/xml`.
  Fetch all channels in parallel, at most 8 at once, 10 s per request.
  Cache each feed on disk; reuse it if it is younger than 15 minutes.
- Merged feed: all subscription entries, newest first, without duplicates.
  Hide YouTube Shorts when the feed entry link contains `/shorts/`.
- Takeout import: Google Takeout `subscriptions.csv`
  (columns: Channel Id, Channel Url, Channel Title).
- Paste URL: accept `youtube.com/watch?v=`, `youtu.be/`, `/shorts/`, `/live/`.

Thumbnails: `https://i.ytimg.com/vi/<id>/mqdefault.jpg` (320x180).

## Interface (internal/ui, Gio)

One window, app id `lighttube`. Dark theme. Layout:

```
[ Search | Subscriptions | Watch later | History ]          [policy: H.264 ≤1080p · AC]
[ search box ........................................ ]  [Paste URL]
┌──────────┬────────────────────────────────────────────┐
│ thumb    │ Title                                       │
│ 160x90   │ Channel · 12:34 · 3 days ago                │
└──────────┴────────────────────────────────────────────┘
  ... scrollable list ...
[ status line: errors, "Loading…", counts ]
```

Subscriptions tab adds: a field to add a channel, an "Import Takeout CSV"
button (file path text field is fine), and a list of channels with remove.

Keys:
- `Ctrl+L` focus the search box; `Enter` in the box runs the search.
- `Up`/`Down` move the selection; `Enter` plays; `A` plays audio only.
- `W` toggles watch later; `Ctrl+1..4` switch tabs; `Ctrl+V` plays a pasted URL.
- Mouse: click a row to select, double-click to play.

Playing a video adds it to history (newest first, keep 500).

## Performance rules

- Draw frames only on events. No animation loop, no timer that redraws.
- All network, disk and process work runs off the UI goroutine. Results come
  back through a channel and one `Invalidate()`.
- Decode thumbnails off the UI goroutine. Keep at most 100 decoded images in an
  LRU. Load thumbnails only for visible rows (plus a margin of 10).
- No polling. The power source is read at play time and when the window gains focus.
- Build: `go build -tags novulkan,nox11 -trimpath -ldflags='-s -w'`.

## Targets (checked at the end of each phase)

| Measure                          | Target            |
|----------------------------------|-------------------|
| Idle CPU, window open            | ≤ 1% of one core  |
| Window memory (RSS)              | ≤ 90 MB           |
| Startup to first frame           | ≤ 500 ms          |
| Search results shown             | ≤ 3 s             |
| mpv playback, 1080p30            | ≤ 25% of one core |

## Phases

1. Player core: `internal/player` + a CLI path `lighttube play <url> [--audio]`.
   Tests for the format policy and the power source parser.
2. Sources and store: `internal/yt`, `internal/store`, `internal/thumbs`, with
   fixture-based tests. A CLI path `lighttube search <query>` prints results.
3. Interface: `internal/ui`, all tabs and keys above.
4. Integration: `Makefile` (build, test, install to `~/.local/bin`), a desktop
   entry, a Hyprland window rule and keybinding snippet in `contrib/`, README.

Out of scope: comments, sign-in, recommendations, live chat, downloads, SponsorBlock.

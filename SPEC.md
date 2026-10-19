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

5. In-place player (see "Embedded player" below) and the watch page in DESIGN.md.
6. Account through Chrome's login (see "Account" below).

7. Portability (see "Portability" below).

Phase 4 runs after phases 5 and 6. Phase 7 runs after phase 4.
Decision (2026-10-18): stay a native app. No hosted website: it would decode
in the browser (the CPU problem this app solves), proxy video through a
server, need the user's cookies on that server, and break YouTube's terms.
A later optional `lighttube serve` (LAN remote control, playback stays in
mpv on the laptop) is allowed.

Out of scope: comments, live chat, downloads, SponsorBlock, OAuth sign-in.

## Embedded player (internal/embed, phase 5)

Video plays inside the lightTube window. The goal is the same CPU cost as
external mpv (about 20% of one core at 1080p30), so hardware decode and a
zero-copy path are required.

Method: libmpv render API (OpenGL) drawing into a Wayland **subsurface** of
the Gio window.

- Get `wl_display` and `wl_surface` from Gio's `app.WaylandViewEvent`.
- Create a private event queue (`wl_display_create_queue`) and a registry on
  it. Bind `wl_compositor` and `wl_subcompositor` there. Never dispatch Gio's
  default queue from another goroutine or thread.
- Create a child `wl_surface`, a `wl_subsurface` on the Gio surface,
  `wl_subsurface_set_desync`, and an empty input region
  (`wl_surface_set_input_region` with an empty `wl_region`). All pointer and
  key input then goes to Gio; Gio sends commands to mpv.
- Create a `wl_egl_window`, an EGL display/context/surface for it, and an
  `mpv_render_context` with `MPV_RENDER_API_TYPE_OPENGL`,
  `MPV_RENDER_PARAM_OPENGL_INIT_PARAMS` and **`MPV_RENDER_PARAM_WL_DISPLAY`**
  (required for VA-API interop). Render on a dedicated locked OS thread when
  the mpv update callback fires. Respect the buffer scale (2 on this machine):
  `wl_surface_set_buffer_scale` and an EGL window size in pixels.
- Each frame, the UI reports the video rectangle (in dp) to the embed package.
  The embed package calls `wl_subsurface_set_position` and resizes the EGL
  window only when the rectangle changes. When the watch page is not shown,
  unmap the subsurface (attach a NULL buffer) and pause.
- mpv options: `hwdec=vaapi`, `vo=libmpv`, `ytdl-format` from the policy,
  `osc=no`, `input-default-bindings=no`, `save-position-on-quit=yes`, the
  same cookie options as the account section.
- Create the mpv handle on the first play and destroy it (and the
  subsurface and EGL objects) when the watch page closes. Idle RSS must go
  back to the value before playback.
- Use cgo with `pkg-config: mpv wayland-client wayland-egl egl`. Do not use a
  Go mpv binding module.
- Forbidden: `--wid` (does not work on Wayland), the software render API
  (`MPV_RENDER_API_TYPE_SW`), `hwdec=vaapi-copy`, any frame readback into Go.
- Keep the external mpv path. `--external-player` flag and the "Open in mpv"
  action on the watch page use it.

Acceptance (measure, 1080p30 H.264, no input):
- mpv property `hwdec-current` is `vaapi` (log it with `--debug`).
- Whole process CPU ≤ 25% of one core while playing.
- After closing the watch page, RSS returns to within 10 MB of the value
  before playback, and idle CPU is ≤ 1%.

## Account (phase 6)

Sign-in uses the Google session that already exists in Google Chrome.
There is no OAuth and no API key.

- Settings page entry "Account": "Use Chrome's YouTube login" toggle, a browser
  field (default `chrome+gnomekeyring`), and the status (signed in / failed with reason).
- On sign-in, run once:
  `yt-dlp --cookies-from-browser chrome+gnomekeyring --cookies <data>/cookies.txt --skip-download ...`
  to write a cookie file. Create it with mode 0600. All later yt-dlp calls
  and mpv (`ytdl-raw-options=cookies=<path>`) use the file, so the keyring is
  not decrypted on every call. If a call fails with a login error, extract
  again once, then show the error.
- Never log or print cookie values.
- Measured 2026-10-18: yt-dlp does not detect Hyprland as a desktop and picks
  the BASICTEXT keyring, so all 912 Chrome cookies fail to decrypt. The
  `+gnomekeyring` suffix is required. With it, 30 items take 4–5 s per feed
  (history 11 s), including keyring decrypt. Some fields are `NA`
  (view_count on rec/subs, channel on history, duration on live items).
  Omit unknown parts in the UI.
- Feeds when signed in (yt-dlp `--flat-playlist -J`, first 60 items):
  Home = `:ytrec`, Subscriptions = `:ytsubs`, Watch later = `:ytwatchlater`,
  History = `:ythistory`. Cache each feed to disk; show the cache first,
  then refresh in the background. When signed out, keep the RSS feed and
  the local lists.
- Signed-in Home replaces the RSS Home; the local subscription list stays as
  the chips source and fallback.

## Portability (phase 7)

Target order: generic Linux (any desktop, X11 and Wayland, any GPU), then
Windows, then macOS.

- `internal/platform`, one file per OS (build tags): power source
  (Linux sysfs, macOS IOKit or `pmset -g batt`, Windows `GetSystemPowerStatus`),
  font files (Linux Noto paths, macOS PingFang + SF, Windows Microsoft YaHei +
  Segoe UI; fall back to Gio's system font scan when none is found),
  default cookie browser spec (Linux `chrome+gnomekeyring` or
  `chrome+kwallet` by desktop, macOS `chrome`, Windows `firefox` because
  Chrome's app-bound cookie encryption blocks yt-dlp there), and the
  modifier key (Cmd on macOS, Ctrl elsewhere).
- Paths: `os.UserConfigDir` / `os.UserCacheDir` instead of raw XDG variables.
- Decode: `hwdec=auto-safe`. Codec policy from what the GPU decodes
  (Linux: query VA-API profiles; others: allow VP9/AV1 when the hwdec
  reports them), with a manual setting. The Broadwell H.264-only chain stays
  as the "Old GPU" choice.
- Embedded player backends: Linux Wayland (subsurface, phase 5),
  Linux X11 and Windows (mpv `wid` with a child window from Gio's view
  event), macOS (NSView; verify before building). External mpv is the
  fallback on every OS.
- External player IPC: Unix socket on Linux and macOS, named pipe on Windows.
  Linux-only peer PID checks behind build tags.
- `lighttube doctor`: checks mpv/libmpv, yt-dlp version age, yt-dlp's
  JavaScript runtime, active hwdec, login state; prints install hints.
- Settings page: login browser spec, codec policy, in-place or external player.
- Build: drop `novulkan,nox11` as defaults (keep them as a local Makefile
  option for this machine). CI matrix builds per OS (cgo, no cross-compile).
  Packaging: Linux AppImage or tarball + `.desktop`; Windows zip with
  mpv-2.dll; macOS `.app` (signing only if shared).

### Phase 7 split and facts (2026-10-18)

Facts: this machine has X11 dev libs and Xwayland, but no Vulkan headers, so
`-tags novulkan` builds with X11 support. `GOOS=windows CGO_ENABLED=0` fails
today on `syscall.Kill` and `syscall.GetsockoptUcred` in `internal/player`
and on `internal/embed` (no Go files for that OS). `GOOS=darwin` vet fails on
the same Ucred call.

- 7a: `internal/platform` (power, fonts, default cookie browser, modifier,
  data and cache dirs), build-tagged player IPC (Unix socket / named pipe,
  process kill, peer PID check), `internal/embed` stub for unsupported OS
  (the UI then uses the external player). Linux data dir stays
  `$XDG_DATA_HOME/lighttube` (default `~/.local/share/lighttube`) so no
  existing data moves; macOS and Windows use `os.UserConfigDir()/lighttube`.
  Cache: `os.UserCacheDir()/lighttube`. Makefile detects tags: add
  `novulkan` when `vulkan/vulkan.h` is missing, `nox11` when pkg-config has
  no `x11-xcb xkbcommon-x11 xcursor`; `TAGS=...` overrides.
  Acceptance: `GOOS=windows CGO_ENABLED=0 go build ./cmd/lighttube` and
  `GOOS=darwin CGO_ENABLED=0 go vet` of the non-GUI packages pass.
- 7b: `lighttube doctor`, `hwdec=auto-safe` with a codec policy setting
  (auto from VA-API profiles through libva when available, "Old GPU"
  H.264 ≤1080p, "Any"), Settings page (login browser, codec policy,
  in-place or external player), settings stored in the data dir.
- 7c: X11 embed (mpv `wid` child window), Windows embed (same API, untested
  here), CI matrix, packaging (tarball + .desktop, Windows zip, macOS .app).

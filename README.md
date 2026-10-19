# lightTube

lightTube is a fast, lightweight YouTube desktop client for Linux (Wayland). It is built for low CPU, memory, and heat usage on resource-constrained hardware. Videos play with hardware-accelerated VA-API decoding, either inside the window (libmpv in a Wayland subsurface) or in an external mpv window.

## Requirements

- Go 1.27 managed via mise (`mise exec -- go ...`)
- Gio build dependencies (Wayland, OpenGL/EGL, fontconfig)
- `mpv` with `libmpv.so.2`
- `yt-dlp` (for YouTube search and metadata extraction)
- Wayland and EGL development libraries (`wayland-client`, `wayland-egl`, `egl`)
- `python-secretstorage` (for extracting YouTube cookies from Google Chrome on Linux using GNOME Keyring)

## Build and Install

Build the application:
```sh
make
```

Run checks (`gofmt`, `go vet`, and tests):
```sh
make check
```

Install binary, desktop file, and icon (defaults to `PREFIX ?= $(HOME)/.local`):
```sh
make install
```

To install to a different prefix:
```sh
make install PREFIX=/usr/local
```

Uninstall:
```sh
make uninstall
```

Clean build artifacts:
```sh
make clean
```

## Hyprland Integration

For Omarchy Hyprland users (using Lua configuration), `contrib/hyprland.lua` provides keybindings and window rules:

```lua
-- Unbind Omarchy default YouTube web app shortcut.
hl.unbind("SUPER + SHIFT + Y")

-- Bind Super+Shift+Y to focus or launch lightTube.
o.bind("SUPER + SHIFT + Y", "lightTube", o.launch_sole("lighttube", "lighttube"))

-- Window rule for external mpv player window (app id: lighttube-mpv).
o.window("^lighttube-mpv$", { float = true, size = { 1280, 720 }, center = true })
```

To load this configuration:
1. Copy or symlink `contrib/hyprland.lua` to `~/.config/hypr/lighttube.lua` and add `require("lighttube")` in `~/.config/hypr/hyprland.lua`, or
2. Paste the snippet directly into `~/.config/hypr/bindings.lua`.

## Usage

### GUI Shortcuts

| Key | Action |
| --- | --- |
| `Ctrl+1..5` | Switch pages (Home, Subscriptions, Watch later, History, You/Account) |
| `Ctrl+L` | Focus the search box |
| `Ctrl+V` | Paste and play a YouTube URL |
| `Enter` (in search) | Execute search (or play if input is a video URL) |
| `Up` / `Down` / Arrows | Navigate cards or search list |
| `Enter` | Play selected video |
| `A` | Play selected video (audio only) |
| `W` | Toggle Watch later for selected video |
| `Space` / `K` | Play / pause toggle |
| `J` / `L` | Seek backward / forward 10 seconds |
| `Left` / `Right` | Seek backward / forward 5 seconds |
| `M` | Mute / unmute audio |
| `F` | Toggle fullscreen |
| `Esc` | Leave fullscreen, clear search, or exit watch page |
| `O` | Open current video in external mpv window |
| Click | Play video from card or search result |
| Double Click (video) | Toggle fullscreen |

### CLI Subcommands and Flags

- `lighttube [--external-player] [--debug]`:
  Launch the graphical interface. `--external-player` opens video playback in an external mpv window rather than using the embedded player. `--debug` enables verbose logging.
- `lighttube play <url> [--audio]`:
  Play a video directly from URL. `--audio` plays audio only with format `ba`.
- `lighttube search <query>`:
  Search YouTube videos and output results (`ID`, `Title`, `Channel`, `Duration`).
- `lighttube feed [--account home|subs|later|history]`:
  Without `--account`, output the merged feed of local subscriptions. With `--account`, output the signed-in YouTube feed (`home`, `subs`, `later`, or `history`).
- `lighttube account <signin|signout|status>`:
  - `signin`: Extract YouTube cookies from Chrome session.
  - `signout`: Delete saved cookies.
  - `status`: Show authentication status (`signed in` or `signed out`).
- `lighttube sub <add|list>`:
  - `lighttube sub add <url|@handle|UC id>`: Resolve channel and add to local subscriptions.
  - `lighttube sub list`: Print local subscriptions (`ChannelID`, `Title`, `URL`).
- `lighttube --version`:
  Print lightTube version.

## Account and Data Storage

Authentication uses an existing Google Chrome login session on the machine. There is no OAuth sign-in and no API key.
- Sign-in extracts session cookies using `yt-dlp --cookies-from-browser chrome+gnomekeyring`.
- Storage locations:
  - `~/.local/share/lighttube/cookies.txt`: Netscape cookie file, written with file mode `0600` (readable only by the owner). Cookie values are never logged or printed.
  - `~/.local/share/lighttube/subscriptions.json`: Local channel subscriptions.
  - `~/.local/share/lighttube/watchlater.json`: Saved watch later items.
  - `~/.local/share/lighttube/history.json`: Video playback history (up to 500 items).
- Cache locations:
  - `~/.cache/lighttube/thumbs/<id>.jpg`: Cached video thumbnails.
  - `~/.cache/lighttube/feeds/<channel_id>.xml`: Cached RSS feeds (15-minute TTL).
  - `~/.cache/lighttube/account/<kind>.json`: Cached account feeds.

## Performance Facts

Measured on a MacBookPro12,1 (Intel Broadwell, Iris 6100) in October 2026:
- The GPU decodes H.264 only, so lightTube asks for H.264 up to 1080p.
- 1080p30 playback in the window: about 15% of one CPU core (`hwdec-current=vaapi`).
  External mpv: about 20%. The same video in Google Chrome: about 95%.
- Startup to first frame: about 200 to 250 ms.
- Idle CPU with the window open: about 0%. The window draws only on events.
- Memory (RSS): about 115 MB idle. This is above the 90 MB target in SPEC.md.
- At most 100 decoded thumbnails are kept in memory.

## Known Limits

- Linux Wayland only: Current builds require Wayland subsurfaces and Wayland window classes (multi-platform support for X11, macOS, and Windows is scheduled for Phase 7).
- No emoji font: Renders Latin and CJK characters using system Noto fonts, but emoji glyphs are not displayed.

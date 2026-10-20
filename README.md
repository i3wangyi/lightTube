# lightTube

lightTube is a YouTube desktop client. It has four spaces: Watch, Shorts, Podcasts, and Music. It supports accounts, playlists, search typeahead, and in-window video playback on Linux. Playback can also use an external mpv window.

## Runtime requirements

- Linux: Wayland or X11, `mpv` with `libmpv.so.2`, and `yt-dlp`.
- macOS: `libmpv` and `yt-dlp` from Homebrew. The app uses an external mpv window.
- Windows: `yt-dlp.exe` on `PATH`. Add `mpv-2.dll` beside `lighttube.exe` for in-window playback.
- Account sign-in reads a browser session through yt-dlp. Cookies stay on the local device. yt-dlp reads the saved cookie file for account feeds and playback.

## Install

### Linux tarball

Download and extract the Linux x86_64 archive from [Releases](https://github.com/i3wangyi/lightTube/releases). Run:

```sh
./install.sh
```

The installer uses `~/.local` by default. Set `PREFIX` to choose another prefix. Run `PREFIX="$HOME/.local" ./install.sh --uninstall` to remove the installed files.

### Arch Linux

Download `packaging/arch/PKGBUILD` and `packaging/arch/.SRCINFO` from the source tree. From that directory, run `makepkg -si`.

### Build from source

Install Go 1.27 through mise and the Gio, Wayland, EGL, X11, fontconfig, and mpv development libraries. Then run:

```sh
make build
make install
```

`make install` installs under `~/.local` by default. Use `make install PREFIX=/usr/local` to select another prefix. Run `make check` to run formatting, vet, and Go tests.

### macOS app

Download and unzip the macOS arm64 app archive from [Releases](https://github.com/i3wangyi/lightTube/releases). Install the runtime libraries:

```sh
brew install mpv yt-dlp
```

The app is unsigned. On first open, Control-click `lightTube.app`, select **Open**, then confirm **Open** in the dialog. macOS can block a direct double-click because the app is unsigned. Playback opens in external mpv.

### Windows zip

Download and unzip the Windows x86_64 archive from [Releases](https://github.com/i3wangyi/lightTube/releases). Install `yt-dlp.exe` and add it to `PATH`. For in-window playback, place `mpv-2.dll` beside `lighttube.exe`. Install mpv for external playback.

## Features

- Four spaces: Watch, Shorts, Podcasts, and Music.
- Local account support with YouTube feeds, saved playlists, and watch history.
- Search suggestions while typing.
- In-window playback on supported Linux systems; external mpv playback on all platforms.
- Local channel subscriptions and Watch later list.

## Keyboard shortcuts

| Shortcut | Action |
| --- | --- |
| `Alt+1` to `Alt+4` | Switch between the four spaces |
| `Ctrl+1` to `Ctrl+5` | Switch tabs |
| `Ctrl+L` | Focus search |
| `Ctrl+V` | Paste and play a YouTube URL |
| `Enter` | Search or play the selected item |
| `Space` or `K` | Play or pause |
| `J` / `L` | Seek backward / forward 10 seconds |
| `Left` / `Right` | Seek backward / forward 5 seconds |
| `M` | Mute or unmute |
| `F` | Toggle fullscreen |
| `Esc` | Exit fullscreen or close the watch page |
| `O` | Open the current video in external mpv |

## Privacy and affiliation

lightTube is not affiliated with YouTube. Cookies stay on your device. yt-dlp reads them to access account feeds and play account-restricted content. lightTube does not print cookie values.

## Demo screenshots

These screenshots use demo data only. They do not show live YouTube content. Stock photo sources are listed in [internal/demo/ASSETS.md](internal/demo/ASSETS.md).

| Watch | Shorts |
| --- | --- |
| ![Watch space](docs/screenshots/watch.png) | ![Shorts space](docs/screenshots/shorts.png) |

| Podcasts | Music |
| --- | --- |
| ![Podcasts space](docs/screenshots/podcasts.png) | ![Music space](docs/screenshots/music.png) |

## License

lightTube is licensed under the GNU General Public License, version 3 or later. See [LICENSE](LICENSE).

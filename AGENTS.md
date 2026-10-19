# Rules for coding agents

- Read `SPEC.md` first. It is the source of truth. Do only the phase you are asked to do.
- Run Go through mise: `mise exec -- go ...`. Go 1.27.
- Build with `make build` / the tags Makefile prints (detected per SPEC: `novulkan` when Vulkan headers are missing, `nox11` when X11 libraries are missing; on this machine: `-tags novulkan`).
- Do not add dependencies beyond the standard library, `gioui.org`, and the
  `golang.org/x/...` modules Gio pulls in, and `github.com/go-text/typesetting`
  (Gio's font library, used directly to parse one face of a font collection).
  Ask first if you think you need one.
- cgo is allowed only in `internal/embed`, and only against system libraries
  through pkg-config: `mpv`, `wayland-client`, `wayland-egl`, `egl`. Do not use
  a Go mpv binding module.
- GUI screenshots: first check the display is on
  (`hyprctl monitors -j | grep dpms` shows `true`). Always run `timeout 10 grim ...`.
  If the display is off, skip screenshots and say so. Close every window and
  process you start.
- Never send simulated key or mouse input to the screen (wtype, ydotool,
  hyprctl sendshortcut): the user works on this display and the input can
  reach their windows. Test input handling headless with Gio's
  `input.Router` (see `internal/ui/keys_test.go`).
- Never log or print cookie values.
- Do not edit files outside this directory. Do not install system packages.
- Do not commit. The reviewer commits after checking your work.
- Before you finish: run `make check` (or `gofmt -l .` [no output], `mise exec -- go vet <tags> ./...`,
  and `mise exec -- go test <tags> ./...`). All must pass.
- Keep code plain and small. Comments explain why, not what.
- End with a short summary: files changed, commands run and their results,
  anything not done and why.

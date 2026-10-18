# Rules for coding agents

- Read `SPEC.md` first. It is the source of truth. Do only the phase you are asked to do.
- Run Go through mise: `mise exec -- go ...`. Go 1.27.
- Always build with `-tags novulkan,nox11`. Vulkan headers are not installed.
- Do not add dependencies beyond the standard library, `gioui.org`, and the
  `golang.org/x/...` modules Gio pulls in. Ask first if you think you need one.
- cgo is allowed only in `internal/embed`, and only against system libraries
  through pkg-config: `mpv`, `wayland-client`, `wayland-egl`, `egl`. Do not use
  a Go mpv binding module.
- GUI screenshots: first check the display is on
  (`hyprctl monitors -j | grep dpms` shows `true`). Always run `timeout 10 grim ...`.
  If the display is off, skip screenshots and say so. Close every window and
  process you start.
- Never log or print cookie values.
- Do not edit files outside this directory. Do not install system packages.
- Do not commit. The reviewer commits after checking your work.
- Before you finish: run `gofmt -l .` (no output), `mise exec -- go vet -tags novulkan,nox11 ./...`,
  and `mise exec -- go test -tags novulkan,nox11 ./...`. All must pass.
- Keep code plain and small. Comments explain why, not what.
- End with a short summary: files changed, commands run and their results,
  anything not done and why.

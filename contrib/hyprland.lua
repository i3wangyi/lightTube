-- Omarchy Hyprland configuration snippet for lightTube.
--
-- How to load this file:
-- 1. Copy or symlink into ~/.config/hypr/ (for example ~/.config/hypr/lighttube.lua)
--    and add `require("lighttube")` to ~/.config/hypr/hyprland.lua, or
-- 2. Paste the snippet below into ~/.config/hypr/bindings.lua.

-- Unbind Omarchy default YouTube web app shortcut.
hl.unbind("SUPER + SHIFT + Y")

-- Bind Super+Shift+Y to focus or launch lightTube.
o.bind("SUPER + SHIFT + Y", "lightTube", o.launch_sole("lighttube", "lighttube"))

-- Window rule for external mpv player window (app id: lighttube-mpv).
o.window("^lighttube-mpv$", { float = true, size = { 1280, 720 }, center = true })

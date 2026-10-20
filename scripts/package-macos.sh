#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
set -eu

version="${1:-${GITHUB_REF_NAME:-}}"
if [ -z "$version" ]; then
	version="$(git describe --tags --always 2>/dev/null || printf '0.1.0')"
fi
version="${version#v}"
name="lightTube-${version}-macos-arm64"
app="dist/lightTube.app"
stage="dist/${name}"
iconset="dist/lightTube.iconset"

command -v rsvg-convert >/dev/null || { echo 'rsvg-convert is required' >&2; exit 1; }
command -v iconutil >/dev/null || { echo 'iconutil is required' >&2; exit 1; }
test -x lighttube || { echo 'Build lighttube before packaging' >&2; exit 1; }
rm -rf "$app" "$stage" "$iconset" "dist/${name}.zip"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$iconset" "$stage"
cp lighttube "$app/Contents/MacOS/lighttube"
cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>lighttube</string>
<key>CFBundleIdentifier</key><string>io.github.i3wangyi.lighttube</string>
<key>CFBundleName</key><string>lightTube</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>${version}</string>
<key>CFBundleVersion</key><string>${version}</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>CFBundleIconFile</key><string>lighttube</string>
</dict></plist>
PLIST
for size in 16 32 128 256 512; do
	rsvg-convert -w "$size" -h "$size" contrib/lighttube.svg -o "$iconset/icon_${size}x${size}.png"
	 doubled=$((size * 2))
	rsvg-convert -w "$doubled" -h "$doubled" contrib/lighttube.svg -o "$iconset/icon_${size}x${size}@2x.png"
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/lighttube.icns"
cp LICENSE "$stage/"
mv "$app" "$stage/"
(cd dist && ditto -c -k --sequesterRsrc --keepParent "$name" "${name}.zip")
rm -rf "$stage" "$iconset"
echo "Created dist/${name}.zip"

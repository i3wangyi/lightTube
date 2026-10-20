# SPDX-License-Identifier: GPL-3.0-or-later
# Package lightTube for Windows as a zip archive containing lighttube.exe, README.md, and MPV_README.txt.
$ErrorActionPreference = "Stop"

$version = "0.1.0"
try {
    $gitVer = (git describe --tags --always --dirty 2>$null)
    if ($gitVer) { $version = $gitVer }
} catch {
    $version = "0.1.0"
}

$distDir = "dist"
$pkgName = "lighttube-$version-windows-x86_64"
$stageDir = Join-Path $distDir $pkgName

if (Test-Path $stageDir) {
    Remove-Item -Recurse -Force $stageDir
}
New-Item -ItemType Directory -Path $stageDir -Force | Out-Null

Write-Host "Building lighttube.exe for $pkgName..."
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-s -w -X main.version=$version" -o (Join-Path $stageDir "lighttube.exe") ./cmd/lighttube

Copy-Item "README.md" -Destination $stageDir
Copy-Item "LICENSE" -Destination $stageDir

$mpvNote = @"
lightTube for Windows
=====================

Embedded In-Window Playback:
To enable in-window video playback, download `mpv-2.dll` from an mpv Windows release
(for example, from https://sourceforge.net/projects/mpv-player-windows/files/libmpv/
or https://github.com/shinchiro/mpv-winbuild-cmake/releases) and place `mpv-2.dll`
in the same directory as `lighttube.exe`.

External Player Playback:
Alternatively, install mpv and yt-dlp on your Windows system PATH to use the external player.
"@

Set-Content -Path (Join-Path $stageDir "MPV_README.txt") -Value $mpvNote

$zipFile = Join-Path $distDir "$pkgName.zip"
if (Test-Path $zipFile) {
    Remove-Item -Force $zipFile
}

Compress-Archive -Path "$stageDir\*" -DestinationPath $zipFile
Remove-Item -Recurse -Force $stageDir

Write-Host "Successfully packaged $zipFile"

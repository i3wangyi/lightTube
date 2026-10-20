#!/bin/sh
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
PREFIX="${PREFIX:-$HOME/.local}"
DESTDIR="${DESTDIR:-}"

BINDIR="${DESTDIR}${PREFIX}/bin"
APPDIR="${DESTDIR}${PREFIX}/share/applications"
ICONDIR="${DESTDIR}${PREFIX}/share/icons/hicolor/scalable/apps"

if [ "${1:-}" = "--uninstall" ]; then
	rm -f "$BINDIR/lighttube" "$APPDIR/lighttube.desktop" "$ICONDIR/lighttube.svg"
	echo "Removed: $BINDIR/lighttube"
	echo "Removed: $APPDIR/lighttube.desktop"
	echo "Removed: $ICONDIR/lighttube.svg"
	exit 0
fi

mkdir -p "$BINDIR" "$APPDIR" "$ICONDIR"

install -Dm755 "$DIR/lighttube" "$BINDIR/lighttube"
install -Dm644 "$DIR/contrib/lighttube.desktop" "$APPDIR/lighttube.desktop"
install -Dm644 "$DIR/contrib/lighttube.svg" "$ICONDIR/lighttube.svg"

if [ -z "$DESTDIR" ] && command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database "$APPDIR" >/dev/null 2>&1 || true
fi

echo "Installed: $BINDIR/lighttube"
echo "Installed: $APPDIR/lighttube.desktop"
echo "Installed: $ICONDIR/lighttube.svg"

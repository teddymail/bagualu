#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SOURCE="$ROOT/web/admin/dist"
TARGET="$ROOT/internal/transport/http/static"

[ -f "$SOURCE/index.html" ] || {
	printf 'missing frontend build: %s\n' "$SOURCE/index.html" >&2
	exit 1
}

rm -rf "$TARGET"
mkdir -p "$TARGET"
cp -a "$SOURCE"/. "$TARGET"/

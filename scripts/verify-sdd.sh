#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

unformatted=$(gofmt -l cmd internal)
test -z "$unformatted" || { printf 'Go files need formatting:\n%s\n' "$unformatted"; exit 1; }

oversized=$(find cmd internal web/admin/src packaging/openwrt -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' -o -name '*.lua' -o -name '*.htm' -o -name '*.sh' \) -print | while IFS= read -r file; do lines=$(wc -l < "$file"); [ "$lines" -le 1000 ] || printf '%s %s\n' "$lines" "$file"; done)
test -z "$oversized" || { printf 'Files exceed 1000 lines:\n%s\n' "$oversized"; exit 1; }

go vet ./...
go test ./...
sh -n packaging/openwrt/etc/init.d/bagualu

if command -v luac >/dev/null 2>&1; then
  luac -p packaging/openwrt/usr/lib/lua/luci/controller/bagualu.lua
  luac -p packaging/openwrt/usr/share/luci/model/cbi/bagualu/config.lua
fi

cd web/admin
npx tsc --noEmit
npm run build
cd "$ROOT"
./scripts/sync-admin-static.sh

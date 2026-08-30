#!/bin/sh
set -eu

if [ "$#" -gt 0 ]; then
    exec /usr/local/bin/bagualu "$@"
fi

exec /usr/local/bin/bagualu \
    -listen "${BAGUALU_LISTEN:-0.0.0.0:18787}" \
    -db "${BAGUALU_DB:-/var/lib/bagualu/bagualu.db}" \
    -mihomo-binary "${BAGUALU_MIHOMO_BINARY:-/usr/local/bin/mihomo}" \
    -mihomo-control "${BAGUALU_MIHOMO_CONTROL:-http://127.0.0.1:19090}" \
    -mihomo-proxy-port "${BAGUALU_MIHOMO_PROXY_PORT:-17890}" \
    -mihomo-token "${BAGUALU_MIHOMO_TOKEN:-}" \
    -admin-password "${BAGUALU_ADMIN_PASSWORD:-admin}"

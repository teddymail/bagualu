FROM node:20-alpine AS web
WORKDIR /src/web/admin
COPY web/admin/package*.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=web /src/web/admin/dist ./web/admin/dist
RUN ./scripts/sync-admin-static.sh
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" go build -trimpath -ldflags="-s -w" -o /out/bagualu ./cmd/bagualu

FROM alpine:3.20 AS mihomo
ARG TARGETARCH=amd64
ARG MIHOMO_VERSION=latest
RUN apk add --no-cache ca-certificates curl gzip jq
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) arch=amd64 ;; \
      arm64) arch=arm64 ;; \
      386) arch=386 ;; \
      *) echo "unsupported Docker architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    if [ "${MIHOMO_VERSION}" = "latest" ]; then \
      release_url=https://api.github.com/repos/MetaCubeX/mihomo/releases/latest; \
    else \
      release_url="https://api.github.com/repos/MetaCubeX/mihomo/releases/tags/${MIHOMO_VERSION}"; \
    fi; \
    asset="$(curl -fsSL "${release_url}" | jq -r --arg arch "${arch}" '[.assets[] | select(.browser_download_url != null) | select((.name | ascii_downcase | contains("linux"))) | select((.name | ascii_downcase | contains($arch))) | select((.name | ascii_downcase | test("\\.(deb|rpm|pkg|zip|tar\\.gz|sha256|sha256sum|sig|dgst)$") | not))] | sort_by((.name | ascii_downcase | contains("compatible")), (.name | ascii_downcase | contains("-v1"))) | reverse | .[0].browser_download_url')"; \
    test -n "${asset}" && test "${asset}" != "null"; \
    curl -fsSL "${asset}" | gzip -dc > /mihomo; \
    test -s /mihomo; \
    chmod 0755 /mihomo

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl iputils tzdata \
    && addgroup -S bagualu \
    && adduser -S -G bagualu -h /var/lib/bagualu bagualu \
    && mkdir -p /var/lib/bagualu \
    && chown -R bagualu:bagualu /var/lib/bagualu
COPY --from=build /out/bagualu /usr/local/bin/bagualu
COPY --from=mihomo /mihomo /usr/local/bin/mihomo
COPY docker/entrypoint.sh /usr/local/bin/bagualu-entrypoint
RUN chmod 0755 /usr/local/bin/bagualu-entrypoint /usr/local/bin/bagualu /usr/local/bin/mihomo

ENV BAGUALU_LISTEN=0.0.0.0:18787 \
    BAGUALU_DB=/var/lib/bagualu/bagualu.db \
    BAGUALU_MIHOMO_BINARY=/usr/local/bin/mihomo \
    BAGUALU_MIHOMO_CONTROL=http://127.0.0.1:19090 \
    BAGUALU_MIHOMO_PROXY_PORT=17890
VOLUME ["/var/lib/bagualu"]
EXPOSE 18787
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD curl -fsS http://127.0.0.1:18787/api/v1/health || exit 1
USER bagualu
ENTRYPOINT ["/usr/local/bin/bagualu-entrypoint"]

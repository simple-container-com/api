# Caddy 2.11.2: clears Go-stdlib CVEs in 2.8.4's binary + CVE-2026-27586.
# Bumping requires editing all three "2.11.2" sites below (two FROMs + xcaddy).
# Refresh: docker buildx imagetools inspect caddy:X.Y.Z[-builder]

FROM caddy:2.11.2-builder@sha256:10ed0251c5cd1dbb4db0b71ad43121147961a51adfec35febce2c93ea25c24f4 AS builder

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache,sharing=locked \
    xcaddy build "v2.11.2" \
        --with github.com/grafana/certmagic-gcs@v0.1.7 \
    && caddy version

FROM caddy:2.11.2@sha256:25cdc846626b62d05f6b633b9b40c2c9f6ef89b515dc76133cefd920f7dbe562

RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/*

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/caddy" \
      org.opencontainers.image.description="Caddy with grafana/certmagic-gcs"

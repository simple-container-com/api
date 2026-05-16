# Caddy 2.11.3: closes vendored-dep CVEs in 2.11.2's binary (go-jose v4,
# otel, smallstep/certificates) plus Caddy core fastcgi + admin-socket
# auth-bypass fixes — see https://github.com/caddyserver/caddy/releases/tag/v2.11.3.
# Bumping requires editing all three "2.11.x" sites below (two FROMs + xcaddy).
# Refresh: docker buildx imagetools inspect caddy:X.Y.Z[-builder]

FROM caddy:2.11.3-builder@sha256:14f5b3ecb208d45a37bc26435a8c0c29181de98115358b4b863c6ec5801116a5 AS builder

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache,sharing=locked \
    xcaddy build "v2.11.3" \
        --with github.com/grafana/certmagic-gcs@v0.1.7 \
    && caddy version

FROM caddy:2.11.3@sha256:3739ea4f0c877259a693d932693cf8f3408e9a9497c004f031b0e830e93e1546

RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/*

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/caddy" \
      org.opencontainers.image.description="Caddy with grafana/certmagic-gcs"

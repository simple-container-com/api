# Refresh: docker buildx imagetools inspect alpine/kubectl:latest
FROM alpine/kubectl:latest@sha256:5d380d18d2509483aef3df54d676c767d798d55ec9f3e02dabfa4c88fe6559bd

# apk upgrade pulls post-tag distro fixes (e.g. nghttp2 CVE-2026-27135 at scan time).
RUN apk update \
    && apk upgrade --no-cache \
    && apk add --no-cache bash curl \
    && rm -rf /var/cache/apk/*

RUN addgroup -S sc && adduser -S -G sc -u 10001 sc
USER 10001:10001

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/kubectl"

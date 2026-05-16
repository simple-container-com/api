# Refresh: docker buildx imagetools inspect alpine/kubectl:latest
FROM alpine/kubectl:latest@sha256:405e7130cb2cd07f85feafd11333f2f46e921f2f8937c375e396791f4776c948

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

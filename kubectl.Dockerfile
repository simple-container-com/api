# Pin by digest (CIS Docker 4.7 — no floating tags).
# alpine/kubectl:latest @ 2026-05-06 → resolved digest below.
# Refresh via: docker buildx imagetools inspect alpine/kubectl:latest
FROM alpine/kubectl:latest@sha256:e9acf90f4aa6e1735a50758ee251d7bc622361ee23c35617dc0dcbe7c50282b0

# apk upgrade clears any base CVEs surfaced after the image was tagged
# (e.g. nghttp2-libs CVE-2026-27135 was outstanding at scan time).
RUN apk update \
    && apk upgrade --no-cache \
    && apk add --no-cache bash curl \
    && rm -rf /var/cache/apk/*

# CIS Docker 4.1 — drop privileges. kubectl needs no root capabilities.
RUN addgroup -S sc && adduser -S -G sc -u 10001 sc
USER 10001:10001

# CIS Docker 4.6 — declare a healthcheck so orchestrators can detect rot.
HEALTHCHECK --interval=30s --timeout=5s --start-period=2s --retries=3 \
    CMD kubectl version --client=true >/dev/null 2>&1 || exit 1

# Refresh: docker buildx imagetools inspect alpine/kubectl:latest
# The final stage is named `runtime` so CI can pass
# `no-cache-filters: runtime` to docker/build-push-action. Without it the
# distro-upgrade layer below is cached FOREVER: the base is digest-pinned and
# the RUN string never changes, so its cache key is permanently stable and
# `apk upgrade` never actually executes again. `simplecontainer/github-actions:latest`
# shipped python3 3.14.5-r0 (12 HIGH) for exactly this reason while Alpine
# already served 3.14.7-r1. Note `--no-cache` on the apk line is unrelated — it
# governs apk's own index cache, not Docker layers.
FROM alpine/kubectl:latest@sha256:036b8cf863b64e4c4dc0ca34f8b30fa9a151e7ae27631acfd7dccdc8bac85e52 AS runtime

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

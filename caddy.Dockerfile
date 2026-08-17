# Caddy 2.11.4: closes CVE-2026-52844, CVE-2026-52845 and CVE-2026-52846 on
# top of 2.11.3's vendored-dep CVEs (go-jose v4, otel, smallstep/certificates)
# and core fastcgi + admin-socket auth-bypass fixes.
#
# The version lives in ONE place: the CADDY_VERSION ARG below. It feeds both
# FROMs and `xcaddy build`, because `COPY --from=builder /usr/bin/caddy`
# overwrites the runtime image's own binary — so a builder/runtime version skew
# ships silently. Only the two digests are per-tag and must be refreshed with it.
# Refresh: docker buildx imagetools inspect caddy:X.Y.Z[-builder]
#
# NOTE: 2.11.4 is a security release upstream flags as breaking if you relied on
# the buggy behaviour — request header fields containing underscores are now
# ignored, Windows backslashes are normalised in the path matcher, and `rewrite`
# no longer re-expands placeholders in an injected query. SC's own generated
# Caddyfiles use none of those, but consumers injecting headers via
# lbConfig.extraHelpers / siteExtraHelpers should check for underscore-named
# request headers.
#
# Plugins:
# - github.com/grafana/certmagic-gcs — GCS-backed certmagic storage for GKE.
# - github.com/mholt/caddy-ratelimit — request rate limiting (third-party
#   module; not part of the official Caddy distribution).
#   Needed for deployments where the SC-managed Caddy is the only HTTP layer
#   between client and origin (no upstream CDN that could enforce rate-limit
#   at the edge instead). Caddy's standard library has no rate-limit handler,
#   so a plugin is required to enforce this from Caddy at all.
#
#   Pinned to commit 16aecbb (2026-05-21). The repo's only formal tag is
#   v0.1.0 (predates ipv4_prefix/ipv6_prefix subnet matching + metrics
#   support), and the module is actively maintained by Caddy's own author,
#   so a commit-pin is the conventional approach here. The build-time
#   `caddy list-modules | grep` below guards against silent plugin drops.
#
#   Usage from SC consumers (cloud-compose stack yaml). Real upstream
#   Caddyfile syntax (verified against the module README — there is NO
#   `rate_limit_zones` global block):
#
#       rate_limit {
#           distributed                       # required for multi-replica
#           zone login {
#               key    {remote_host}          # or a header / custom field
#               events 5
#               window 1m
#           }
#           zone api {
#               key    {remote_host}
#               events 60
#               window 1m
#           }
#       }
#
#   The `rate_limit` directive is a SITE-LEVEL HTTP handler (sibling of
#   `reverse_proxy`), NOT a `reverse_proxy` subdirective — so it can't be
#   placed via the existing `lbConfig.extraHelpers` field, which renders
#   inside the `reverse_proxy` block. Consumers wire it via the new
#   `lbConfig.siteExtraHelpers` field introduced in the same PR as this
#   plugin (see pkg/api/client.go + simple_container.go).
#
#   Two landmines worth knowing:
#     - WITHOUT `distributed`, rate-limit state is per-pod in-memory. On
#       multi-replica deployments a "5/min login" limit becomes 5×replicas
#       per minute → enforcement silently weakened. `distributed` requires
#       a shared Caddy storage module — the parent stack already uses
#       certmagic-gcs, which doubles as shared storage.
#     - `{remote_host}` is only the true client IP if Caddy actually sees
#       it. On a GKE Service with default `externalTrafficPolicy: Cluster`
#       the source IP is SNAT'd to a node IP, so all clients collapse into
#       a few buckets — rate-limit becomes either useless or self-DoS.
#       Verify the LB is `externalTrafficPolicy: Local` + the parent
#       Caddy's `trustedProxies` covers the LB CIDR range.

FROM caddy:2.11.4-builder@sha256:c7ae80243a530d532d20062d56d6198b3ab161eb6971d28716ef7ec55599fea4 AS builder

# `$CADDY_VERSION` is set by the base image itself (v2.11.4 here), so xcaddy
# builds exactly the version the builder ships and a skew is impossible by
# construction — there is no second version literal to forget. The tag on the
# FROM line is informational only; the digest is what resolves.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache,sharing=locked \
    test -n "${CADDY_VERSION}" \
    && xcaddy build "${CADDY_VERSION}" \
        --with github.com/grafana/certmagic-gcs@v0.1.7 \
        --with github.com/mholt/caddy-ratelimit@16aecbbcb8ca07dc1c671e263379606ff9493c55 \
    && caddy version | grep -qF "${CADDY_VERSION} " \
    && caddy list-modules | grep -qE '^http\.handlers\.rate_limit$' \
    && caddy list-modules | grep -qE '^caddy\.storage\.gcs$'
# ^ The greps are gates, not decoration:
#   - `caddy version | grep` pins the built binary to the base image's own
#     version. The previous line printed `caddy version` and never compared it,
#     which is how a 2.11.3 binary shipped inside a 2.11.4 base unnoticed.
#   - both module greps catch a silently dropped plugin (xcaddy does this when
#     versions disagree). Dropping certmagic-gcs is the expensive one: Caddy
#     falls back to local-filesystem cert storage, so a multi-replica parent
#     stack gets per-pod ACME state and risks Let's Encrypt rate-limit lockout.

FROM caddy:2.11.4@sha256:df7f1c2fb114453b951de51a98efc010db1655a92c2e86be6706714e2417a78d

RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/*

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

# Re-assert against the RUNTIME base's own $CADDY_VERSION: this is what catches
# builder/runtime digest skew, and stops a cache-hit builder stage slipping a
# stale binary into a freshly-pulled runtime base.
RUN test -n "${CADDY_VERSION}" && caddy version | grep -qF "${CADDY_VERSION} "

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/caddy" \
      org.opencontainers.image.description="Caddy with grafana/certmagic-gcs + mholt/caddy-ratelimit"

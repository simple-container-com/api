# Caddy 2.11.3: closes vendored-dep CVEs in 2.11.2's binary (go-jose v4,
# otel, smallstep/certificates) plus Caddy core fastcgi + admin-socket
# auth-bypass fixes — see https://github.com/caddyserver/caddy/releases/tag/v2.11.3.
# Bumping requires editing all three "2.11.x" sites below (two FROMs + xcaddy).
# Refresh: docker buildx imagetools inspect caddy:X.Y.Z[-builder]
#
# Plugins:
# - github.com/grafana/certmagic-gcs — GCS-backed certmagic storage for GKE.
# - github.com/mholt/caddy-ratelimit — request rate limiting (third-party
#   module; not part of the official Caddy distribution).
#   Needed for grey-cloud-only deployments (no Cloudflare in front of the
#   origin) where a CF zone rate-limit rule can't be the enforcement layer
#   — e.g. PAY-SPACE's pay.space / app.pay.space / app.x-core.pro, which
#   are intentionally DNS-only because CF is blocked in Russia.
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

FROM caddy:2.11.3-builder@sha256:f96a3b748f2ce4e5f6595453615da734b93993b231213fe35d0673893b5613ef AS builder

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache,sharing=locked \
    xcaddy build "v2.11.3" \
        --with github.com/grafana/certmagic-gcs@v0.1.7 \
        --with github.com/mholt/caddy-ratelimit@16aecbb24beddc9095da2716fa8d3a30fa2dc8ea \
    && caddy version \
    && caddy list-modules | grep -qE '^http\.handlers\.rate_limit$'
# ^ Final grep is a sanity check that the ratelimit module actually registered
# into the resulting binary (xcaddy has been known to silently drop plugins
# when versions disagree). If this fails the RUN exits non-zero with the
# failing command visible — no misleading prefixed echo.

FROM caddy:2.11.3@sha256:ec18ee54aab3315c22e25f3b2babda73ff8007d39b13b3bd1bfffa2f0444c7d9

RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/*

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/caddy" \
      org.opencontainers.image.description="Caddy with grafana/certmagic-gcs + mholt/caddy-ratelimit"

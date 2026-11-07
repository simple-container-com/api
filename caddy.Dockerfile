# Caddy 2.11.9: the end of the 2.11.6 regression train, plus the FastCGI
# security batch. 2.11.6 was a large hardening release that introduced default
# idle read/write timeouts and then broke streaming with them; 2.11.7, 2.11.8
# and 2.11.9 are the fixes. Landing on anything below 2.11.9 would ship a known
# stream-cutting bug, so the three are not separable.
#
# On top of 2.11.4 (CVE-2026-52844/52845/52846) this carries:
#   - GHSA-q34h-28rj-5hjm — a client could override any `HTTP_*` variable set
#     via the FastCGI transport's `env` subdirective by sending a matching
#     request header (e.g. choosing the user an app sees in
#     `HTTP_X_AUTH_USER`). Configured values now win. SC generates no FastCGI
#     config, so this is not reachable through an SC-rendered Caddyfile — it is
#     reachable through `lbConfig.siteExtraHelpers` if a consumer writes one.
#   - slowloris mitigation (idle read/write timeouts, optional minimum transfer
#     rates, a per-route `timeouts` directive)
#   - the cel-go advisory below, closed by upstream adopting cel.dev/cel-go
#
# The version lives in ONE place: the CADDY_VERSION ARG below. It feeds both
# FROMs and `xcaddy build`, because `COPY --from=builder /usr/bin/caddy`
# overwrites the runtime image's own binary — so a builder/runtime version skew
# ships silently. Only the two digests are per-tag and must be refreshed with it.
# Refresh: docker buildx imagetools inspect caddy:X.Y.Z[-builder]
#
# NOTE — breaking changes across 2.11.4 → 2.11.9 that a consumer can trip:
#   - request header fields containing underscores are ignored (2.11.4). If that
#     broke an app, 2.11.6 added `expected_underscore_headers` to list the
#     specific headers to keep.
#   - default idle read/write timeouts exist now (2.11.6). As shipped they only
#     cut a connection that has stalled, not one that is slow but progressing —
#     but that is true only as of 2.11.7/2.11.8/2.11.9; the per-route
#     `timeouts` directive is the override, and a handler's own deadline is a
#     ceiling rather than being overridden (2.11.8). SC proxies long-lived
#     streams (SSE, the stream-relay), which is the reason this image must not
#     sit on 2.11.6.
#   - two site blocks sharing a hostname's TLS connection policy with different
#     `client_auth` is a Caddyfile ERROR rather than silently keeping the first
#     (2.11.9). Previously that could quietly drop a stricter mTLS requirement.
#   - Windows backslash normalisation in the path matcher, and `rewrite` no
#     longer re-expanding placeholders in an injected query (2.11.4).
# SC's own generated Caddyfiles use none of these; consumers injecting config
# via `lbConfig.extraHelpers` / `siteExtraHelpers` should check.
#
# Plugins:
# - github.com/grafana/certmagic-gcs — GCS-backed certmagic storage for GKE.
# - github.com/techknowlogick/certmagic-s3 — S3-compatible certmagic storage
#   (module id `caddy.storage.s3`), for every fleet that is not on GCS.
#
#   This is a PREREQUISITE of HTTPS on a multi-machine VM fleet, not an
#   optimisation. The image carries no DNS-provider module, so HTTP-01 is the
#   only ACME challenge available; a fleet behind round-robin A records sends
#   Let's Encrypt's validator to an arbitrary machine, which 404s a token
#   another machine minted. certmagic's distributed solver shares that
#   challenge state — and the issued certificate — through the storage module,
#   so one shared store turns N machines into one ACME client. Without it the
#   fleet either serves plain HTTP on the origin or walks into a rate-limit
#   lockout. See forge `docs/design/2026-11-07/multi-node-clusters/` §5.1-5.2.
#
#   Pinned to v1.5.0 (2026-10-02), which is the first tag carrying #25 —
#   `Lock` made mutually exclusive across instances via S3 conditional writes.
#   An advisory lock that is not actually exclusive is the one defect that
#   matters here: it is precisely what keeps two machines from both answering
#   an ACME order. Apache-2.0. Picked over github.com/ss098/certmagic-s3
#   (more stars) because that one still pins caddy v2.5.1 + certmagic v0.16.1
#   from 2025 and would not build against this base.
#
#       {
#           storage s3 {
#               bucket       {env.ACME_BUCKET}
#               endpoint     {env.ACME_S3_ENDPOINT}   # minio / yc / cloud.ru
#               region       {env.ACME_S3_REGION}
#               access_key   {env.ACME_S3_ACCESS_KEY}
#               secret_key   {env.ACME_S3_SECRET_KEY}
#               use_path_style                        # minio + most non-AWS
#               prefix       acme
#           }
#       }
#
#   `use_path_style` is not cosmetic: virtual-host addressing requires the
#   bucket name to be a DNS label of the endpoint, which minio and several
#   S3-compatible clouds do not serve. `encryption_key` (exactly 32 bytes) is
#   available and encrypts objects at rest with secretbox; without it the
#   module logs "Clear text certificate storage active" and stores private keys
#   as-is, which is only acceptable when the bucket is private AND the cloud
#   encrypts at rest.
#
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
#       a shared Caddy storage module — on GKE that is certmagic-gcs, and on
#       a VM fleet it is certmagic-s3 above. Both double as shared storage.
#     - `{remote_host}` is only the true client IP if Caddy actually sees
#       it. On a GKE Service with default `externalTrafficPolicy: Cluster`
#       the source IP is SNAT'd to a node IP, so all clients collapse into
#       a few buckets — rate-limit becomes either useless or self-DoS.
#       Verify the LB is `externalTrafficPolicy: Local` + the parent
#       Caddy's `trustedProxies` covers the LB CIDR range.

FROM caddy:2.11.9-builder@sha256:3fddbec729442a56b942d4c9eaef1df80893dbf2cf5b86701bfae6aad1408eeb AS builder

# `$CADDY_VERSION` is set by the base image itself (v2.11.9 here), so xcaddy
# builds exactly the version the builder ships and a skew is impossible by
# construction — there is no second version literal to forget. The tag on the
# FROM line is informational only; the digest is what resolves.
#
# There are NO `--replace` lines any more, and adding one back needs a reason.
# 2.11.4 vendored deps behind CVE-2026-46600 (x/net), CVE-2026-56852 (x/text)
# and GHSA-hrxh-6v49-42gf (grpc), so this file carried three replaces lifting
# them to x/net v0.58.0, x/text v0.41.0 and grpc v1.82.1. 2.11.9 requires
# x/net v0.60.0, x/text v0.42.0 and grpc v1.83.2 — all newer. Keeping the
# replaces would have pinned the build BACKWARDS onto the older three, which
# is the exact failure the previous revision of this comment warned about:
# a replace is absolute, not a floor.
#
# cel-go is likewise resolved and no longer an exception. 2.11.4 could not take
# github.com/google/cel-go v0.29.0 (it renames interpreter.Interpretable, an
# upstream code change rather than a version bump), which left
# GHSA-gcjh-h69q-9w9g (MEDIUM) open. 2.11.9 depends on cel.dev/cel-go v0.32.0
# — the module's new path, past the rename — so the advisory is closed by the
# base bump itself.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache,sharing=locked \
    test -n "${CADDY_VERSION}" \
    && xcaddy build "${CADDY_VERSION}" \
        --with github.com/grafana/certmagic-gcs@v0.1.7 \
        --with github.com/techknowlogick/certmagic-s3@v1.5.0 \
        --with github.com/mholt/caddy-ratelimit@16aecbbcb8ca07dc1c671e263379606ff9493c55 \
    && caddy version | grep -qF "${CADDY_VERSION} " \
    && caddy list-modules | grep -qE '^http\.handlers\.rate_limit$' \
    && caddy list-modules | grep -qE '^caddy\.storage\.gcs$' \
    && caddy list-modules | grep -qE '^caddy\.storage\.s3$'
# ^ The greps are gates, not decoration:
#   - `caddy version | grep` pins the built binary to the base image's own
#     version. The previous line printed `caddy version` and never compared it,
#     which is how a 2.11.3 binary shipped inside a 2.11.4 base unnoticed. It
#     is also what catches a plugin whose own `caddy/v2` requirement is HIGHER
#     than the base: MVS would quietly build that newer Caddy instead.
#   - the module greps catch a silently dropped plugin (xcaddy does this when
#     versions disagree). Dropping either storage module is the expensive one:
#     Caddy falls back to local-filesystem cert storage, so every replica or
#     machine gets its own ACME state — per-pod on GKE, per-machine on a VM
#     fleet — and the fleet risks a Let's Encrypt rate-limit lockout while
#     serving certificates no sibling can validate.

# The final stage is named `runtime` so CI can pass
# `no-cache-filters: runtime` to docker/build-push-action. Without it the
# distro-upgrade layer below is cached FOREVER: the base is digest-pinned and
# the RUN string never changes, so its cache key is permanently stable and
# `apk upgrade` never actually executes again. `simplecontainer/github-actions:latest`
# shipped python3 3.14.5-r0 (12 HIGH) for exactly this reason while Alpine
# already served 3.14.7-r1. Note `--no-cache` on the apk line is unrelated — it
# governs apk's own index cache, not Docker layers.
FROM caddy:2.11.9@sha256:0c2243efcf437ab7f7a978f1027fcf0d4f9be8a75df27b1b3fbc5dbe0ffa8cf0 AS runtime

RUN apk update && apk upgrade --no-cache && rm -rf /var/cache/apk/*

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

# Re-assert against the RUNTIME base's own $CADDY_VERSION: this is what catches
# builder/runtime digest skew, and stops a cache-hit builder stage slipping a
# stale binary into a freshly-pulled runtime base.
RUN test -n "${CADDY_VERSION}" && caddy version | grep -qF "${CADDY_VERSION} "

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/caddy" \
      org.opencontainers.image.description="Caddy with grafana/certmagic-gcs + techknowlogick/certmagic-s3 + mholt/caddy-ratelimit"

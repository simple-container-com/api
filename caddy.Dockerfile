# Caddy version bump: 2.8.4 → 2.11.2 — clears Go stdlib CVEs that were present
# in the older Caddy binary (CVE-2025-58187/58188/58189, CVE-2025-61723/61724/
# 61725/61727/61730, CVE-2026-27139/27142, CVE-2026-32282/32288/32289) and the
# Caddy-level CVE-2026-27586 (HIGH) reachable in <2.11.1.
#
# Versions live in three places that MUST be kept in sync when bumping:
#   - the FROM digest of caddy:X.Y.Z-builder
#   - the FROM digest of caddy:X.Y.Z
#   - the literal in xcaddy build "vX.Y.Z" below
# Refresh digests via `docker buildx imagetools inspect caddy:X.Y.Z[-builder]`.

# Pin builder by digest (CIS Docker 4.7).
FROM caddy:2.11.2-builder@sha256:10ed0251c5cd1dbb4db0b71ad43121147961a51adfec35febce2c93ea25c24f4 AS builder

# certmagic-gcs bumped 0.1.2 → 0.1.7 to align with current upstream.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache,sharing=locked \
    xcaddy build "v2.11.2" \
        --with github.com/grafana/certmagic-gcs@v0.1.7 \
    && caddy version

# Pin runtime by digest.
FROM caddy:2.11.2@sha256:25cdc846626b62d05f6b633b9b40c2c9f6ef89b515dc76133cefd920f7dbe562

# Pull post-tag distro security updates without bloating the layer.
RUN apk update \
    && apk upgrade --no-cache \
    && rm -rf /var/cache/apk/*

# Replace upstream binary with the build that has certmagic-gcs.
COPY --from=builder /usr/bin/caddy /usr/bin/caddy

# No HEALTHCHECK: a meaningful liveness probe would need to hit the port the
# consumer binds (or the admin API at :2019, which many consumers disable).
# Both are config-specific. Probing only the binary (`caddy version`) reports
# healthy even when the daemon is crashlooping, which is worse than no probe.
# Consumers who run Caddy in orchestrators should add a HEALTHCHECK in their
# own deployment manifest or use the orchestrator's HTTP probe primitives.

# Note on USER: upstream caddy:2.11.2 runs as root so it can bind 80/443. Switching
# to non-root requires setcap CAP_NET_BIND_SERVICE on the binary AND certmagic state
# directories owned by that user, which is intrusive given consumers mount their own
# volumes. Tracked for follow-up; defaults preserved here.

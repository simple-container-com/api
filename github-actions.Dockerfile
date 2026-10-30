# GitHub docker-action runtime: builder downloads/verifies/slims tools,
# runtime keeps only what github-actions invokes via exec.LookPath.
#
# USER stays root: GitHub mounts /github/workspace as root, non-root breaks
# git ops. HEALTHCHECK omitted: one-shot action, never long-running.

# Refresh: docker buildx imagetools inspect alpine:3.24
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS builder

# python3 needed so `gcloud components install` doesn't fall back to (and recreate) the bundled Python we want to delete.
RUN apk update && apk upgrade --no-cache \
    && apk add --no-cache curl bash binutils upx ca-certificates tar python3 \
    && rm -rf /var/cache/apk/*

# Pulumi: version from go.mod, SHA-256 verified against per-release checksums
# file Pulumi publishes on GitHub Releases (replaces curl|sh from get.pulumi.com).
# Cache mount avoids re-downloading the tarball; integrity check runs every build.
COPY go.mod /tmp/go.mod
RUN --mount=type=cache,target=/tmp/pulumi-dl,sharing=locked \
    set -euo pipefail \
    && PULUMI_VERSION="$(grep 'github.com/pulumi/pulumi/sdk/v3' /tmp/go.mod | awk '{print $2}' | sed 's/^v//')" \
    && [ -n "${PULUMI_VERSION}" ] || { echo "no pulumi version in go.mod" >&2; exit 1; } \
    && TARBALL="pulumi-v${PULUMI_VERSION}-linux-x64.tar.gz" \
    && CHECKSUMS="pulumi-${PULUMI_VERSION}-checksums.txt" \
    && cd /tmp/pulumi-dl \
    && [ -f "${TARBALL}" ] || curl -fsSL -o "${TARBALL}" \
        "https://github.com/pulumi/pulumi/releases/download/v${PULUMI_VERSION}/${TARBALL}" \
    && curl -fsSL -o "${CHECKSUMS}" \
        "https://github.com/pulumi/pulumi/releases/download/v${PULUMI_VERSION}/${CHECKSUMS}" \
    && EXPECTED_SHA="$(grep "${TARBALL}" "${CHECKSUMS}" | awk '{print $1}')" \
    && [ -n "${EXPECTED_SHA}" ] || { echo "no checksum entry for ${TARBALL}" >&2; exit 1; } \
    && echo "${EXPECTED_SHA}  ${TARBALL}" | sha256sum -c - \
    && mkdir -p /opt/pulumi/bin \
    && tar -xzf "${TARBALL}" -C /tmp \
    && mv /tmp/pulumi/* /opt/pulumi/bin/ \
    && rm -rf /tmp/pulumi /tmp/go.mod \
    && { strip /opt/pulumi/bin/* 2>/dev/null || true; } \
    && { upx --best --lzma /opt/pulumi/bin/* 2>/dev/null || true; }

# gcloud: pinned version + SHA-256 (Google does not publish per-release sig).
# Refresh: pull the tarball, sha256sum it, paste below.
# The component install has a fixed 60s read timeout in gcloud, so it retries.
ARG GCLOUD_VERSION="579.0.0"
ARG GCLOUD_SHA256="a9a7fbe51cda37cf6142b1bbcff12227550e60a6c67e8cf84644fb301371c4de"
RUN --mount=type=cache,target=/tmp/gcloud-dl,sharing=locked \
    set -euo pipefail \
    && TARBALL="google-cloud-cli-${GCLOUD_VERSION}-linux-x86_64.tar.gz" \
    && cd /tmp/gcloud-dl \
    && [ -f "${TARBALL}" ] || curl -fsSL -o "${TARBALL}" \
        "https://storage.googleapis.com/cloud-sdk-release/${TARBALL}" \
    && echo "${GCLOUD_SHA256}  ${TARBALL}" | sha256sum -c - \
    && tar -xzf "${TARBALL}" -C /opt \
    && /opt/google-cloud-sdk/install.sh --quiet \
        --usage-reporting=false --path-update=false --bash-completion=false \
    && for attempt in 1 2 3 4; do \
         /opt/google-cloud-sdk/bin/gcloud components install gke-gcloud-auth-plugin --quiet && break; \
         test "${attempt}" -lt 4 || exit 1; \
         sleep $((attempt * 20)); \
       done

# Slim gcloud — must be a SEPARATE RUN because `gcloud components install`
# touches `bundledpythonunix` after the rm chain in the same RUN executes.
RUN rm -rf \
        /opt/google-cloud-sdk/.install/.backup \
        /opt/google-cloud-sdk/.install/.download \
        /opt/google-cloud-sdk/bin/anthoscli \
        /opt/google-cloud-sdk/bin/docker-credential-gcloud \
        /opt/google-cloud-sdk/bin/git-credential-gcloud.sh \
        /opt/google-cloud-sdk/platform/bundledpythonunix \
        /opt/google-cloud-sdk/platform/gsutil/third_party/pyasn1* \
        /opt/google-cloud-sdk/platform/gsutil/third_party/rsa/doc \
        /opt/google-cloud-sdk/platform/gsutil/third_party/oauth2client/contrib \
        /opt/google-cloud-sdk/platform/gsutil/third_party/urllib3/dummyserver \
        /opt/google-cloud-sdk/lib/third_party/grpc \
        /opt/google-cloud-sdk/lib/googlecloudsdk/api_lib/container/images \
        /opt/google-cloud-sdk/help \
        /opt/google-cloud-sdk/data/cli \
        /opt/google-cloud-sdk/completion.bash.inc \
        /opt/google-cloud-sdk/completion.zsh.inc \
        /opt/google-cloud-sdk/path.bash.inc \
        /opt/google-cloud-sdk/path.zsh.inc \
        /root/.config/gcloud/logs \
        /root/.config/gcloud/.last_update_check.json \
        /root/.config/gcloud/.last_opt_in_prompt.yaml \
        /root/.config/gcloud/configurations \
    && find /opt/google-cloud-sdk -name "*.pyc" -delete \
    && find /opt/google-cloud-sdk -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true \
    && find /opt/google-cloud-sdk \( -name "*.md" -o -name "*.txt" -o -name "COPYING*" -o -name "LICENSE*" \) -delete \
    && rm -rf /tmp/* /var/tmp/*

# ── runtime ─────────────────────────────────────────────────────────────────
# The final stage is named `runtime` so CI can pass
# `no-cache-filters: runtime` to docker/build-push-action. Without it the
# distro-upgrade layer below is cached FOREVER: the base is digest-pinned and
# the RUN string never changes, so its cache key is permanently stable and
# `apk upgrade` never actually executes again. `simplecontainer/github-actions:latest`
# shipped python3 3.14.5-r0 (12 HIGH) for exactly this reason while Alpine
# already served 3.14.7-r1. Note `--no-cache` on the apk line is unrelated — it
# governs apk's own index cache, not Docker layers.
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS runtime

# python3 stays — gcloud invokes it. py3-pip / binutils / upx confined to builder.
# aws-cli needed by Pulumi local.Command shell-outs (e.g. `aws s3 sync` in the
# static-website template at pkg/clouds/pulumi/aws/static_website.go).
#
# Retried because dl-cdn.alpinelinux.org times out when it has to share the
# runner's egress with a large BuildKit cache import. This RUN is unavoidably
# the one that races it: it is the runtime stage's first layer, so BuildKit
# schedules it in parallel with the builder stage, and on a cold builder that
# means ~287 MB of cache layers in flight at the same time (release run
# 39571929882 — `Operation timed out` on APKINDEX at 60.11 s, while stage 17
# was pulling at ~2.5 MB/s). apk exposes no network-timeout knob, so retrying
# is the only lever; same shape as the `gcloud components install` retry above.
#
# Deliberately NOT applied to the builder stage's apk: that layer is CACHED and
# pulumi/gcloud sit downstream of it, so editing it busts a ~500 MB download on
# any runner whose cache mounts are cold (BuildKit never exports cache mounts).
RUN for attempt in 1 2 3 4; do \
        apk update && apk upgrade --no-cache \
        && apk add --no-cache ca-certificates git openssh-client curl jq bash python3 aws-cli \
        && break; \
        test "${attempt}" -lt 4 || exit 1; \
        echo "apk attempt ${attempt} failed, retrying" >&2; \
        sleep $((attempt * 15)); \
    done \
    && rm -rf /var/cache/apk/* /tmp/* /var/tmp/*

COPY --from=builder /opt/pulumi /opt/pulumi
COPY --from=builder /opt/google-cloud-sdk /opt/google-cloud-sdk

ENV PATH="/opt/pulumi/bin:/opt/google-cloud-sdk/bin:${PATH}"

WORKDIR /root/

# The binary is compiled by the CALLER, natively, and arrives in the build
# context at dist/github-actions. Every caller already did this except
# push.yaml: welder's `build-github-actions` task writes exactly this path
# (welder.yaml:124-131) and runs before `dockerImages`, and
# branch-preview.yaml downloads the build-binaries artifact into `dist/`.
# Both of them were having that binary thrown away by an in-image `gobuild`
# stage that recompiled the same ~2095-package closure.
#
# Why not compile in-image (the shape here until 2026-10-30): buildkit does
# not export `--mount=type=cache` contents through ANY cache exporter, so an
# in-image compile can only be warm if the buildkit INSTANCE survives. On the
# fleet it often does not — `/cache/docker` is wiped wholesale by
# guest-cache-gc at >=88% and nothing reseeds it — and on an ephemeral runner
# it never does. Measured across six releases, the same closure under the same
# flags: 760-1812 s in-image on 10 of 12 legs, against 141-310 s natively on
# EVERY run, because the guest's GOCACHE (/cache/go-build) is seeded by
# reflink from a golden drive whose bake explicitly covers this namespace
# (forgeci-cache-bake, "[B] linux/amd64 -trimpath CGO=0 (binaries + docker
# gobuild)"). The in-image compile was the one consumer in the fleet that
# structurally could not read that warmth.
#
# This is NOT the artifact round-trip that the gobuild stage replaced. That
# shape uploaded the binary from a separate job and downloaded it here —
# measured 448 s in the artifact store per ~70 MB incompressible binary, and
# 866 s on the fleet (run 39253348066), because fleet egress is the
# bottleneck. The compile now happens in THIS job, so there is no artifact
# store and no egress.
#
# The flags must stay identical in all three callers, or the shipped binary
# stops being the build the release attests: -trimpath -buildvcs=false,
# CGO_ENABLED=0, -ldflags "-s -w -X=.../internal/build.Version=<version>".
#
# chmod is belt-and-braces: `go build` sets the bit and the context preserves
# it, but branch-preview's path goes through actions/upload-artifact, which
# does not.
COPY dist/github-actions ./github-actions
# `sc` symlink so Pulumi local.Command subprocesses can invoke sc image sign/scan/sbom etc.
RUN chmod +x ./github-actions \
    && test -x ./github-actions \
    && ln -s /root/github-actions /usr/local/bin/sc

# Build-time smoke test — fails the build if tool wiring breaks.
RUN pulumi version > /dev/null \
    && gcloud version > /dev/null \
    && gcloud components list --filter="name:gke-gcloud-auth-plugin" --format="value(name)" | grep -q gke-gcloud-auth-plugin \
    && aws --version > /dev/null \
    && test -L /usr/local/bin/sc && test -x /usr/local/bin/sc

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/github-actions" \
      org.opencontainers.image.description="SC GitHub Actions runner image"

ENTRYPOINT ["/root/github-actions"]

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
# BLACKSMITH TRADEOFF (read before re-enabling Blacksmith).
# This stage replaced an artifact handoff: build-binaries compiled the binary
# in a parallel job and docker-build downloaded it. That download measured
# 866 s on the self-hosted fleet (run 39253348066) because the fleet's egress
# is the bottleneck — which is what justified moving the compile in here.
# On BLACKSMITH the arithmetic may invert: slots are plentiful, the artifact
# store is fast, and setup-go's cache made that parallel compile ~4.6 min,
# whereas this in-container compile cannot reuse setup-go's GOCACHE and its
# buildkit cache mounts do not survive an ephemeral runner. So if Blacksmith
# comes back, MEASURE this job before keeping it.
# Revert recipe (all of it, or the image loses its binary):
#   1. this stage out; `COPY --from=gobuild /out/<bin>` -> `COPY dist/<bin>`
#   2. re-add `!dist` to .dockerignore (the allowlist excludes it today)
#   3. push.yaml: restore the two `download-artifact` steps in docker-build
#      and the `upload-artifact` steps in build-binaries
# The conditional-stage pattern (ARG BIN_SOURCE + `FROM bin-${BIN_SOURCE}`)
# can serve both shapes from one Dockerfile if it is ever worth the
# complexity; it was judged not to be while only the fleet is in use.
# Go builder stage. The binary used to arrive as a `dist/` ARTIFACT from
# build-binaries — measured 448 s in the artifact store per ~70 MB
# incompressible binary (run 39253348066), twice per release, re-fetching
# bytes the pipeline had compiled minutes earlier. Building it here removes
# the round-trip entirely, with the exact workflow flags so the binary is the
# same build: -trimpath -buildvcs=false, CGO_ENABLED=0, same -X version stamp
# (VERSION arrives as a build-arg from push.yaml; the 0.0.0 fallback matches
# local builds). go.mod/go.sum key their own `go mod download` layer, cached
# through the gha buildx cache. The .dockerignore allowlist keeps this stage
# to cmd/pkg/internal — .sc/ (decrypted secrets post `sc secrets reveal`) and
# everything else cannot enter the context at all.
FROM golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS gobuild
ARG VERSION=0.0.0
WORKDIR /src
COPY go.mod go.sum ./
# Same cache mount as the build below, deliberately: GOMODCACHE lives at
# /go/pkg/mod, so without the mount here this layer would populate the image
# filesystem while the build step reads the (empty) cache mount instead —
# downloading every module twice. With it, both steps share one store.
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# Cache mounts, not a bare RUN: without them ANY source change invalidates
# `COPY . .` and recompiles the whole ~2095-package closure inside the
# container on every build. With them the compile is incremental on a
# PERSISTENT builder (the fleet guests keep their dockerd and are now
# golden-seeded). Note the limit honestly: buildkit does NOT export
# type=cache mounts through `cache-to: type=gha`, so on an EPHEMERAL runner
# (Blacksmith / ubuntu-latest) these mounts start empty every run — see the
# BLACKSMITH TRADEOFF note above the gobuild stage.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=${VERSION}" \
      -o /out/github-actions ./cmd/github-actions

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS runtime

# python3 stays — gcloud invokes it. py3-pip / binutils / upx confined to builder.
# aws-cli needed by Pulumi local.Command shell-outs (e.g. `aws s3 sync` in the
# static-website template at pkg/clouds/pulumi/aws/static_website.go).
RUN apk update && apk upgrade --no-cache \
    && apk add --no-cache ca-certificates git openssh-client curl jq bash python3 aws-cli \
    && rm -rf /var/cache/apk/* /tmp/* /var/tmp/*

COPY --from=builder /opt/pulumi /opt/pulumi
COPY --from=builder /opt/google-cloud-sdk /opt/google-cloud-sdk

ENV PATH="/opt/pulumi/bin:/opt/google-cloud-sdk/bin:${PATH}"

WORKDIR /root/

COPY --from=gobuild /out/github-actions ./github-actions
# `sc` symlink so Pulumi local.Command subprocesses can invoke sc image sign/scan/sbom etc.
RUN chmod +x ./github-actions \
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

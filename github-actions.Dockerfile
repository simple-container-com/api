# GitHub docker-action runtime: builder downloads/verifies/slims tools,
# runtime keeps only what github-actions invokes via exec.LookPath.
#
# USER stays root: GitHub mounts /github/workspace as root, non-root breaks
# git ops. HEALTHCHECK omitted: one-shot action, never long-running.

# Refresh: docker buildx imagetools inspect alpine:3.21
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
    && strip /opt/pulumi/bin/* 2>/dev/null || true \
    && upx --best --lzma /opt/pulumi/bin/* 2>/dev/null || true

# gcloud: pinned version + SHA-256 (Google does not publish per-release sig).
# Refresh: pull the tarball, sha256sum it, paste below.
ARG GCLOUD_VERSION="567.0.0"
ARG GCLOUD_SHA256="bd5afc0d249609cb40d45f665209190fdd38b9937954291b8f9ae54206c75d83"
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
    && /opt/google-cloud-sdk/bin/gcloud components install gke-gcloud-auth-plugin --quiet

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
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

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

COPY dist/github-actions ./github-actions
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

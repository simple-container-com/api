# Staging variant of github-actions.Dockerfile. Identical hardening; only
# difference is that it consumes ./bin/github-actions (built by welder) rather
# than dist/github-actions (built by CI). Keep these two files in sync — any
# change to base, tooling versions, or runtime layout MUST be mirrored in
# github-actions.Dockerfile.

# ─────────────────────────────────────────────────────────────────────────────
# Stage 1: tool downloader/builder
# ─────────────────────────────────────────────────────────────────────────────
FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d AS builder

RUN apk update && apk upgrade --no-cache \
    && apk add --no-cache curl bash binutils upx ca-certificates tar python3 \
    && rm -rf /var/cache/apk/*
# python3 in the builder is required for `gcloud components install`; without it,
# gcloud falls back to its bundled Python (which is what we want to delete).

COPY go.mod /tmp/go.mod
RUN PULUMI_VERSION="$(grep 'github.com/pulumi/pulumi/sdk/v3' /tmp/go.mod | awk '{print $2}' | sed 's/^v//')" \
    && echo "Installing Pulumi ${PULUMI_VERSION}" \
    && cd /tmp \
    && curl -fsSL -o pulumi.tar.gz \
        "https://github.com/pulumi/pulumi/releases/download/v${PULUMI_VERSION}/pulumi-v${PULUMI_VERSION}-linux-x64.tar.gz" \
    && curl -fsSL -o pulumi-checksums.txt \
        "https://github.com/pulumi/pulumi/releases/download/v${PULUMI_VERSION}/pulumi-${PULUMI_VERSION}-checksums.txt" \
    && grep "pulumi-v${PULUMI_VERSION}-linux-x64.tar.gz" pulumi-checksums.txt \
        | awk '{print $1"  pulumi.tar.gz"}' \
        | sha256sum -c - \
    && mkdir -p /opt/pulumi/bin \
    && tar -xzf pulumi.tar.gz -C /tmp \
    && mv /tmp/pulumi/* /opt/pulumi/bin/ \
    && rm -rf pulumi.tar.gz pulumi-checksums.txt /tmp/pulumi /tmp/go.mod \
    && strip /opt/pulumi/bin/* 2>/dev/null || true \
    && upx --best --lzma /opt/pulumi/bin/* 2>/dev/null || true

ARG GCLOUD_VERSION="567.0.0"
ARG GCLOUD_SHA256="bd5afc0d249609cb40d45f665209190fdd38b9937954291b8f9ae54206c75d83"
RUN cd /tmp \
    && curl -fsSL -o gcloud.tar.gz \
        "https://storage.googleapis.com/cloud-sdk-release/google-cloud-cli-${GCLOUD_VERSION}-linux-x86_64.tar.gz" \
    && echo "${GCLOUD_SHA256}  gcloud.tar.gz" | sha256sum -c - \
    && tar -xzf gcloud.tar.gz -C /opt \
    && rm -f gcloud.tar.gz \
    && /opt/google-cloud-sdk/install.sh --quiet \
        --usage-reporting=false --path-update=false --bash-completion=false \
    && /opt/google-cloud-sdk/bin/gcloud components install gke-gcloud-auth-plugin --quiet

# Slim gcloud SDK — see github-actions.Dockerfile for the full rationale; must
# run AFTER `gcloud components install` in a separate RUN, otherwise gcloud
# touches `bundledpythonunix` again and the rm in the same chain becomes a no-op.
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
    && find /opt/google-cloud-sdk -name "*.md" -delete \
    && find /opt/google-cloud-sdk -name "*.txt" -delete \
    && find /opt/google-cloud-sdk -name "COPYING*" -delete \
    && find /opt/google-cloud-sdk -name "LICENSE*" -delete \
    && rm -rf /tmp/* /var/tmp/*

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2: runtime
# ─────────────────────────────────────────────────────────────────────────────
FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d

RUN apk update && apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        git \
        openssh-client \
        curl \
        jq \
        bash \
        python3 \
    && rm -rf /var/cache/apk/* /tmp/* /var/tmp/*

COPY --from=builder /opt/pulumi /opt/pulumi
COPY --from=builder /opt/google-cloud-sdk /opt/google-cloud-sdk

ENV PATH="/opt/pulumi/bin:/opt/google-cloud-sdk/bin:${PATH}"

WORKDIR /root/

# Staging path: welder writes the binary to ./bin/github-actions.
COPY ./bin/github-actions ./github-actions
RUN chmod +x ./github-actions \
    && ln -s /root/github-actions /usr/local/bin/sc

RUN pulumi version > /dev/null \
    && gcloud version > /dev/null \
    && gcloud components list --filter="name:gke-gcloud-auth-plugin" --format="value(name)" | grep -q gke-gcloud-auth-plugin \
    && test -L /usr/local/bin/sc && test -x /usr/local/bin/sc

HEALTHCHECK --interval=30s --timeout=5s --start-period=2s --retries=3 \
    CMD /root/github-actions --version >/dev/null 2>&1 || exit 1

# GitHub Actions runner overrides WORKDIR with --workdir /github/workspace, so
# the entrypoint needs to be an absolute path.
ENTRYPOINT ["/root/github-actions"]

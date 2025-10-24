FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Set Go toolchain to auto to allow downloading newer versions
ENV GOTOOLCHAIN=auto

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the GitHub Actions binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o github-actions ./cmd/github-actions

# Final stage - minimal runtime with optimizations
FROM alpine:3.19

# Install runtime dependencies in single layer with aggressive cleanup
RUN apk --no-cache add \
    ca-certificates \
    git \
    openssh-client \
    curl \
    jq \
    bash \
    python3 \
    py3-pip \
    upx \
    binutils \
    && rm -rf /var/cache/apk/* /tmp/* /var/tmp/*

# Install Pulumi CLI - Required for Simple Container provisioning
# Read version from go.mod to ensure consistency with Go dependencies
COPY go.mod /tmp/go.mod
RUN PULUMI_VERSION=$(grep 'github.com/pulumi/pulumi/sdk/v3' /tmp/go.mod | awk '{print $2}' | sed 's/^v//') && \
    echo "Installing Pulumi version: ${PULUMI_VERSION} (extracted from go.mod)" && \
    curl -fsSL https://get.pulumi.com | sh -s -- --version ${PULUMI_VERSION} && \
    # Optimize Pulumi binaries - strip debug symbols and compress
    strip /root/.pulumi/bin/* 2>/dev/null || true && \
    upx --best --lzma /root/.pulumi/bin/* 2>/dev/null || true && \
    rm -rf /tmp/* /var/tmp/*

ENV PATH="/root/.pulumi/bin:${PATH}"

# Install Google Cloud SDK (gcloud CLI) - Fixed installation with proper cleanup
RUN cd /tmp && \
    curl -sSL https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-x86_64.tar.gz -o gcloud.tar.gz && \
    tar -xzf gcloud.tar.gz && \
    mv google-cloud-sdk /opt/ && \
    /opt/google-cloud-sdk/install.sh --quiet --usage-reporting=false --path-update=false --bash-completion=false && \
    # Remove unnecessary components, documentation, and cache files
    rm -rf /opt/google-cloud-sdk/.install/.backup \
           /opt/google-cloud-sdk/.install/.download \
           /opt/google-cloud-sdk/bin/anthoscli \
           /opt/google-cloud-sdk/bin/docker-credential-gcloud \
           /opt/google-cloud-sdk/bin/git-credential-gcloud.sh \
           /opt/google-cloud-sdk/platform/bundledpythonunix \
           /opt/google-cloud-sdk/platform/gsutil/third_party/pyasn1* \
           /opt/google-cloud-sdk/platform/gsutil/third_party/rsa/doc \
           /opt/google-cloud-sdk/platform/gsutil/third_party/oauth2client/contrib \
           /opt/google-cloud-sdk/lib/third_party/grpc \
           /opt/google-cloud-sdk/lib/googlecloudsdk/api_lib/container/images \
           /opt/google-cloud-sdk/help \
           /opt/google-cloud-sdk/data/cli \
           /opt/google-cloud-sdk/completion.bash.inc \
           /opt/google-cloud-sdk/completion.zsh.inc \
           /opt/google-cloud-sdk/path.bash.inc \
           /opt/google-cloud-sdk/path.zsh.inc \
    && find /opt/google-cloud-sdk -name "*.pyc" -delete \
    && find /opt/google-cloud-sdk -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true \
    && find /opt/google-cloud-sdk -name "*.md" -delete \
    && find /opt/google-cloud-sdk -name "*.txt" -delete \
    && find /opt/google-cloud-sdk -name "COPYING*" -delete \
    && find /opt/google-cloud-sdk -name "LICENSE*" -delete \
    && rm -rf /tmp/gcloud.tar.gz /tmp/google-cloud-sdk

ENV PATH="/opt/google-cloud-sdk/bin:${PATH}"

# Install only essential GKE components and clean up immediately
RUN gcloud components install gke-gcloud-auth-plugin --quiet && \
    # Clean up component installation cache and logs
    rm -rf /root/.config/gcloud/logs \
           /root/.config/gcloud/.last_update_check.json \
           /root/.config/gcloud/.last_opt_in_prompt.yaml \
           /root/.config/gcloud/configurations \
           /tmp/* /var/tmp/*

WORKDIR /root/

# Copy the binary from builder stage and optimize it
COPY --from=builder /app/github-actions .
RUN chmod +x ./github-actions && \
    # Strip debug symbols if not already done (reduces binary size)
    strip ./github-actions 2>/dev/null || true && \
    # Remove build tools no longer needed
    apk del upx binutils && \
    rm -rf /var/cache/apk/* /tmp/* /var/tmp/*

# Verify installations work (but remove verification output to reduce layer size)
RUN pulumi version > /dev/null && \
    gcloud version > /dev/null && \
    gcloud components list --filter="name:gke-gcloud-auth-plugin" --format="value(name)" | grep -q gke-gcloud-auth-plugin

# Set the entrypoint
ENTRYPOINT ["./github-actions"]

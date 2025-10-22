# Staging GitHub Actions Dockerfile - Uses pre-built static github-actions binary for fast development iteration
# 
# Development Workflow:
#   1. welder run build-github-actions-staging  # Builds static ./bin/github-actions binary (Alpine/MUSL compatible)
#   2. welder docker build --push github-actions-staging  # Creates and pushes Docker image
#   3. Test with simplecontainer/github-actions:staging in your workflows
#
# This approach eliminates the need to rebuild Go dependencies in Docker for every test iteration
# Uses CGO_ENABLED=0 to build a static binary that works in Alpine (MUSL) environment
FROM alpine:latest

# Install runtime dependencies including Python (required for gcloud)
RUN apk --no-cache add ca-certificates git curl jq bash python3 py3-pip

# Install Pulumi CLI - Required for Simple Container provisioning
RUN curl -fsSL https://get.pulumi.com | sh
ENV PATH="/root/.pulumi/bin:${PATH}"

# Install Google Cloud SDK (gcloud CLI) - Required for GCP provisioning
RUN curl -sSL https://sdk.cloud.google.com | bash -s -- --disable-prompts --install-dir=/opt
ENV PATH="/opt/google-cloud-sdk/bin:${PATH}"

WORKDIR /root/

# Copy the pre-built static github-actions binary from local bin directory
# Built with: CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -a -installsuffix cgo -o bin/github-actions ./cmd/github-actions
COPY ./bin/github-actions ./github-actions

# Make sure the binary is executable
RUN chmod +x ./github-actions

# Verify installations
RUN pulumi version
RUN gcloud version

# Set the entrypoint to use the github-actions binary with absolute path
# GitHub Actions runner overrides WORKDIR with --workdir /github/workspace
# so we must use absolute path to avoid "./github-actions: no such file or directory"
ENTRYPOINT ["/root/github-actions"]

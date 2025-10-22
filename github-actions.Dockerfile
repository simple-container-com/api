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

# Final stage - minimal runtime
FROM alpine:latest

# Install runtime dependencies including Python (required for gcloud)
RUN apk --no-cache add ca-certificates git curl jq bash python3 py3-pip

# Install Pulumi CLI - Required for Simple Container provisioning
# Use explicit version to avoid auto-detection failures  
RUN curl -fsSL https://get.pulumi.com | sh -s -- --version v3.185.0
ENV PATH="/root/.pulumi/bin:${PATH}"

# Install Google Cloud SDK (gcloud CLI) - Required for GCP provisioning
RUN curl -sSL https://sdk.cloud.google.com | bash -s -- --disable-prompts --install-dir=/opt
ENV PATH="/opt/google-cloud-sdk/bin:${PATH}"

# Install GKE authentication plugin - Required for modern GKE cluster access
RUN gcloud components install gke-gcloud-auth-plugin --quiet

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/github-actions .

# Verify installations
RUN pulumi version
RUN gcloud version
RUN gcloud components list --filter="name:gke-gcloud-auth-plugin" --format="value(name)" | grep -q gke-gcloud-auth-plugin

# Set the entrypoint
ENTRYPOINT ["./github-actions"]

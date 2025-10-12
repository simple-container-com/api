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

# Install runtime dependencies
RUN apk --no-cache add ca-certificates git curl jq

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/github-actions .

# Set the entrypoint
ENTRYPOINT ["./github-actions"]

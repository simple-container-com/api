# Staging GitHub Actions Dockerfile - Uses pre-built static sc binary for fast development iteration
# 
# Development Workflow:
#   1. welder run build-github-actions-staging  # Builds static ./bin/sc binary (Alpine/MUSL compatible)
#   2. welder docker build --push github-actions-staging  # Creates and pushes Docker image
#   3. Test with simplecontainer/github-actions:github-actions-staging in your workflows
#
# This approach eliminates the need to rebuild Go dependencies in Docker for every test iteration
# Uses CGO_ENABLED=0 to build a static binary that works in Alpine (MUSL) environment
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates git curl jq

WORKDIR /root/

# Copy the pre-built static sc binary from local bin directory
# Built with: CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -a -installsuffix cgo -o bin/sc ./cmd/sc
COPY ./bin/sc ./github-actions

# Make sure the binary is executable
RUN chmod +x ./github-actions

# Set the entrypoint to use the sc binary in GitHub Actions mode
ENTRYPOINT ["./github-actions"]

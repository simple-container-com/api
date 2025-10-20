# Staging GitHub Actions Dockerfile - Uses pre-built local sc binary for fast development iteration
# 
# Development Workflow:
#   1. welder run build-github-actions-staging  # Builds ./bin/sc binary
#   2. welder docker build --push github-actions-staging  # Creates and pushes Docker image
#   3. Test with simplecontainer/github-actions:staging in your workflows
#
# This approach eliminates the need to rebuild Go dependencies in Docker for every test iteration
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates git curl jq

WORKDIR /root/

# Copy the pre-built sc binary from local bin directory
# Ensure ./bin/sc exists and is built with: go build -o bin/sc ./cmd/sc
COPY ./bin/sc ./github-actions

# Make sure the binary is executable
RUN chmod +x ./github-actions

# Set the entrypoint to use the sc binary in GitHub Actions mode
ENTRYPOINT ["./github-actions"]

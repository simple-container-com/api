# Declare version argument only once at the beginning
ARG version="2.8.4"

# Use a builder image for compiling Caddy
FROM caddy:${version}-builder AS builder

# Pass ARG version explicitly
ARG version
ENV CADDY_VERSION="${version}"

# Build Caddy with cache mounts for optimal Blacksmith performance
FROM builder AS final-builder

# Build Caddy with the required module using cache mounts
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    xcaddy build "v${CADDY_VERSION}" \
        --with github.com/grafana/certmagic-gcs@v0.1.2 && \
    caddy version

# Final runtime image
FROM caddy:${version}

# Copy the compiled Caddy binary
COPY --from=final-builder /usr/bin/caddy /usr/bin/caddy
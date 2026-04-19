# Declare version argument only once at the beginning
ARG version="2.8.4"

# Use a builder image for compiling Caddy
FROM caddy:${version}-builder AS builder

# Pass ARG version explicitly
ARG version
ENV CADDY_VERSION="${version}"

# Build Caddy with the required module using BuildKit cache mounts
# Cache mounts persist across builds on the same runner, more efficient than layer caching
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache,sharing=locked \
    xcaddy build "v${CADDY_VERSION}" \
        --with github.com/grafana/certmagic-gcs@v0.1.2 && \
    caddy version

# Final runtime image
FROM caddy:${version}

# Copy the compiled Caddy binary
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
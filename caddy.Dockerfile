# Declare version argument only once at the beginning
ARG version="2.8.4"

# Use a builder image for compiling Caddy
FROM caddy:${version}-builder AS builder

# Pass ARG version explicitly
ARG version
ENV CADDY_VERSION="${version}"

# Set up a dedicated stage for caching Go modules
FROM builder AS go-mod-cache

# Download Go modules to leverage Docker caching
WORKDIR /usr/local/go/src
RUN go mod download

# Now do the actual build, leveraging cached Go modules
FROM builder AS final-builder

# Copy cached Go modules from the previous stage
COPY --from=go-mod-cache /go/pkg /go/pkg

# Build Caddy with the required module
RUN xcaddy build "v${CADDY_VERSION}" \
        --with github.com/grafana/certmagic-gcs@v0.1.2 && \
    caddy version

# Final runtime image
FROM caddy:${version}

# Copy the compiled Caddy binary
COPY --from=final-builder /usr/bin/caddy /usr/bin/caddy
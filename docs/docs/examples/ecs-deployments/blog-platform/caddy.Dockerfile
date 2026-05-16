FROM caddy:2.11-alpine@sha256:3739ea4f0c877259a693d932693cf8f3408e9a9497c004f031b0e830e93e1546

# Copy custom Caddyfile
COPY Caddyfile /etc/caddy/Caddyfile

# Create log directory
RUN mkdir -p /var/log/caddy

# Expose ports
EXPOSE 80 443

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD caddy version || exit 1

FROM caddy:2.11.4-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648

# Copy custom Caddyfile
COPY Caddyfile /etc/caddy/Caddyfile

# Create log directory
RUN mkdir -p /var/log/caddy

# Expose ports
EXPOSE 80 443

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD caddy version || exit 1

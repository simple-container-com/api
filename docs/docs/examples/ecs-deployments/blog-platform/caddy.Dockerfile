FROM caddy:2.7-alpine

# Copy custom Caddyfile
COPY Caddyfile /etc/caddy/Caddyfile

# Create log directory
RUN mkdir -p /var/log/caddy

# Expose ports
EXPOSE 80 443

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD caddy version || exit 1

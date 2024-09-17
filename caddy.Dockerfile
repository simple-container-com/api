ARG version="2.8.4"
FROM caddy:${version}-builder AS builder

ARG version
ENV CADDY_VERSION="v${version}"
RUN set -e; \
    echo "Building caddy ${version}..." ; \
    xcaddy build "v${version}" \
        --with github.com/grafana/certmagic-gcs@v0.1.2 \
    ; \
    caddy version ; \
    echo "DONE"

ARG version
FROM caddy:${version}

COPY --from=builder /usr/bin/caddy /usr/bin/caddy
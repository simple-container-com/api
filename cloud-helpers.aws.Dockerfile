# Refresh: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
FROM public.ecr.aws/lambda/provided:al2023@sha256:777e461e02cd42bb0fdc692c0e8b056ed612f103910ecf8c5463d6fd7a92cde2

# Pull post-tag distro fixes (e.g. glibc CVE-2026-4046 once published to AL2023 dnf).
RUN dnf upgrade -y --setopt=tsflags=nodocs \
    && dnf clean all \
    && rm -rf /var/cache/dnf

WORKDIR /
COPY dist/cloud-helpers /cloud-helpers
EXPOSE 8080

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/cloud-helpers"

ENTRYPOINT ["/cloud-helpers"]

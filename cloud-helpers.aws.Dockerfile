# Refresh: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
FROM public.ecr.aws/lambda/provided:al2023@sha256:a48275a6cb21dbd9cae6f8cc10ee8ccc416e1b48f9376d049c5b347985239456

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

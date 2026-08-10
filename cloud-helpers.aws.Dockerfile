# Refresh: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
FROM public.ecr.aws/lambda/provided:al2023@sha256:5f3ae3216e07bb3677cc4dfa0c7867973f7e536abb114d6b44a7b8c558824812

# Pull post-tag distro fixes (e.g. glibc CVE-2026-4046 once published to AL2023 dnf).
RUN dnf upgrade -y --setopt=tsflags=nodocs \
    && dnf clean all \
    && rm -rf /var/cache/dnf

WORKDIR /
COPY dist/cloud-helpers /cloud-helpers
# actions/upload-artifact does not preserve the executable bit, and the release
# path (push.yaml) downloads this binary without a chmod — unlike
# branch-preview.yaml, which has one. Assert it here so the image is correct
# regardless of which workflow built it.
RUN chmod +x /cloud-helpers && test -x /cloud-helpers
EXPOSE 8080

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/cloud-helpers"

ENTRYPOINT ["/cloud-helpers"]

# Pin AWS Lambda base image by digest (CIS Docker 4.7).
# public.ecr.aws/lambda/provided:al2023 @ 2026-05-06 → resolved digest below.
# Refresh via: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
FROM public.ecr.aws/lambda/provided:al2023@sha256:a48275a6cb21dbd9cae6f8cc10ee8ccc416e1b48f9376d049c5b347985239456

# Pull post-tag glibc updates (CVE-2026-4046 was outstanding at scan time).
RUN dnf upgrade -y --setopt=tsflags=nodocs \
    && dnf clean all \
    && rm -rf /var/cache/dnf

WORKDIR /

# CIS Docker 4.9 — prefer COPY over ADD (ADD adds tar/URL semantics not needed here).
COPY dist/cloud-helpers /cloud-helpers

EXPOSE 8080

# Lambda execution environment overrides USER via the bootstrap, so USER is intentionally omitted.

ENTRYPOINT ["/cloud-helpers"]

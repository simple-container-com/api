# Refresh: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
FROM public.ecr.aws/lambda/provided:al2023@sha256:5f3ae3216e07bb3677cc4dfa0c7867973f7e536abb114d6b44a7b8c558824812

# Pull post-tag distro fixes (e.g. glibc CVE-2026-4046 once published to AL2023 dnf).
RUN dnf upgrade -y --setopt=tsflags=nodocs \
    && dnf clean all \
    && rm -rf /var/cache/dnf

# Drop the Runtime Interface Emulator. It is the base image's LOCAL-testing
# shim: /lambda-entrypoint.sh execs it only when AWS_LAMBDA_RUNTIME_API is
# unset, and the ENTRYPOINT below replaces that script with /cloud-helpers
# outright — so the RIE binary is never executed in this image, in Lambda or
# anywhere else. It is also 9 MB of Go built by AWS with an older toolchain,
# which is where every stdlib finding in this image came from (8 HIGH at the
# al2023 digest pinned above, all fixed in Go >= 1.26.6; the currently-tagged
# al2023 digest carries 30). Deleting it is the fix, not a suppression: nothing
# links it and no workflow invokes it.
#
# Local debugging is unaffected — `welder run debug-aws-cloud-helpers` runs on
# the HOST and uses the host's own `aws-lambda-rie` (welder.yaml), not this
# image's copy. What this does remove is `docker run --entrypoint
# /lambda-entrypoint.sh <image> /cloud-helpers`; use the welder task instead.
RUN rm -f /usr/local/bin/aws-lambda-rie && test ! -e /usr/local/bin/aws-lambda-rie

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

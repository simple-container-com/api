# Refresh: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
# The final stage is named `runtime` so CI can pass
# `no-cache-filters: runtime` to docker/build-push-action. Without it the
# distro-upgrade layer below is cached FOREVER: the base is digest-pinned and
# the RUN string never changes, so its cache key is permanently stable and
# `dnf upgrade` never actually executes again. `simplecontainer/github-actions:latest`
# shipped python3 3.14.5-r0 (12 HIGH) for exactly this reason while Alpine
# already served 3.14.7-r1. Note `--no-cache` on the apk line is unrelated — it
# governs apk's own index cache, not Docker layers.
# Go builder stage. The binary used to arrive as a `dist/` ARTIFACT from
# build-binaries — measured 448 s in the artifact store per ~70 MB
# incompressible binary (run 39253348066), twice per release, re-fetching
# bytes the pipeline had compiled minutes earlier. Building it here removes
# the round-trip entirely, with the exact workflow flags so the binary is the
# same build: -trimpath -buildvcs=false, CGO_ENABLED=0, same -X version stamp
# (VERSION arrives as a build-arg from push.yaml; the 0.0.0 fallback matches
# local builds). go.mod/go.sum key their own `go mod download` layer, cached
# through the gha buildx cache. The .dockerignore allowlist keeps this stage
# to cmd/pkg/internal — .sc/ (decrypted secrets post `sc secrets reveal`) and
# everything else cannot enter the context at all.
FROM golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS gobuild
ARG VERSION=0.0.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=${VERSION}" \
      -o /out/cloud-helpers ./cmd/cloud-helpers

FROM public.ecr.aws/lambda/provided:al2023@sha256:5f3ae3216e07bb3677cc4dfa0c7867973f7e536abb114d6b44a7b8c558824812 AS runtime

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
COPY --from=gobuild /out/cloud-helpers /cloud-helpers
# Historical note: this chmod existed because actions/upload-artifact does not
# preserve the executable bit on the artifact round-trip that the gobuild stage
# above has since removed. Kept as belt-and-braces: `go build` sets the bit,
# and asserting it costs nothing and still guards any future path that does not.
RUN chmod +x /cloud-helpers && test -x /cloud-helpers
EXPOSE 8080

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/cloud-helpers"

ENTRYPOINT ["/cloud-helpers"]

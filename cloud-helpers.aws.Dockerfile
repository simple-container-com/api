# Refresh: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
# The final stage is named `runtime` so CI can pass
# `no-cache-filters: runtime` to docker/build-push-action. Without it the
# distro-upgrade layer below is cached FOREVER: the base is digest-pinned and
# the RUN string never changes, so its cache key is permanently stable and
# `dnf upgrade` never actually executes again. `simplecontainer/github-actions:latest`
# shipped python3 3.14.5-r0 (12 HIGH) for exactly this reason while Alpine
# already served 3.14.7-r1. Note `--no-cache` on the apk line is unrelated — it
# governs apk's own index cache, not Docker layers.
# BLACKSMITH TRADEOFF (read before re-enabling Blacksmith).
# This stage replaced an artifact handoff: build-binaries compiled the binary
# in a parallel job and docker-build downloaded it. That download measured
# 866 s on the self-hosted fleet (run 39253348066) because the fleet's egress
# is the bottleneck — which is what justified moving the compile in here.
# On BLACKSMITH the arithmetic may invert: slots are plentiful, the artifact
# store is fast, and setup-go's cache made that parallel compile ~4.6 min,
# whereas this in-container compile cannot reuse setup-go's GOCACHE and its
# buildkit cache mounts do not survive an ephemeral runner. So if Blacksmith
# comes back, MEASURE this job before keeping it.
# Revert recipe (all of it, or the image loses its binary):
#   1. this stage out; `COPY --from=gobuild /out/<bin>` -> `COPY dist/<bin>`
#   2. re-add `!dist` to .dockerignore (the allowlist excludes it today)
#   3. push.yaml: restore the two `download-artifact` steps in docker-build
#      and the `upload-artifact` steps in build-binaries
# The conditional-stage pattern (ARG BIN_SOURCE + `FROM bin-${BIN_SOURCE}`)
# can serve both shapes from one Dockerfile if it is ever worth the
# complexity; it was judged not to be while only the fleet is in use.
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
#
# THIS DIGEST MOVES IN LOCKSTEP WITH go.mod's `go` DIRECTIVE. The official
# golang images set GOTOOLCHAIN=local, so the in-image toolchain is the ONLY
# one available — it cannot download a newer patch release the way a developer
# machine or `setup-go` would. Pin a digest whose Go is older than the `go`
# directive and every build in this stage dies on the first `go` invocation:
#
#   go: go.mod requires go >= 1.26.10 (running go 1.26.9; GOTOOLCHAIN=local)
#
# Live-caught 2026-10-22 (run 39844463896): #472 bumped the directive
# 1.26.9 -> 1.26.10 for GO-2026-6809 and this digest stayed on 1.26.9, so both
# in-image build legs (here and github-actions.Dockerfile, which carries an
# identical pin) failed while every other job stayed green — the rest of CI
# reads `go-version-file: go.mod` and self-updates. `Docker finalize` is
# `needs:` both legs, so it skipped and the release published NOTHING.
#
# When you bump the directive, re-resolve the digest and bump it here AND in
# github-actions.Dockerfile:
#   docker buildx imagetools inspect golang:1.26-alpine   # -> Digest:
# then confirm that digest is the intended patch release:
#   curl -s 'https://hub.docker.com/v2/repositories/library/golang/tags?name=<x.y.z>-alpine'
FROM golang:1.27-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS gobuild
ARG VERSION=0.0.0
WORKDIR /src
COPY go.mod go.sum ./
# Same cache mount as the build below, deliberately: GOMODCACHE lives at
# /go/pkg/mod, so without the mount here this layer would populate the image
# filesystem while the build step reads the (empty) cache mount instead —
# downloading every module twice. With it, both steps share one store.
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# Cache mounts, not a bare RUN: without them ANY source change invalidates
# `COPY . .` and recompiles the whole ~2095-package closure inside the
# container on every build.
#
# Whether they actually help depends ENTIRELY on the builder surviving, which
# is a push.yaml property, not a Dockerfile one: buildkit does NOT export
# `type=cache` mount contents through any cache exporter — not `type=gha`, not
# `type=registry`. Mount contents are builder-local state. So these mounts pay
# off only where the buildkit instance persists, which on the fleet is what
# `setup-buildx-action`'s `name` + `cleanup: false` + `keep-state: true` buy
# (added 2026-10-20 — before that the builder was torn down after every job
# and this stage compiled from cold every release, despite an earlier version
# of this comment asserting otherwise).
#
# On an EPHEMERAL runner (Blacksmith / ubuntu-latest) the VM is discarded, so
# these mounts start empty every run regardless — see the BLACKSMITH TRADEOFF
# note above the gobuild stage.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -buildvcs=false \
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

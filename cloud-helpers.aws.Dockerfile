# Refresh: docker buildx imagetools inspect public.ecr.aws/lambda/provided:al2023
# The final stage is named `runtime` so CI can pass
# `no-cache-filters: runtime` to docker/build-push-action. Without it the
# distro-upgrade layer below is cached FOREVER: the base is digest-pinned and
# the RUN string never changes, so its cache key is permanently stable and
# `dnf upgrade` never actually executes again. `simplecontainer/github-actions:latest`
# shipped python3 3.14.5-r0 (12 HIGH) for exactly this reason while Alpine
# already served 3.14.7-r1. Note `--no-cache` on the apk line is unrelated — it
# governs apk's own index cache, not Docker layers.
# The Go binary is compiled by the CALLER, natively, and arrives in the build
# context at dist/cloud-helpers — see the long rationale on the `COPY
# dist/cloud-helpers` line below. An in-image `gobuild` stage lived here until
# 2026-10-30; removing it also removes the `golang:1.26-alpine` digest pin
# that had to move in LOCKSTEP with go.mod's `go` directive (the official
# images set GOTOOLCHAIN=local, so a stale digest failed every `go`
# invocation). That trap published NOTHING on release run 39844463896
# (2026-10-22) and is now structurally gone from both image Dockerfiles.
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
# The binary is compiled by the CALLER, natively, and arrives in the build
# context at dist/cloud-helpers. Every caller already did this except
# push.yaml: welder's `build-cloud-helpers` task writes exactly this path
# (welder.yaml:118-123) and runs before `dockerImages`, and
# branch-preview.yaml downloads the build-binaries artifact into `dist/`.
# Both of them were having that binary thrown away by an in-image `gobuild`
# stage that recompiled the same ~2095-package closure.
#
# Why not compile in-image (the shape here until 2026-10-30): buildkit does
# not export `--mount=type=cache` contents through ANY cache exporter, so an
# in-image compile can only be warm if the buildkit INSTANCE survives. On the
# fleet it often does not — `/cache/docker` is wiped wholesale by
# guest-cache-gc at >=88% and nothing reseeds it — and on an ephemeral runner
# it never does. Measured across six releases, the same closure under the same
# flags: 760-1812 s in-image on 10 of 12 legs, against 141-310 s natively on
# EVERY run, because the guest's GOCACHE (/cache/go-build) is seeded by
# reflink from a golden drive whose bake explicitly covers this namespace
# (forgeci-cache-bake, "[B] linux/amd64 -trimpath CGO=0 (binaries + docker
# gobuild)"). The in-image compile was the one consumer in the fleet that
# structurally could not read that warmth.
#
# This is NOT the artifact round-trip that the gobuild stage replaced. That
# shape uploaded the binary from a separate job and downloaded it here —
# measured 448 s in the artifact store per ~70 MB incompressible binary, and
# 866 s on the fleet (run 39253348066), because fleet egress is the
# bottleneck. The compile now happens in THIS job, so there is no artifact
# store and no egress.
#
# The flags must stay identical in all three callers, or the shipped binary
# stops being the build the release attests: -trimpath -buildvcs=false,
# CGO_ENABLED=0, -ldflags "-s -w -X=.../internal/build.Version=<version>".
COPY dist/cloud-helpers /cloud-helpers
# chmod is belt-and-braces: `go build` sets the bit and the build context
# preserves it, but branch-preview's path goes through actions/upload-artifact,
# which does not.
RUN chmod +x /cloud-helpers && test -x /cloud-helpers
EXPOSE 8080

LABEL org.opencontainers.image.source="https://github.com/simple-container-com/api" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="simplecontainer/cloud-helpers"

ENTRYPOINT ["/cloud-helpers"]

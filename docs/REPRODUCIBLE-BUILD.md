# Reproducible builds

The release binaries (`sc`, `cloud-helpers`, `github-actions`) are built
deterministically: building the **same source at the same version** on a
clean machine yields a **bit-for-bit identical** binary. This lets anyone
independently rebuild a release and confirm the published artifact was
built from the published source.

## What makes the build reproducible

| Input | How it is pinned | Where |
|---|---|---|
| **Go toolchain** | exact patch version pinned by the `go` directive; `GOTOOLCHAIN` resolves it deterministically | [`go.mod`](../go.mod) |
| **Filesystem paths** | `-trimpath` strips the build machine's `$GOPATH`/module paths from the binary | [`welder.yaml`](../welder.yaml) `build`, `build-cloud-helpers`, `build-github-actions`, `build-github-actions-staging` |
| **C toolchain variance** | `CGO_ENABLED=0` — pure-Go static build, no host libc/linker | [`welder.yaml`](../welder.yaml) `build`, `build-github-actions-staging` |
| **Embedded version string** | `-ldflags "-s -w -X .../internal/build.Version=$VERSION"` — symbol table stripped, version supplied explicitly (not derived from wall-clock) | [`welder.yaml`](../welder.yaml) `default.build.args.ld-flags` |
| **AI-assistant embeddings** | committed to the repo (`pkg/assistant/embeddings/vectors/`) and consumed via `go:embed`; the build never calls an external LLM | repo tree |
| **Dependencies** | module graph pinned by `go.sum`; Docker base images + GitHub Actions are SHA-pinned | `go.sum`, Dockerfiles, `.github/` |

The only intentional build input is `VERSION` (the CalVer tag). Two
builds of the same commit with the same `VERSION` are identical; the
version is passed in explicitly rather than read from the clock or the
build host, so it does not introduce nondeterminism.

## Verifying reproducibility

Build the `sc` binary twice from a clean checkout and compare digests:

```sh
git checkout <released-tag>            # e.g. v2026.6.20
VERSION=<released-tag> welder run build -a os=linux -a arch=amd64
sha256sum dist/linux-amd64/sc

# rebuild from a fresh clone and compare
git clean -fdx && \
VERSION=<released-tag> welder run build -a os=linux -a arch=amd64
sha256sum dist/linux-amd64/sc        # identical to the first
```

Or directly with the Go toolchain (no welder):

```sh
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=<tag>" \
  -o sc ./cmd/sc
sha256sum sc
```

To confirm a **published** release matches the source, rebuild the tag
as above and compare the digest against the release's
`cosign verify-blob` / SLSA provenance subject digest (see
[`SECURITY.md`](SECURITY.md) → "Verifying tarballs").

## Known boundaries

- The **Docker images** wrap the reproducible binaries; image-layer
  reproducibility additionally depends on the SHA-pinned base images and
  the deterministic `welder docker build` context.
- Reproducibility is asserted for the release toolchain (Linux/macOS,
  amd64/arm64) targeted by the `build` task; cross-builds use the same
  flags per `GOOS`/`GOARCH`.

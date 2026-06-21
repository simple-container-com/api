# Reproducible builds

The release binaries (`sc`, `cloud-helpers`, `github-actions`) and the
published `sc-<os>-<arch>.tar.gz` archives are built deterministically:
rebuilding the **same source at the same version** on a clean machine
yields a **bit-for-bit identical** binary and archive. This lets anyone
independently rebuild a release and confirm the published artifact (and
its `cosign` / SLSA digest) was built from the published source.

## What makes the build reproducible

| Input | How it is pinned | Where |
|---|---|---|
| **Go toolchain** | exact patch version pinned by the `go` directive; `GOTOOLCHAIN` resolves it deterministically | [`go.mod`](../go.mod) |
| **Filesystem paths** | `-trimpath` strips the build machine's `$GOPATH`/module paths from the binary | release jobs in [`.github/workflows/push.yaml`](../.github/workflows/push.yaml) (`build sc`, `build <target>`) and the [`welder.yaml`](../welder.yaml) `build` / `build-cloud-helpers` / `build-github-actions` tasks |
| **VCS stamping** | `-buildvcs=false` — Go would otherwise embed `vcs.revision`/`vcs.time`/`vcs.modified` into the binary (which `-trimpath` does **not** strip), so a rebuild from a source tarball without `.git`, or from a tree with any local modification, would diverge | every release build (same `go build` invocations as above) |
| **C toolchain variance** | `CGO_ENABLED=0` — pure-Go static build, no host libc/linker dependency | every release build: `push.yaml` `build sc` + `build <target>`; `welder.yaml` `build`, `build-cloud-helpers`, `build-github-actions` |
| **Embedded version string** | `-ldflags "-s -w -X .../internal/build.Version=$VERSION"` — symbol table stripped, version supplied explicitly (not derived from the wall-clock) | `push.yaml` + `welder.yaml` `default.build.args.ld-flags` |
| **Archive metadata** | `tar --sort=name --mtime='@0' --owner=0 --group=0 --numeric-owner --mode='u=rwx,go=rx' \| gzip -9 -n` — fixed entry order, zeroed timestamps/ownership, a forced `0755` mode (independent of the builder's umask), no gzip timestamp | `push.yaml` `build sc`, `welder.yaml` `build` |
| **AI-assistant embeddings** | committed to the repo (`pkg/assistant/embeddings/vectors/`) and consumed via `go:embed`; the build never calls an external LLM | repo tree |
| **Dependencies** | module graph pinned by `go.sum`; Docker base images + GitHub Actions are SHA-pinned | `go.sum`, Dockerfiles, `.github/` |

The only intentional build input is `VERSION` — the **bare** CalVer
value (e.g. `2026.6.20`, no `v` prefix) that the release embeds into
`internal/build.Version`. Two builds of the same commit with the same
`VERSION` are identical; the version is passed in explicitly rather than
read from the clock or the build host, so it introduces no nondeterminism.

## Verifying reproducibility

Use the **bare** CalVer the release embedded (the `v` prefix only ever
appears in the `-v<version>` copy of the tarball filename, never in
`build.Version`). Build twice from a clean checkout and compare:

```sh
git checkout <released-tag>
VERSION=2026.6.20 welder run build -a os=linux -a arch=amd64   # bare version
sha256sum .sc/stacks/dist/bundle/sc-linux-amd64.tar.gz

git clean -fdx
VERSION=2026.6.20 welder run build -a os=linux -a arch=amd64
sha256sum .sc/stacks/dist/bundle/sc-linux-amd64.tar.gz         # identical digest
```

Or just the binary, directly with the Go toolchain (matches the flags the
release uses):

```sh
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=2026.6.20" \
  -o sc ./cmd/sc
sha256sum sc
```

`-buildvcs=false` is required: without it `go build` either embeds the
local VCS state (diverging across checkouts) or, when run from a source
tarball with no `.git`, fails with `error obtaining VCS status`.

To confirm a **published** release matches the source, rebuild the tag as
above and compare the rebuilt `sc-<os>-<arch>.tar.gz` digest (or the inner
binary's digest) against the release's `cosign verify-blob --bundle` /
SLSA provenance subject digest — see [`SECURITY.md`](SECURITY.md) →
"Verifying tarballs".

## Known boundaries

- The **Docker images** wrap the reproducible binaries; image-layer
  reproducibility additionally depends on the SHA-pinned base images and
  the deterministic `welder docker build` context.
- Reproducibility is asserted for the release toolchain (Linux/macOS,
  amd64/arm64); each cross-build target uses the same flags per
  `GOOS`/`GOARCH`.
- The deterministic archive step relies on GNU `tar` options
  (`--sort`, `--mtime`, `--owner`/`--group`, `--mode`). Release archives
  are produced on the Linux CI runners (GNU tar); to reproduce the
  archive byte-for-byte on macOS use GNU tar (`gtar`) rather than the
  bundled BSD `tar`. The binary itself reproduces with either.

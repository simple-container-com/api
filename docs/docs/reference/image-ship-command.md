# Shipping an Image Without a Registry (`sc image ship`)

`sc image ship` copies a locally built image straight into a machine's docker, over
SSH, with no registry in between.

It exists for deployments where a registry is the wrong dependency: one virtual
machine running `docker compose`, an air-gapped or geo-restricted environment where
Docker Hub is unreachable, or a CI job that has just built an image and wants it on a
host without first publishing it anywhere.

## How it works

```
docker save (local daemon, via the API)  →  compress  →  ssh  →  docker load (remote)
```

The image is a single stream: nothing is written to disk on either side, no temporary
tarball, and **no docker CLI is needed on the machine running `sc`** — it talks to the
local daemon over the docker API (`DOCKER_HOST`). The machine being shipped to needs
only `docker` and `sshd`.

## Usage

```bash
# The machine's host key is already in ~/.ssh/known_hosts
sc image ship --image ghcr.io/myorg/myapp:v1.0.0 --host 203.0.113.10 --ssh-key ./deploy_key

# CI: a freshly created machine is in nobody's known_hosts, so pin its key
sc image ship --image ghcr.io/myorg/myapp:v1.0.0 --host 203.0.113.10 \
  --ssh-key ./deploy_key --host-key "$(ssh-keyscan -t ed25519 203.0.113.10)"
```

**Flags:**

- `--image` (required): the local image reference to ship
- `--host` (required): the machine's address
- `--user`: SSH user (default `sc`)
- `--port`: SSH port (default `22`)
- `--ssh-key`: path to the SSH private key (default `$SC_SSH_KEY`). Password and
  ssh-agent authentication are not supported
- `--ssh-key-password`: passphrase for an encrypted key (default
  `$SC_SSH_KEY_PASSWORD`)
- `--host-key`: pin the machine's public key, as an `authorized_keys` line
- `--known-hosts`: file to verify against (default `~/.ssh/known_hosts`)
- `--insecure-skip-host-key-check`: accept any host key
- `--compression`: `zstd` (default), `gzip` or `none`
- `--force`: ship even when the machine already holds this image id
- `--allow-arch-mismatch`: ship an image built for another architecture
- `--timeout`: timeout for the whole transfer (default `30m`)

## Three behaviours worth knowing

**It is idempotent, by image id.** Before transferring anything, `ship` asks the
machine which image the reference resolves to. If that is already the id you are
shipping, nothing moves — so re-running a deploy of an unchanged image costs one SSH
round trip rather than several hundred megabytes. `--force` overrides it.

**It refuses an architecture mismatch.** `docker load` accepts an image built for a
different architecture without complaint, and the failure then surfaces much later as
a container exiting with `exec format error` — far from the deploy that caused it. So
`ship` compares the image's platform (including the ARM variant) with what the remote
daemon reports and refuses up front. `--allow-arch-mismatch` downgrades it to a
warning.

**It verifies the machine's identity by default.** Without a host key, SSH
authenticates the channel but nothing authenticates the *machine*: the deploy key and
every shipped byte go to whoever answered on that address. `ship` therefore verifies
against `known_hosts`, accepts a pinned `--host-key` (what CI should use), and makes
skipping the check an explicit `--insecure-skip-host-key-check` that logs a warning.

## Compression

`zstd` is the default — roughly three times faster than gzip at a better ratio, and
on a large image over a long link the compression is what makes the transfer bearable.
No zstd binary is needed on the machine: `docker load` detects the compression from
the stream itself.

Use `--compression gzip` for a docker engine older than 23, whose load path predates
zstd support; it reports the problem as an opaque `unexpected EOF` rather than naming
the compression, which is the reason the escape hatch exists.

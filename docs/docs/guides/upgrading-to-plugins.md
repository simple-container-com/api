---
title: Upgrading to SC plugins
description: Cloudflare and Yandex Cloud support moved out of the sc binary into separate plugin binaries. This page is everything you need to do about it.
platform: platform
product: simple-container
category: devguide
subcategory: upgrade
date: '2026-10-27'
---

# Upgrading to SC plugins

Some cloud integrations no longer ship inside the `sc` binary. They are now
**plugins** — separate, independently released executables that `sc` downloads
and talks to over a local gRPC bus. Nothing about your `client.yaml` changes.
What changes is that a `server.yaml` which uses a plugin-owned type must now
say **which plugin release** to use.

!!! info "Do you need to do anything?"
    Only if a `server.yaml` of yours uses one of the type tokens in the table
    below. Run `sc version` first: **`2026.10.32` and older still have
    Cloudflare and Yandex built in** and need no changes. The split takes
    effect in the first release that links to this page.

---

## 1. Check whether you are affected

Grep your stacks for the type tokens that moved out of tree:

```bash
grep -rnE 'type:[[:space:]]*(cloudflare|yandex|yc-dns|yc-bucket|yc-serverless-container|yc-object-storage|yc-kms|yc-lockbox|yc-service-account)' .sc/stacks/
```

| Plugin | Repository | Type tokens it now owns |
|---|---|---|
| **`cloudflare`** | [`simple-container-com/sc-plugin-cloudflare`](https://github.com/simple-container-com/sc-plugin-cloudflare) | `cloudflare` (DNS registrar) |
| **`yandex`** | [`simple-container-com/sc-plugin-yandex`](https://github.com/simple-container-com/sc-plugin-yandex) | `yandex` (provider), `yc-dns`, `yc-bucket`, `yc-serverless-container`, `yc-object-storage`, `yc-kms`, `yc-lockbox`, `yc-service-account` |

Everything else — AWS, GCP, Kubernetes, Cloud.ru, MongoDB, `k3s`, `vm` — is
still built into `sc`. No action needed for those.

No hits? You are done; stop reading.

---

## 2. What it looks like if you skip this

`sc` refuses the stack at parse time, before any state is touched, and prints
the block you need:

```
server.yaml uses type tokens owned by out-of-tree SC plugins but is missing
the corresponding `plugins:` entries. Add the block(s) below to this stack's
`server.yaml` (or inherit from the parent stack):

plugins:
  cloudflare:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-cloudflare
      version: v<pick-a-release-tag>
    # Owns these types used in this stack: resources.registrar.type
```

That message is the authoritative list for *your* stack — it names the exact
YAML paths that triggered each plugin. Copy it, fill in a version, done.

---

## 3. Add the `plugins:` block

Put it in the **parent stack's** `server.yaml`, as a top-level key (same level
as `provisioner:`, `templates:`, `resources:`). Child stacks inherit it
automatically — do not repeat it per child.

```yaml
schemaVersion: 1.0
provisioner:
  type: pulumi
  # ...

plugins:
  cloudflare:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-cloudflare
      version: v2026.10.26-5eddde3
  yandex:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-yandex
      version: v2026.10.26-36a86d2

resources:
  registrar:
    type: cloudflare          # ← owned by the `cloudflare` plugin above
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      # ... unchanged
```

**The map key is the plugin name** (`cloudflare`, `yandex`) exactly as the
error message spells it — not the repository, not the type token.

### Pick a version

```bash
gh release list --repo simple-container-com/sc-plugin-cloudflare --limit 5
gh release list --repo simple-container-com/sc-plugin-yandex --limit 5
```

or browse
[sc-plugin-cloudflare/releases](https://github.com/simple-container-com/sc-plugin-cloudflare/releases)
and
[sc-plugin-yandex/releases](https://github.com/simple-container-com/sc-plugin-yandex/releases).

!!! warning "`latest` is not a version"
    `sc` deliberately does **not** resolve `latest`. A deploy must be
    reproducible, so you pin a tag and bump it on purpose. `version: latest`
    fails with
    `plugin source.version is empty (SC does not auto-resolve "latest" — pin a specific release)`.

### Then deploy as usual

```bash
sc provision -s <parent-stack> -e <env>   # only if you were going to anyway
sc deploy -s <stack> -e <env>
```

Adding the block does not change any provisioned resource, so **no
re-provision is required** just to adopt it — the block is read from
`server.yaml` on every command. Run `sc preview` first if you want to see
that for yourself.

---

## 4. Verify it worked

A loaded plugin announces itself on stdout:

```
pre-engine loaded plugin "cloudflare" v2026.10.26-5eddde3 state_store=[] secrets_provider=[]
```

**No such line means no plugin was loaded**, whatever else the output says.
That is the single most useful thing to grep for when a plugin-owned type
behaves as if it does not exist.

The binary is cached under `~/.sc/plugins/<owner>/<repo>/<version>/`:

```bash
ls ~/.sc/plugins/simple-container-com/sc-plugin-cloudflare/v2026.10.26-5eddde3/
# sc-plugin-cloudflare-linux-amd64
```

---

## 5. Integrity pinning (recommended)

By default `sc` fetches `sha256sums.txt` from the release and verifies the
downloaded binary against it. To pin the hash in your own repo instead — so a
re-tagged release cannot change what runs — add `sha256`:

```yaml
plugins:
  cloudflare:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-cloudflare
      version: v2026.10.26-5eddde3
    sha256: 9f2b...   # for YOUR platform's binary
```

Get the value for the platform `sc` runs on:

```bash
curl -sL https://github.com/simple-container-com/sc-plugin-cloudflare/releases/download/v2026.10.26-5eddde3/sha256sums.txt \
  | grep "sc-plugin-cloudflare-linux-amd64"
```

A mismatch is fatal and names both hashes. When neither `sha256` nor a
`sha256sums.txt` asset is available, `sc` logs a warning and proceeds — treat
that warning as an error in production.

---

## 6. Air-gapped, vendored, or local development

Point at a binary on disk instead of a release. `path` wins over `source`:

```yaml
plugins:
  cloudflare:
    path: /opt/sc/plugins/sc-plugin-cloudflare   # ~ is expanded
```

Useful for restricted networks (download once, ship it with your image),
for vendoring a plugin into your own repo, and for developing a plugin
locally against a real stack.

To pre-seed the cache instead of changing the YAML, drop the binary at
`~/.sc/plugins/<owner>/<repo>/<version>/<binary-name>` — `sc` uses a cached
file whose hash already matches and makes no network call. The binary name is
`<repo-leaf>-<goos>-<goarch>`, e.g. `sc-plugin-yandex-linux-amd64`.

---

## 7. Things to know before you hit them

**Published platforms are `linux/amd64`, `linux/arm64`, and `darwin/arm64`.**
There is no `darwin/amd64` (Intel Mac) or Windows asset today. On an
unsupported platform the fetch fails with a 404 on the asset URL; build the
plugin from source and use `path:`.

**The fetch is an unauthenticated GitHub download.** Both plugins are public,
so this is fine — but a private plugin repository will not work through
`source:` today. Use `path:`.

**CI runners need egress to `github.com`.** The first run on a fresh runner
downloads each plugin (a few tens of MB). Cache `~/.sc/plugins` keyed on the
pinned versions if that cost matters, or vendor via `path:`.

**Do not install two plugins that claim the same type token.** For state-store
and secrets-provider types `sc` refuses to start
(`plugins "a" and "b" both claim state-store type "x"`). For provider,
resource and registrar types registration is **last-write-wins and silent** —
including over a type built into `sc`. Keep one plugin per token.

**The binary filename can be overridden.** If you publish your own plugin
under a different convention, set `source.binaryName` — `{goos}` and
`{goarch}` are substituted:

```yaml
plugins:
  mycloud:
    source:
      type: github-release
      repository: me/sc-plugin-mycloud
      version: v1.0.0
      binaryName: "plugin_{goos}_{goarch}"
```

---

## 8. Troubleshooting

| What you see | What it means | Fix |
|---|---|---|
| `server.yaml uses type tokens owned by out-of-tree SC plugins…` | No `plugins:` block for a type you use | Add the block it printed (§3) |
| `unsupported registrar type "cloudflare" for stack "<stack>"` **and no `pre-engine loaded plugin` line** | The plugin never loaded, so nothing claimed the type | Check the block is on the **parent** stack and that `sc` is new enough (§4) |
| `plugin source.version is empty (SC does not auto-resolve "latest"…)` | `version:` missing or set to `latest` | Pin a real tag |
| `download "https://github.com/…": 404` | That version has no asset for your OS/arch | Check the tag exists and §7's platform list |
| `plugin SHA256 mismatch: want …, got …` | Pinned hash ≠ downloaded binary | Re-copy the hash **for your platform** from `sha256sums.txt` |
| `plugins "a" and "b" both claim state-store type "x"` | Two plugins own the same token | Remove one |
| `plugin spec has neither path nor source` | A `plugins:` entry is empty | Give it `source:` or `path:` |
| `unknown plugin source type "oci"` | Only `github-release` is supported in v1 | Use `github-release` or `path:` |

---

## 9. Rolling back

Pinning an older plugin release is a one-line change — bump `version:` back
and redeploy. Pinning an older `sc` is the other direction: `sc` `2026.10.32`
and older still have Cloudflare and Yandex compiled in and ignore the
`plugins:` block, so a `server.yaml` carrying the block stays compatible with
both. **Add the block before you upgrade `sc`, not after** — that ordering
makes the upgrade a no-op rather than a flag day.

---

## Why this changed

Keeping every cloud's Pulumi SDK inside one binary made `sc` slow to build,
large to ship, and impossible to release independently per cloud: a Cloudflare
fix waited on an unrelated AWS review. Plugins decouple those release
cadences and let a cloud be added without touching SC core. The full rationale
and the plugin-author contract live in the `sc-plugin-architecture` design
docs.

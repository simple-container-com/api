---
title: Upgrading to SC plugins
description: Cloudflare, Yandex Cloud, Cloud.ru, MongoDB Atlas and AWS support moved out of the sc binary into separate plugin binaries. This page is everything you need to do about it.
platform: platform
product: simple-container
category: devguide
subcategory: upgrade
date: '2026-10-30'
---

# Upgrading to SC plugins

Some cloud integrations no longer ship inside the `sc` binary. They are now
**plugins** — separate, independently released executables that `sc` downloads
and talks to over a local gRPC bus. Nothing about your `client.yaml` changes.
What changes is that a `server.yaml` using a plugin-owned type should now say
**which plugin release** to use.

!!! info "Nothing is required of you — but one thing is recommended"
    The upgrade is **not** a flag day. A `server.yaml` that predates the split
    keeps deploying: `sc` resolves the plugin's latest release for you (§2).
    What we recommend is pinning that release explicitly (§3), so your deploys
    stay reproducible and make no network call. If no `server.yaml` of yours
    uses the type tokens below, there is nothing to read here at all.

    `sc version` **`2026.10.32` and older** still have Cloudflare and Yandex
    built in and ignore all of this.

---

## 1. Check whether you are affected

Grep your stacks for the type tokens that moved out of tree:

```bash
grep -rnE 'type:[[:space:]]*(cloudflare|yandex|yc-dns|yc-bucket|yc-serverless-container|yc-object-storage|yc-kms|yc-lockbox|yc-service-account|cloudru|cloudru-container|cloudru-postgres|mongodb-atlas|aws|aws-token|aws-lambda|aws-static-website|ecs-fargate|s3-bucket|ecr-repository|aws-rds-postgres|aws-rds-mysql|aws-cloudtrail-security-alerts|aws-kms)' .sc/stacks/
```

| Plugin | Repository | Type tokens it now owns |
|---|---|---|
| **`cloudflare`** | [`simple-container-com/sc-plugin-cloudflare`](https://github.com/simple-container-com/sc-plugin-cloudflare) | `cloudflare` (DNS registrar) |
| **`yandex`** | [`simple-container-com/sc-plugin-yandex`](https://github.com/simple-container-com/sc-plugin-yandex) | `yandex` (provider), `yc-dns`, `yc-bucket`, `yc-serverless-container`, `yc-object-storage`, `yc-kms`, `yc-lockbox`, `yc-service-account` |
| **`cloudru`** | [`simple-container-com/sc-plugin-cloudru`](https://github.com/simple-container-com/sc-plugin-cloudru) | `cloudru` (provider), `cloudru-container`, `cloudru-postgres` |
| **`mongodb`** | [`simple-container-com/sc-plugin-mongodb`](https://github.com/simple-container-com/sc-plugin-mongodb) | `mongodb-atlas` (provider + ComputeProcessor for the `uses:` env fan-out) |
| **`aws`** | [`simple-container-com/sc-plugin-aws`](https://github.com/simple-container-com/sc-plugin-aws) | `aws` (provider), `aws-token`, state-store `s3-bucket`, secrets-provider `aws-kms`, templates `aws-lambda` / `ecs-fargate` / `aws-static-website`, resources `s3-bucket` / `ecr-repository` / `aws-rds-postgres` / `aws-rds-mysql` / `aws-cloudtrail-security-alerts`, and the `cloud-helpers` Lambda for CloudWatch alerts + health events |

Everything else — GCP, Kubernetes, `k3s`, `vm` — is still built into `sc`. No
action needed for those.

No hits? You are done; stop reading.

---

## 2. If you do nothing, the upgrade still works

**Your existing `server.yaml` keeps deploying.** When `sc` parses a stack that
uses a plugin-owned type without a `plugins:` entry, it resolves that plugin's
**latest release** from the GitHub Releases API and injects the pin for you,
at parse time, before the Pulumi engine starts. No edit required.

That is a deliberate backwards-compatibility guarantee, not a fallback: an
upgrade must *work*, not merely diagnose.

The cost of relying on it:

- **One HTTPS call per undeclared plugin per `sc` invocation** (10 s timeout,
  uses `GITHUB_TOKEN` for the 5000/h rate budget if one is in the environment,
  otherwise the unauthenticated 60/h).
- **Your deploys stop being reproducible.** Whatever is "latest" the moment you
  run `sc` is what runs. A plugin release between two deploys changes your
  behaviour with no change in your repo.
- **It needs egress to `api.github.com`** from wherever `sc` runs, including CI.

`sc` only errors when resolution itself fails — offline, rate-limited, repo
gone — and then it prints the exact block to paste:

```
server.yaml uses type tokens owned by out-of-tree SC plugins that are not
declared under `plugins:` and whose latest release could not be resolved
from GitHub. Reasons:
  - cloudflare: GET https://api.github.com/repos/.../releases/latest: 503 Service Unavailable

Paste the block(s) below into this stack's `server.yaml` (or inherit from the
parent stack):

plugins:
  cloudflare:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-cloudflare
      version: v<pick-a-release-tag>
    # Owns these types used in this stack: resources.registrar.type
```

That message is the authoritative list for *your* stack — it names the exact
YAML paths that triggered each plugin.

!!! tip "Do §3 anyway"
    Auto-injection exists so the upgrade cannot break you. Pinning exists so
    your deploys stay reproducible and make no network call. Pin.

---

## 3. Pin the `plugins:` block (recommended)

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
  cloudru:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-cloudru
      version: v2026.10.27-7dc7ca9
  mongodb:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-mongodb
      version: v2026.10.29-c0b4733
  aws:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-aws
      version: v2026.10.30-b021ccb

resources:
  registrar:
    type: cloudflare          # ← owned by the `cloudflare` plugin above
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      # ... unchanged
```

**The map key is the plugin name** (`cloudflare`, `yandex`, `cloudru`,
`mongodb`, `aws`) exactly as the error message spells it — not the repository,
not the type token.

### Pick a version

```bash
gh release list --repo simple-container-com/sc-plugin-cloudflare --limit 5
gh release list --repo simple-container-com/sc-plugin-yandex --limit 5
```

or browse
[sc-plugin-cloudflare/releases](https://github.com/simple-container-com/sc-plugin-cloudflare/releases)
and
[sc-plugin-yandex/releases](https://github.com/simple-container-com/sc-plugin-yandex/releases).

!!! warning "Write a real tag, not the word `latest`"
    Once you declare a plugin explicitly, `sc` uses exactly what you wrote and
    does no resolution — so `version:` must be a real tag. An empty `version:`
    fails with
    `plugin source.version is empty (SC does not auto-resolve "latest" — pin a specific release)`,
    and the literal string `latest` becomes a 404 on the download URL. If you
    *want* latest-tracking, omit the whole entry and let §2's auto-injection
    do it.

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

## 3b. Pinning the plugins is only half of it — the CLI floats too

The `plugins:` pins above fix which plugin *binaries* run. They say nothing
about which `sc` runs, and by default that is whatever is newest.

`simple-container-com/api/.github/actions/deploy-client-stack` has a
`version` input, but it is **"Version to deploy"** — the artifact CalVer,
mapped to env `VERSION`, which `sc deploy --deploy-version` reads and SC
injects as a Docker build arg. The action has **no** SC-CLI-version input.
The CLI comes from the `simplecontainer/github-actions` image tag,
resolved as `latest`.

So a workflow line like

```yaml
# Pin SC CLI to 2026.10.7 — the first release with the Fargate SG UDP egress fix…
version: "${{ github.event.inputs.sc_version || '2026.10.7' }}"
```

does neither of the things it looks like: the CLI still floats, and every
deployed image is stamped `VERSION=2026.10.7` instead of its
`YYYY.MM.DD-<sha>` CalVer — so the running binary can no longer tell you
which build it is.

To learn which CLI actually ran, grep the job log for
`Simple Container CLI version:` — never infer it from the workflow input.
It prints twice: the requested tag, then the resolved number. forge-baas
across 2026-10-29 and 10-30, under an identical workflow input:

| run | CLI logged | result |
|---|---|---|
| 40953533956 (10-29 20:57) | `latest` → 2026.10.45 | red (nil-pointer panic) |
| 41101319935 (10-30 17:45) | `latest` → 2026.10.50 | red (both stacks) |
| 41113107084 (10-30 19:10) | `latest` → 2026.10.52 | green |

**That sequence shows the CLI float, but it does NOT isolate the CLI as
the cause of the outcome change** — until the plugins block in the parent
`server.yaml` grew an explicit `aws:` entry (which happened at the end of
10-30), `sc` auto-latest-resolved the aws plugin as well, and the plugin
cut three releases inside this very window. Two moving halves. The lesson
is the unpinnedness itself: three outcomes across three CLI versions and
an unknown number of plugin versions in two days, under one unchanged
workflow input. If you want a different outcome than yesterday from the
same input, pin **both** halves — this section covers the CLI; §3's
`plugins:` block covers the plugins.

!!! warning "Don't confuse `Simple Container CLI version:` with `Deploy version:`"
    The same log also prints `Deploy version:` (e.g. `2026.10.7` on
    10-30). That is the **artifact CalVer** from `$VERSION`, which SC
    stamps onto the deployed image as a Docker build arg — a third,
    unrelated version from the CLI and the plugins. If you grep for
    the wrong one, you'll pin something that doesn't move the CLI at
    all.

**Pin it at the image tag if you want reproducible deploys.** The pin is
in the workflow, not in `server.yaml`:

```yaml
# .github/workflows/deploy.yaml
jobs:
  deploy:
    container:
      image: simplecontainer/github-actions:2026.10.45
    steps:
      - uses: simple-container-com/api/.github/actions/deploy-client-stack@v2026.10.45
```

!!! warning "The plugins pin different api refs. Cross-plugin skew is the normal state."
    Every plugin and the CLI share the `api` module across a gRPC bus, but
    the plugins are released independently and almost never pin the same
    `api` SHA. As of 2026-10-30 the five live plugins pinned five different
    `api` refs spanning four days:

    | plugin | api ref in `plugin/go.mod` |
    |---|---|
    | aws | `v0.0.0-20261030131545-7d88543d7487` (10-30 13:15) |
    | yandex | `v0.0.0-20261029151018-e0ac743c03f5` (10-29 15:10) |
    | mongodb | `v0.0.0-20261029105927-d49ee7927310` (10-29 10:59) |
    | cloudflare | `v0.0.0-20261028062016-731821e9ee10` (10-28 06:20) |
    | cloudru | `v0.0.0-20261027182937-685d153ee7de` (10-27 18:29) |

    So you cannot "match the CLI to the plugin's api ref" even in
    principle — the plugins don't agree with each other, let alone with
    core. The bus runs multiple api versions simultaneously by design;
    no tested combination exists. What makes a deploy reproducible is
    pinning both halves — `plugins:` in `server.yaml` AND the GitHub
    Actions image tag — not matching SHAs.

    Each plugin's `api` ref lives in `plugin/go.mod` (NOT the root
    `go.mod`, which has no api dep). If you need to know which api a
    particular plugin release was built against, read that one line.

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

## 6b. Preview before you deploy a stack that was in-tree yesterday

A plugin is wire-compatible with pre-split state — URNs are byte-identical,
and `sc preview` on a plugin-pinned stack should read as the same resource
graph it had before. Before you deploy a stack that was still in-tree on its
last green run, run `sc deploy -s <stack> -e <env> -P` (the `-P` is preview-
only) and read the plan: **zero creates and zero deletes** on the resources
you expect to be untouched is the pass criterion.

If the plan wants to **create and delete the same shape of resource at once
with a different URN** — a Secret, a provider, a DatabaseUser, a worker
script — you've hit a URN-parity miss between in-tree and the plugin. Three
patterns bit forge's own staging smoke on 2026-10-30:

- **Secret name uses the parent CLI stack name**
  (`<stack>--<env>--values----<KEY>`), not the child stack. If the preview
  turns `forge-notifier--staging--values----API_KEY` into
  `forge-notifier--staging--staging--values----API_KEY` (note the doubled
  `staging`), the plugin is reading the child name. Fixed in `sc`
  `v2026.10.53+` by carrying both via a new `cli_stack_name` header field.
- **Provider URN flips between parent and child prefix** on the first
  deploy that moves a `uses:` dependency type to the plugin bus — e.g.
  mongodb-atlas going from
  `forge-notifier--staging--mongodb-atlas--mongodb--staging--provider--staging`
  (child, in-tree) to `forge-notifier--mongodb-atlas--…` (parent, plugin).
  This destroys the DatabaseUser + random password with it. Also fixed in
  `sc` `v2026.10.53+` once the compute-processor sidecar carries the child
  name.
- **A plugin returns `EOF`** mid-provision (`error reading from server:
  EOF`). This is `pulumi-aws`'s driver calling `ctx.Log.Warn` in a
  background goroutine on a plugin pctx whose `logState.engine` is nil.
  Fixed by the EngineProxy in `sc` `v2026.10.53+` — upgrade `sc` and the
  issue goes away.

If you see any of the above on an older `sc`, **do not proceed with the
deploy** — pin the plugin at the version that matches your `sc`, upgrade
`sc` to `v2026.10.53` or newer, and preview again.

---

## 7. Things to know before you hit them

**Published platforms are `linux/amd64`, `linux/arm64`, and `darwin/arm64`.**
There is no `darwin/amd64` (Intel Mac) or Windows asset today. On an
unsupported platform the fetch fails with a 404 on the asset URL; build the
plugin from source and use `path:`.

**The fetch is an unauthenticated GitHub download.** Both plugins are public,
so this is fine — but a private plugin repository will not work through
`source:` today. Use `path:`.

**CI runners need egress to `github.com`** — and to `api.github.com` as well
if you rely on §2's auto-resolution instead of pinning. The first run on a
fresh runner downloads each plugin (a few tens of MB). Cache `~/.sc/plugins`
keyed on the pinned versions if that cost matters, or vendor via `path:`.

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
| `server.yaml uses type tokens owned by out-of-tree SC plugins… could not be resolved from GitHub` | No `plugins:` block **and** auto-resolution failed (offline, rate-limited, repo gone) | Paste the block it printed (§3) |
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

For `cloudru`, `mongodb` and `aws`, the pre-split `sc` is `2026.10.44` and
older. Those versions still have the respective driver compiled in and
ignore the plugin entry, so the "pin the block first, then upgrade `sc`"
rule applies to them too. The one asymmetry: §6b's URN-parity + EngineProxy
fixes landed in `sc` `v2026.10.53+` and the corresponding plugin releases
of the same date — downgrade `sc` *or* the plugin individually, not both.

---

## Why this changed

Keeping every cloud's Pulumi SDK inside one binary made `sc` slow to build,
large to ship, and impossible to release independently per cloud: a Cloudflare
fix waited on an unrelated AWS review. Plugins decouple those release
cadences and let a cloud be added without touching SC core. The full rationale
and the plugin-author contract live in the `sc-plugin-architecture` design
docs.

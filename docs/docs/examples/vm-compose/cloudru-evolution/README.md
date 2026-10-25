# Docker Compose on a Machine SC Creates in Cloud.ru Evolution (`cloudru-evolution-vm`)

The same single-machine delivery path as [the `byo` example](../byo-host/README.md), with
the machine created by SC instead of by you. Declare a `type: vm` resource with
`provider: cloudru-evolution-vm` and a Cloud.ru Evolution service-account key, and
`sc provision` creates the boot disk, the instance, the floating address and the ingress
rules, installs Docker over SSH, waits for the machine to actually be ready, and exports
its host facts.

Everything from the SSH connection upwards — readiness, image delivery, the generated
compose project, Caddy, TLS, blue-green — is the **same code** the `byo` path runs. That
is why `byo` came first: by the time this driver existed, everything above SSH had
already been proven live on a machine made by hand.

## Which Cloud.ru this is

Cloud.ru sells two unrelated estates under one brand, one console and one bill. A
credential for one does not authenticate against the other.

| | **Evolution** — this example | **Advanced** (ex-SberCloud) |
|---|---|---|
| Driver | `cloudru-evolution-vm` | `cloudru-vm` — [that example](../cloudru/README.md) |
| Credential | service-account key id + secret → `iam.api.cloud.ru` | Keystone access/secret pair → `iam.<region>.hc.sbercloud.ru` |
| SC auth type | `cloudru-evolution-key` | `cloudru-access-key` |
| Regions | **none** — placement is the availability zone | `region: ru-moscow-1` is required |
| How SC reaches it | the public REST API: `cloud-ru/evo-terraform` ships as a binary only, and Cloud.ru's own resources are not all in it | the bridged `terraform-provider-sbercloud` (MPL-2.0, open sources) |
| cloud-init | **none** — see below | yes |

If `region:` looks missing from `server.yaml`, you are thinking of Advanced. It is not
optional here, it is **refused**: this cloud's compute API has no region dimension at
all, and a `region:` SC silently dropped would read as placement you controlled.

## Files

| File | Goes to | What it is |
|---|---|---|
| `server.yaml` | `.sc/stacks/mycompany/server.yaml` | the parent stack: one `type: vm` resource (`cloudru-evolution-vm`) and the `vm-compose` template |
| `client.yaml` | `.sc/stacks/portal/client.yaml` | the application stack — the same `type: single-image` config that deploys to Lambda |

The directory name is the stack name (`-s mycompany`, `-s portal`); the keys under
`stacks:` in `client.yaml` are environments (`-e staging`).

## Credentials

One service-account key, from the Cloud.ru console under **Customer → service
accounts → keys**. It needs the roles to create compute, disks and floating addresses,
and to edit the security group the instance joins.

```bash
sc secrets reveal
```

```yaml
# .sc/stacks/mycompany/secrets.yaml
auth:
  cloudru:
    type: cloudru-evolution-key
    config:
      keyId: 01234567-89ab-cdef-0123-456789abcdef
      keySecret: "<the key secret, verbatim — see below>"
      # The Evolution project, a UUID visible in the console URL as `projectId=`.
      projectId: 01234567-89ab-cdef-0123-456789abcdef
values:
  SC_STATE_PASSPHRASE: "…"
  staging-portal-mongo-uri: "mongodb://…"
```

```bash
sc secrets hide          # re-encrypt before committing anything
```

> **Write the credential values verbatim here — a `${secret:…}` is passed to the cloud
> literally.** An `auth:` entry lives in `secrets.yaml`, which *is* the encrypted store,
> and SC does not expand placeholders inside it. The failure is expensive because it is
> late: Cloud.ru IAM answers `HTTP 400 … (key_id) is invalid argument: key_id does not
> match required format`, which reads like a revoked or mistyped key, **after** the stack
> has created resources. SC now refuses an unexpanded placeholder at validation and says
> so. `${secret:…}` belongs in `server.yaml` and `client.yaml`, which are plaintext.

`projectId` is required and is **not** inferable from the key. Every Evolution endpoint
is project-scoped, and a call that omits it answers **403 `non_authorized`** — which
reads exactly like a missing role and sends you off granting permissions that were never
the problem.

The same key pair is also the `docker login` credential for the project's Artifact
Registry, which is what the [Container App](../../../reference/supported-resources.md#cloudru-provider)
template pushes to. There is deliberately no separate registry credential.

## Deploy

```bash
sc provision -s mycompany        # creates the machine, installs docker, exports host facts
sc deploy -s portal -e staging   # ships the image, renders compose, brings the slot up
```

`sc provision` has no `-e` flag — it provisions every environment the parent stack
declares.

Measured on a live stand (2026-10-24): the first provision takes a few minutes — boot
disk, then instance, then floating address, then the SSH key, then Docker over SSH, then
the readiness gate. The instance itself reaches `running` in ~45 s; the ten-minute
timeout is the point at which "still booting" stopped being plausible. The first
`sc deploy` after that is **11 resources in 11 seconds**, and a redeploy is faster still.

**Read the first provision's output through a redactor, not with your eyes.** It carries
the generated deploy private key, and with `imageDelivery: registry` the registry
credentials too. `sc stack outputs` prints secret-marked outputs in the clear; redirect
it to a mode-600 file (`umask 077`) rather than to a terminal you will later paste from.

## Pin the host key after the first provision

**This is the one thing this cloud makes you do by hand**, and it is worth doing.

Evolution's `POST /vms` refuses `user_data` — every spelling of it (measured
2026-10-24: HTTP 422 `extra_forbidden`), because the image renders a fixed template from
the `image_metadata` slots it declares and nothing else reaches the first boot. So SC
cannot write a host key into the machine, and cannot learn the real one from the API
either. The **first** connection therefore has nothing to verify, and that connection
carries the deploy key and every `${secret:…}` of every stack on the machine.

Close it on the next provision:

```bash
IP=$(sc stack outputs -s mycompany | jq -r '."portal-vm-vm-host-0-address"')
ssh-keyscan -t ed25519 "$IP"
```

Paste the line into the resource's `hostKey:` — `ssh-keyscan` output goes in verbatim,
and comparing it against the Cloud.ru console by eye is the point of the field being
readable rather than secret. SC warns loudly while it is unset.

On `cloudru-vm` and every other cloud-init cloud the field is **refused**, because there
SC generates and pins the key itself and a pasted one would never be used. The rule is
written against the cloud-init *capability*, not a driver name, so it never becomes dead
config.

## What the driver creates

All of it owned by the `vm` resource, and all of it destroyed with it:

| Resource | Notes |
|---|---|
| boot disk | `size.disk`, or 30 GiB. Ubuntu 24.04 declares `min_disk: 11`, so the default is not a minimum — it is room for the images a compose stack pulls |
| VM | flavour resolved from `size.cores` + `size.memory`; the login and the deploy public key go in as `image_metadata` slots |
| floating IP | allocated and attached **after** the machine exists. Nothing has to happen on the first boot, so it does not need to be bound at creation the way `cloudru-vm`'s elastic IP does |
| ingress rules | on whichever security group the machine joined — 22 always, plus 80/443 TCP and 443 UDP when `ingress.enable` is on |
| deploy key | generated by SC; the public half goes into the VM's `public_key` slot, the private half stays encrypted in the Pulumi checkpoint and is never pasted anywhere |

Satellite objects are named from the VM (`<name>-disk`, `<name>-ip`) rather than being
configurable, so that a destroy can still find them after a create that failed halfway.

**No network and no security group of your choosing.** The same create call refuses
`subnet_id`, `security_group_ids`, `interfaces`, `networks`, `nics` and `floating_ip`
alike, so the machine always lands on the availability zone's **default** subnet and
**default** security group. `network:` is therefore refused in config rather than
accepted and dropped. To place a machine on your own network, create it in the console
and declare it with `provider: byo` and `hosts:`.

## Image and size

```yaml
image: ubuntu-2404-lts
size:
  cores: 2
  memory: 4Gi
  disk: 40Gi
```

`image` accepts a portable SC alias (`ubuntu-2404-lts`, `ubuntu-2204-lts`), an exact
catalogue name, a display name, or a raw image UUID. The alias is **mapped**, not passed
through: Cloud.ru calls the same image `Ubuntu-24.04`.

`size` resolves a flavour **by cores and memory, never by flavour name**, and that is not
a style preference — the catalogue renames sizes underneath you. The 2 vCPU / 4 GiB
flavour was `low-2-4` on 2026-10-23 and `lowcost10-2-4` a day later, with no `low-2-4`
left in the catalogue at all. A name pinned in SC would have failed a deploy on
Cloud.ru's schedule. Defaults are 2 cores / 4Gi — the smallest shape that runs a compose
stack plus the Caddy ingress without the build cache pushing it into swap.

`diskType` is left to the project's default when unset. SC invents no default: the type
names are not the same across projects, and a guess here would be a guess about your
quota.

`zones` takes at most one entry (`ru.AZ-1` … `ru.AZ-3`) — this is a single machine, and a
list of zones would describe a cluster. Left out, SC takes the first zone
`GET /availability-zones` reports.

## Which ports are open

Only what SC needs and what you asked for: **22/tcp always**, plus **80/tcp, 443/tcp and
443/udp** (HTTP/3) when `ingress.enable` is set. **Nothing else** — not a compose
host-port range, not the Kubernetes NodePort range, not 6443.

So a stack that falls back to a host port (no `domain:`, or no ingress) is reachable from
inside the cloud but **not from the internet** until you open that port in the security
group yourself. Both Cloud.ru drivers share one port table, so they cannot drift apart.

## Reachability from inside Russia

Two egress paths a fresh machine in this cloud uses, and neither is assumed to work:

| Path | Used by | If it is slow or blocked |
|---|---|---|
| `get.docker.com` + the distro's apt mirrors | the Docker bootstrap SC runs over SSH after the machine exists | use an image that already ships Docker (the step is then a no-op), or set `packageMirror:` and point at a private mirror of Docker's apt repository |
| Docker Hub | `imageDelivery: registry`, and base-image pulls | `registryMirrors:`, or the default `imageDelivery: ssh-tar`, which streams the built image over the SSH connection and needs no registry reachable from the machine at all |
| Let's Encrypt | the machine's Caddy, on the first deploy with a `domain:` | `ingress.acmeCa` points at another ACME directory; whether the Russian Trusted CA speaks ACME is **unverified** — the field exists so there is an option, not because the path is known to work |

Unlike `cloudru-vm`, none of this happens during a first boot SC cannot watch: there is
no cloud-init, so the Docker install is an SSH step whose output you see and whose
failure is a failed provision rather than a machine that never becomes ready.

`ssh-tar` is the default delivery for exactly this reason, and it is worth more here than
on a machine you brought.

## What this driver refuses

Every refusal is deliberate: a field accepted and silently ignored is a future incident
in which the operator is certain the setting applied.

| Field | Why |
|---|---|
| `region` | this cloud's API has no region — one global endpoint, placement by availability zone. Pin placement with `zones:` |
| `publicIP: false` | every delivery step is an outbound connection *from* wherever `sc deploy` runs, so a machine with no public address is created, billed and unreachable |
| `hosts` | SC creates the machine; a host list describes one you brought (`provider: byo`) |
| `network` | the create call accepts no subnet or security group at all (measured), so the machine always joins the zone's defaults |
| `extraSshKeys` | extra keys are authorised through user-data, which this cloud does not run; the create takes exactly one public key — the deploy key. Add one by hand, or keep console access for incidents |
| `zones` with more than one entry | one machine is in one zone |
| `size.memory` that is not whole GiB | memory resolves a flavour by exact match; rounding would hand you a different machine |

The `vm-compose` template's own refusals (`scale` other than 1, `domainProxied`,
`clusterIPAddress`) are unchanged from the `byo` path and are documented
[there](../byo-host/README.md#what-this-template-refuses).

## Where the stack is reachable

Identical to the `byo` path: with `ingress.enable: true` and a `domain:` on the client
stack, the machine's Caddy terminates TLS and the stack publishes no host port; without
it, the ingress service is published on a deterministic host port in the 20000–30000
range. Both are reported as `<stack>-vm-compose-url`:

```bash
sc stack outputs -s portal -e staging | jq -r '."portal-vm-compose-url"'
```

The DNS A record points at the floating address the driver created
(`portal-vm-vm-host-0-address`) and must **not** be proxied — Cloudflare's orange cloud
breaks the ACME challenge the machine answers itself.

Cloud.ru has its own Cloud DNS and SC's `cloudru-dns` registrar for it is live-verified
against a delegated apex, so this record can be SC's job rather than a console step —
add the registrar to the parent stack and give the service account the `cdns.admin` role
in the project. See
[DNS Management](../../../guides/dns-management.md#4-cloudru-cloud-dns-registrar-cloudru-dns).

## Teardown

```bash
yes Y | sc destroy -s portal -e staging
yes Y | sc destroy -s mycompany --parent
```

`sc destroy` prompts for `Y`/`N` and has **no `--yes` flag**, and — the part that costs
money — when its prompt gets no answer it prints `'Y' or 'N' expected, but got ""` and
**exits 0**. An unattended teardown therefore reports success having destroyed nothing.
Pipe the answer, and then confirm against the cloud's own console or API rather than
against the Pulumi log.

## Next: k3s on this machine

The same resource, plus a `k3s-cluster` that installs a control plane on it:
[k3s on a Machine SC Creates in Cloud.ru Evolution](../../k3s/cloudru-evolution/README.md).

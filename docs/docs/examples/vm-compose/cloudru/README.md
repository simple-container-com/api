# Docker Compose on a Machine SC Creates in Cloud.ru (`cloudru-vm`)

The same single-machine delivery path as [the `byo` example](../byo-host/README.md), with
the machine created by SC instead of by you. Declare a `type: vm` resource with
`provider: cloudru-vm` and a Cloud.ru key pair, and `sc provision` creates the VPC, the
subnet, the security group, the floating address, the SSH key pair and the instance, then
connects to what it created.

Everything from the SSH connection upwards — readiness, image delivery, the generated
compose project, Caddy, TLS, blue-green — is the **same code** the `byo` path runs. That
is deliberate and is why the `byo` path came first: by the time this driver existed,
everything above SSH had already been proven live on a machine made by hand.

## Which Cloud.ru this is

Cloud.ru sells two unrelated estates, with different APIs and different credentials. A
key pair from one does not authenticate against the other.

| | Driver | How SC reaches it |
|---|---|---|
| **Advanced** (ex-SberCloud) | `cloudru-vm` — this example | its Terraform provider (`sbercloud-terraform/terraform-provider-sbercloud`, MPL-2.0) has open Go sources, so it is bridged into Pulumi the classic way (`simple-container-com/pulumi-sbercloud`) |
| **Evolution** | `cloudru-evolution-vm` — [that example](../cloudru-evolution/README.md) | its provider (`cloud-ru/evo-terraform`) ships as a binary only, so that driver talks to Cloud.ru's public REST API instead of riding a provider at all |

**If your console says "Evolution", this example does not apply** — the credentials
are different (a service-account key, not an access/secret pair), there are no regions,
and the cloud runs no cloud-init. Go to
[Compose on a Machine SC Creates in Cloud.ru Evolution](../cloudru-evolution/README.md).

## Files

| File | Goes to | What it is |
|---|---|---|
| `server.yaml` | `.sc/stacks/mycompany/server.yaml` | the parent stack: one `type: vm` resource (`cloudru-vm`) and the `vm-compose` template |
| `client.yaml` | `.sc/stacks/portal/client.yaml` | the application stack — the same `type: single-image` config that deploys to Lambda |

The directory name is the stack name (`-s mycompany`, `-s portal`); the keys under
`stacks:` in `client.yaml` are environments (`-e staging`).

## Credentials

One permanent key pair, from the Cloud.ru console under **My Credentials → Access Keys**.
It has to be allowed to create compute, VPC, security groups and elastic IPs — that is
exactly the set the driver makes, listed below.

The pair is declared as an `auth:` entry in the parent stack's encrypted value store, and
the resource references it as `${auth:cloudru}`:

```bash
sc secrets reveal
```

```yaml
# .sc/stacks/mycompany/secrets.yaml
auth:
  cloudru:
    type: cloudru-access-key
    config:
      accessKey: AKIA…
      secretKey: ${secret:CLOUDRU_SECRET_KEY}
      # Optional. Defaults: region ru-moscow-1, project named after the region,
      # Keystone endpoint derived from the region. Cloud.ru is OpenStack-descended, so
      # the scoping vocabulary is Keystone's: a *project* is the tenancy resources are
      # created in, a *domain* is the account above it. Both are normally inferable
      # from the key pair, which is why only the pair itself is required.
      # region: ru-moscow-1
      # projectName: ru-moscow-1
      # domainName: your-account
      # authUrl: https://iam.ru-moscow-1.hc.sbercloud.ru/v3
values:
  CLOUDRU_SECRET_KEY: "…"
  SC_STATE_PASSPHRASE: "…"
  staging-portal-mongo-uri: "mongodb://…"
```

```bash
sc secrets hide          # re-encrypt before committing anything
```

**Region is not one input among many on this cloud.** It also selects the IAM endpoint
and the project resources are created in, because Cloud.ru names an account's default
project after its region. SC keeps the three in step from a single `region:` — setting
them apart authenticates against one region's tenancy and then creates in another, which
presents as resources that exist but cannot be found.

There is no `projectId` field, deliberately: upstream's scoping inputs are
`project_name` / `domain_name` / `domain_id`, so a field accepting an id would silently
drop it.

## Deploy

```bash
sc provision -s mycompany        # creates the machine, waits for cloud-init, exports host facts
sc deploy -s portal -e staging   # ships the image, renders compose, brings the slot up
```

`sc provision` has no `-e` flag — it provisions every environment the parent stack
declares.

**Read the first provision's output through a redactor, not with your eyes.** It carries
the generated deploy private key and, with `imageDelivery: registry`, the registry
credentials. `sc stack outputs` prints secret-marked outputs in the clear; redirect it to
a mode-600 file rather than to a terminal you will later paste from.

## What the driver creates

All of it owned by the `vm` resource, and all of it destroyed with it:

| Resource | Notes |
|---|---|
| VPC | `10.47.0.0/16` unless you pin an existing network |
| subnet | `10.47.1.0/24`, gateway `10.47.1.1`, in the chosen zone |
| security group | 22 always; 80 and 443 (TCP **and** UDP, for HTTP/3) only when `ingress.enable` is on |
| its ingress rules | one per protocol/port pair above — one with the ingress off, four with it on |
| SSH key pair | the public half of a key SC generates; the private half lives encrypted in the Pulumi checkpoint and is never pasted anywhere |
| elastic IP | 100 Mbit, `5_bgp`, **bound at instance creation** |
| ECS instance | flavour chosen from the flavours datasource by `size.cores` + `size.memory` |

The ranges `10.47.0.0/16` / `10.47.1.0/24` are deliberately neither `10.0.0.0/16` nor
`192.168.0.0/16`: the only job they have is to not collide with the VPN or the peering
you already run.

Two upstream behaviours the driver pins rather than inherits:

- **`delete_eip_on_termination` defaults to true.** Forced to false, so destroying the
  machine does not silently release an address something else points at. The address is
  a resource of its own and is destroyed as one.
- **The address is bound at creation, not attached afterwards.** `eip_id` is ForceNew,
  and more importantly cloud-init needs egress on the first boot — it installs Docker
  from `get.docker.com`. A machine that gets its address a minute later has already
  failed its first boot.
- **`user_data` is not ForceNew upstream.** It is made replace-on-change here, because
  cloud-init only runs on the first boot: a changed user-data that did not replace the
  machine would report a green deploy while the machine still ran the old configuration.

## Image and size

```yaml
image: ubuntu-2404-lts
size:
  cores: 2
  memory: 4Gi
  disk: 60Gi
  diskType: SSD
```

`image` accepts a cloud-independent alias (`ubuntu-2404-lts`, `ubuntu-2204-lts`), an exact
Cloud.ru image name, or a raw image UUID. An alias resolves through a regex over the
public image names rather than a compiled-in id, because the names carry a build number
that changes on Cloud.ru's schedule and an exact id would rot.

`cores` and `memory` are a **flavour filter**, not a shape SC creates: the driver asks the
flavours datasource for general-purpose shapes matching both and takes the smallest match
deterministically (the datasource returns ids unsorted, so they are sorted before
picking). Memory is matched in whole GiB — `1536Mi` is refused rather than rounded,
because rounding would hand you a different machine than the one you asked for.

`disk` is **not** defaulted: left out, the image keeps its own root-disk size, which is a
better answer than one SC invents.

`zones` takes at most one entry — this is a single machine, and a list of zones would
describe a cluster. Left out, SC takes the first zone the availability-zones datasource
reports (documented as alphanumeric, so stable).

## Reachability from inside Russia

Three egress paths a fresh machine in this cloud uses on its first boot or first deploy,
and none of them is assumed to work:

| Path | Used by | If it is slow or blocked |
|---|---|---|
| `get.docker.com` + `download.docker.com` | cloud-init's Docker install | use an image that already ships Docker (the install step is a no-op then), or a private mirror of Docker's apt repository |
| the distribution's apt mirrors | `package_update` on the first boot | set `packageMirror:` — it is written into cloud-init's `apt` module, which rewrites the sources *before* any package-installing module runs |
| Docker Hub | `imageDelivery: registry`, and base-image pulls | `registryMirrors:`, or the default `imageDelivery: ssh-tar`, which streams the built image over the SSH connection and needs no registry reachable from the machine at all |
| Let's Encrypt | the machine's Caddy, on the first deploy with a `domain:` | `ingress.acmeCa` points at another ACME directory; whether the Russian Trusted CA speaks ACME is **unverified** — the field exists so there is an option, not because the path is known to work |

`packageMirror` does **not** cover the Docker install: that is a curl to a host of
Docker's own, and the script then installs from Docker's apt repository rather than from
the distribution. The two answers for a machine that cannot reach it are the two in the
table; neither is expressible as a one-line field, which is why there is no flag
pretending to be one.

`ssh-tar` is the default delivery for exactly this reason, and it is worth more here than
on a machine you brought.

## What this driver refuses

Every refusal is deliberate: a field accepted and silently ignored is a future incident
in which the operator is certain the setting applied.

| Field | Why |
|---|---|
| `publicIP: false` | every delivery step is an outbound connection *from* wherever `sc deploy` runs, so a machine with no public address is created, billed and unreachable — and cloud-init has no egress either |
| `hosts` | SC creates the machine; a host list describes one you brought (`provider: byo`) |
| `zones` with more than one entry | one machine is in one zone |
| `network` with only `id` or only `subnets` | picking "some subnet in the VPC" places the machine in a zone, or behind a route table, you did not choose |
| `size.memory` that is not whole GiB | the flavours datasource matches memory exactly; rounding would hand you a different machine |
| `region` absent | stated on the resource, not inherited silently: it selects the IAM endpoint and the project as well as the placement, so a resource that did not say where it lives could be created somewhere the credential's default moved to |

The `vm-compose` template's own refusals (`scale` other than 1, `domainProxied`,
`clusterIPAddress`, `bind` mounts) are unchanged from the `byo` path and are documented
[there](../byo-host/README.md#what-this-template-refuses).

## Where the stack is reachable

Identical to the `byo` path: with `ingress.enable: true` and a `domain:` on the client
stack, the machine's Caddy terminates TLS and the stack publishes no host port; without
it, the ingress service is published on a deterministic host port in the 20000–30000
range. Both are reported as `<stack>-vm-compose-url`:

```bash
sc stack outputs -s portal -e staging | jq -r '."portal-vm-compose-url"'
```

The DNS A record points at the elastic IP the driver created
(`portal-vm-vm-host-0-address`) and must **not** be proxied — Cloudflare's orange cloud
breaks the ACME challenge the machine answers itself.

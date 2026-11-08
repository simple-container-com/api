# Docker Compose on a Machine You Brought (`byo`)

Deploy an existing `single-image` or `cloud-compose` client stack onto one virtual
machine over SSH — no Kubernetes, no managed container runtime, and (with the default
`ssh-tar` delivery) no container registry the machine has to be able to reach.

This is the path to use when the cloud you are deploying into has no SC driver yet:
you create the machine in its console, declare it with `provider: byo`, and everything
above SSH works the same as it will once a driver for that cloud exists.

## Files

| File | Goes to | What it is |
|---|---|---|
| `server.yaml` | `.sc/stacks/mycompany/server.yaml` | the parent stack: one `type: vm` resource (`byo`) and the `vm-compose` template |
| `client.yaml` | `.sc/stacks/portal/client.yaml` | the application stack — the same `type: single-image` config that deploys to Lambda |

The directory name is the stack name (`-s mycompany`, `-s portal`); the keys under
`stacks:` in `client.yaml` are environments (`-e staging`).

## Prepare the machine

Nothing in SC creates it. In your cloud's console:

1. Create an Ubuntu 24.04 VM with a public IP and port 22 reachable.
2. Create a login for deploys (`sc` is SC's default; any name works via `user:`), and
   authorise a key pair you generate for this purpose only:

   ```bash
   ssh-keygen -t ed25519 -f ./portal_vm_deploy_key -N ''
   # add portal_vm_deploy_key.pub to ~/.ssh/authorized_keys on the machine
   ```

3. Read the machine's host key and pin it in `server.yaml`:

   ```bash
   ssh-keyscan -t ed25519 203.0.113.10
   ```

   Compare it against what the console shows before you trust it — reading it off the
   network is the thing pinning protects you from. Deploying without `hostKey` works and
   warns: SSH then authenticates the channel but nothing authenticates the *machine*, so
   the deploy key and every secret in the stack's env file go to whoever answers on that
   address. A CI runner has no `known_hosts` to fall back on, which is why there is no
   implicit default.

You do **not** need to install Docker. `sc provision` installs Docker Engine and the
compose v2 plugin on a `byo` host that will need them, and adds the deploy user to the
`docker` group — the same guarded install cloud-init performs on a machine SC creates,
so a host that already has Docker is left alone and nothing is downloaded on later
runs. Two cases where it is still yours to do:

- the machine cannot reach `https://get.docker.com` (the install says so by name, and
  an apt mirror does not help — the installer adds Docker's own apt repository);
- `docker-compose` v1 is present but the v2 plugin is not. Add the
  `docker-compose-plugin` package; SC refuses to guess which of the two you meant.

A host declared only as a k3s node (`role: k3s-server` / `k3s-agent`) gets no Docker:
k3s brings its own containerd, and Docker's `iptables -P FORWARD DROP` next to flannel
is a hazard, not a convenience. Such a host does get Docker if the same vm resource
also runs the Caddy ingress, which is itself a container.

## Deploy

The deploy key is a `${secret:...}` like any other, so it lives in the parent stack's
encrypted value store — a block scalar, because it is a multi-line PEM:

```bash
sc secrets reveal
```

```yaml
# .sc/stacks/mycompany/secrets.yaml
values:
  PORTAL_VM_DEPLOY_KEY: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    …
    -----END OPENSSH PRIVATE KEY-----
  SC_STATE_PASSPHRASE: "…"
  staging-portal-mongo-uri: "mongodb://…"
```

```bash
sc secrets hide          # re-encrypt before committing anything
sc provision -s mycompany   # connects, waits for docker, exports the host facts
sc deploy -s portal -e staging   # ships the image, renders compose, brings the slot up
```

`sc provision` has no `-e` flag — it provisions every environment the parent stack
declares.

`sc provision` publishes the machine's facts as stack outputs the template reads
(`portal-vm-vm-host-0-address`, `-user`, `-port`, `-host-key`, and the deploy key as a
secret output). `sc deploy` then:

1. creates `/etc/sc/<stack>/<env>` and the shared networks and named volumes,
2. writes the env file `0600` — values arrive on the SSH connection's stdin, never in a
   command line or a log,
3. gets the images onto the machine (`ssh-tar` streams them; `registry` logs in and
   pulls) — the stack's own image, and with `externalImages: ship` every third-party
   image in the compose file too, in one stream,
4. renders a compose project and brings up the inactive slot,
5. gates on the ingress service being healthy, points the machine's Caddy at the new
   slot (or, with the ingress off, publishes it on the host port), then stops the slot
   that was serving,
6. prunes images beyond `keepImages`.

## Where the stack is reachable

Two cases, and `ingress.enable` on the `vm` resource decides which.

**With the ingress on** and a `domain:` on the client stack, the machine's Caddy
terminates TLS and the stack publishes **no host port at all** — 443 is the only way in,
`<stack>-vm-compose-url` is the `https://` URL, and the last step of a deploy is
rewriting one file in Caddy's `conf.d` and reloading it:

```bash
sc stack outputs -s portal -e staging | jq -r '."portal-vm-compose-url"'
# https://portal.example.com
curl -sS https://portal.example.com   # valid certificate, issued on the first deploy
```

The DNS A record is SC's to write when the parent stack declares a registrar — one
record per machine on the `vm` resource — and the provision warns, naming the addresses,
when it declares none. Whether that record may be **proxied** follows the machine: while
the machine terminates TLS it must not be, because Cloudflare's orange cloud breaks the
ACME challenge the machine answers itself, and `domainProxied: true` is refused on that
arrangement. On a fleet that sets `ingress.originTls: false` the proxy *is* the front
door and `domainProxied: true` is the correct setting — see
[Front door](#front-door-who-terminates-tls). Caddy runs as a container in its own compose project
(`sc-caddy`) owned by the **`vm` resource**, not by any stack — one machine has one port
443, and a Caddy living inside a stack's project would be destroyed by the very deploy
that is flipping traffic away from it. Its ACME account key and issued certificates live
in named volumes that no teardown removes, because Let's Encrypt allows 50 certificates
per domain per week.

Because a stack's deploy owns exactly one file in `conf.d` plus a reload, two stacks on
one machine can deploy concurrently without a lock. The reverse of that: `conf.d` is
loaded as a single config, so a bad site file would break the reload for *every* stack
behind the ingress — the new file is therefore validated against the global Caddyfile
before the reload, and the previous one is restored if either step fails.

**With the ingress off** (the default), the ingress service is published on a
deterministic host port in the 20000–30000 range, derived from the stack and environment
name:

```bash
sc stack outputs -s portal -e staging | jq -r '."portal-vm-compose-url"'
# http://203.0.113.10:29763
curl -sS http://203.0.113.10:29763   # answers the application
```

Two stacks on one machine get different ports, and a collision is detected at deploy
time when docker refuses to bind — reported with both stack names, not left to be
discovered later.

A `domain:` with no ingress, or an ingress with no `domain:`, is **warned about and
deployed anyway** on the host port: the stack genuinely answers there, and the warning
exists because "my certificate never appeared" otherwise leaves no local evidence. A
bare `prefix:` without a `domain:` cannot be served on a VM at all — on Kubernetes a
prefix is grafted onto an aggregator site that already owns a host name, and one machine
has no such site.

## Front door: who terminates TLS

`ingress.originTls` on the `vm` resource decides it, and the rest of the front door
follows from that one answer. Unset means `true` — every resource that predates the
field.

| | `originTls: true` (default) | `originTls: false` |
|---|---|---|
| who presents the certificate | the machine's Caddy, via ACME | whatever is in front (Cloudflare's proxy, a cloud LB) |
| `ingress.email` | required — no CA issues without one | not used; no ACME runs at all |
| the origin speaks | HTTPS on 443 | plain HTTP on 80 |
| `domainProxied: true` on a stack | refused — the orange cloud breaks HTTP-01 | correct; an *unproxied* record would expose a cleartext origin |
| a fleet of N machines | needs `ingress.acmeStorage` | needs nothing shared |

**Why a fleet needs the store.** Behind N A records, Let's Encrypt's validator reaches an
arbitrary machine, which 404s a challenge token another machine minted. certmagic shares
that state — and the issued certificate — through storage, so `ingress.acmeStorage`
points every machine of the resource at one S3-compatible bucket:

```yaml
ingress:
  enable: true
  email: ops@example.com
  acmeStorage:
    bucket: mycompany-acme
    endpoint: https://storage.yandexcloud.net   # omit for AWS S3
    region: ru-central1
    accessKey: ${secret:ACME_S3_ACCESS_KEY}
    secretKey: ${secret:ACME_S3_SECRET_KEY}
    # Encrypts the certificates and the account key at rest in the bucket. Exactly
    # 32 bytes. Lose it and the store is unreadable — the fleet re-issues, against
    # Let's Encrypt's 50-per-domain-per-week limit.
    encryptionKey: ${secret:ACME_S3_ENCRYPTION_KEY}
```

The three credentials never reach the Caddyfile: it renders `{$SC_ACME_S3_*}`
placeholders and the values arrive in a 0600 env file. SC also renders
`admin { config { persist false } }` alongside the store, because Caddy substitutes
those placeholders at adapt time and would otherwise autosave the resolved JSON —
credentials included — to `/config/caddy/autosave.json` on every start.

The same block exists on the Kubernetes edge as `kubernetes-caddy`'s `acmeStorage`,
rendered by the same code, with the credentials delivered as secret env vars.

`lbConfig` on the client stack is the other half of the edge and is unchanged by any of
this: `siteExtraHelpers` injects Caddy directives into *this stack's* site file (rate
limits, headers, a redirect), and it means the same thing on the Kubernetes path because
both run the same `simplecontainer/caddy` image.

## A machine with no route to a registry

`imageDelivery: ssh-tar` is only half the promise on its own: it moves the image SC
built, and leaves every other image in the compose file to be pulled by the machine.
Two settings close the gap, and together they mean the machine pulls nothing at all:

| Setting | Where | What it moves |
|---|---|---|
| `imageDelivery: ssh-tar` | `vm-compose` template | the stack's own image (the default) |
| `externalImages: ship` | `vm-compose` template | every `image:` in the compose file that SC did not build — a mongo, a redis (the default under `ssh-tar`) |
| `ingress.imageDelivery: ssh-tar` | `vm` resource | the Caddy image (NOT the default — see below) |

All three work the same way: the **deploy runner** pulls what it does not have and
streams it over the SSH connection it is already using. So the runner needs a route to
the registry and a local docker; the machine needs neither.

Three things to know before relying on it:

- **Pin third-party tags.** The re-ship is triggered by the image reference, so
  `mongo:7.0.14` is re-shipped when you change it to `7.0.15` — but `mongo:7`, after
  upstream moves it, is not. Detecting that would mean pulling during `pulumi preview`.
- **`docker login` on the runner first** for a private third-party image. The ship
  pulls through the docker engine API, which takes no credentials from
  `~/.docker/config.json`, and pulls only what the runner does not already have — so a
  `docker login && docker pull` step before `sc deploy` is all it needs.
- **The ingress is opt-in, and it changes `sc provision`.** `ingress.imageDelivery:
  ssh-tar` makes `sc provision` need a working local docker daemon, which it has never
  needed. It is left off by default for that reason, not because the path is worse.

## What this template refuses

Every refusal below is deliberate: a field accepted and silently ignored is a future
incident in which the operator is certain the setting applied.

| Field | Why |
|---|---|
| `clusterIPAddress` | there is no cluster — the stack is reachable at the address of each machine the stack was deployed onto |
| a `bind` mount in a compose file | its source is a path on the developer's machine, which does not exist on the VM |
| `region`, `size`, `image`, `publicIP`, `network`, `extraSshKeys` on a `byo` resource | SC did not create the machine and cannot apply them; configure them in the console |

`timeout` and `maxEphemeralStorage` are accepted and warned about rather than refused:
they describe a function runtime that is not present here.

Two fields that **used** to be on this list and are not any more:

- **`scale`** with `max` other than 1 is now a warning. A fleet made it an expressible
  thing to want — the stack gets one replica of each service *per machine*, so a
  three-host `vm` resource genuinely serves three. The number is not this field's to set,
  though: it is the host count on the parent stack's `vm` resource, and the warning says
  so. The count is also checked against the real one a tier down, at deploy, which is the
  first place both are in hand. Scaling *within* a machine still needs a k3s/Kubernetes
  parent.
- **`domainProxied`** is refused only while the machine terminates TLS. See
  [Front door](#front-door-who-terminates-tls) — and note that refusal now lands at
  deploy rather than at config load, because the field it depends on is on the parent
  stack's resource.

## Two stacks, one machine

The machine lives in the parent stack's `resources:` and the delivery behaviour in
`templates:` — the same split `gcp-gke-autopilot` uses for `gkeClusterResource`. That is
what lets a second client stack reference the same `vm` resource (its own compose
project, its own env file, its own slots) and lets you switch `strategy` or
`imageDelivery` without touching the machine description.

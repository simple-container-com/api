---
title: Cloud.ru
description: This guide is for DevOps teams configuring a parent stack for Cloud.ru — serverless Container Apps, machines SC creates itself, and single-node Kubernetes on them
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2026-10-24'
---


# **Guide: Configuring a Parent Stack for Cloud.ru with Simple Container**

This guide is for **DevOps teams** deploying on **Cloud.ru** with **Simple Container**.
Three delivery paths are supported, all from one credential:

| Path | Template | What runs | Use when |
|---|---|---|---|
| **Serverless** | `cloudru-container-app` | an Evolution **Container App** — public HTTPS, scale to zero | a stateless HTTP service; the cheapest thing that exists |
| **One machine** | `vm-compose` | a generated docker compose project on a VM SC created | you need a long-lived process, a sidecar, or a port that is not HTTP |
| **Kubernetes** | `k3s-cloudrun` | single-node k3s on a VM SC created | the workload wants the Kubernetes API |

All three are **live-verified** (2026-10-24). The one piece that is implemented but not
yet smoked end to end is the DNS registrar — see [DNS and domains](#dns-and-domains).

---

# **First: which Cloud.ru do you have?**

**Cloud.ru sells two unrelated clouds under one brand, one console and one bill.** A
credential for one does not authenticate against the other, and which one your account
has decides the auth type, the driver and whether `region:` is even legal. Settle this
before anything else — it is the single most common way to lose an hour here.

| | **Evolution** | **Advanced** (ex-SberCloud) |
|---|---|---|
| What it is | Cloud.ru's own platform | the ex-SberCloud estate, OpenStack-descended by way of Huawei Cloud |
| Credential | service-account **key id + secret** → `iam.api.cloud.ru` | Keystone **access/secret pair** → `iam.<region>.hc.sbercloud.ru` |
| SC auth type | `cloudru-evolution-key` | `cloudru-access-key` |
| Regions | **none** — one global endpoint; placement is the AZ (`ru.AZ-1`…`ru.AZ-3`) | `ru-moscow-1` and friends; `region` is **required** |
| Serverless | **Container Apps** | — |
| Machines | `provider: cloudru-evolution-vm` | `provider: cloudru-vm` |
| DNS | Cloud DNS (`cloudru-dns`) | ex-SberCloud DNS, not wired into SC |
| Registry | Artifact Registry | SWR, not wired into SC |
| cloud-init | **no** (see below) | yes |

Most Cloud.ru projects today are **Evolution**, and the serverless path exists only
there. This guide is written for Evolution; where Advanced differs it says so.

---

# **Prerequisites**

- **Simple Container is installed**:

  ```sh
  curl -s "https://dist.simple-container.com/sc.sh" | bash
  ```

- A **Cloud.ru project**, and its **project id**. Every Evolution endpoint is
  project-scoped and the id is **not inferable from the credential** — a call that
  omits it answers `403 non_authorized`, which reads exactly like a missing role and
  sends you off granting permissions that were never the problem.

- A **service account** in that project with a **key pair** (key id + secret). The same
  pair authenticates the REST API *and* is the `docker login` for Artifact Registry, so
  there is no separate registry credential to create.

  Roles needed depend on the path: Container Apps and Artifact Registry for serverless;
  compute + networking for the VM paths; **Cloud DNS** additionally if you want the
  registrar (see below).

- **Somewhere to keep Pulumi state.** This is the one gap worth knowing up front: SC has
  **no Cloud.ru state backend**. The options are `fs` (state next to the checkout,
  encrypted with a passphrase) or a bucket in another cloud (`s3-bucket`, `gcp-bucket`,
  `yc-object-storage` — none of which accept a custom S3 endpoint, so Cloud.ru's own
  object storage cannot stand in). For one operator `fs` is fine; for a team, today that
  means a bucket elsewhere.

---

# **Setting Up Cloud.ru Secrets**

## **Step 1: Define `secrets.yaml`**

```yaml
---
# File: "myproject/.sc/stacks/devops/secrets.yaml"

schemaVersion: 1.0

auth:
  cloudru:
    type: cloudru-evolution-key
    config:
      keyId: "a1b2c3d4-...."
      keySecret: "...."
      projectId: "11111111-2222-3333-4444-555555555555"

values:
  # The same key pair again, as the registry login — see the note below.
  CLOUDRU_KEY_ID: "a1b2c3d4-...."
  CLOUDRU_KEY_SECRET: "...."
  SC_STATE_PASSPHRASE: "a long random string"
  MY_SERVICE_TOKEN: "...."
```

### **What This Does**

- `auth.cloudru` is referenced from `server.yaml` as `${auth:cloudru}` by every
  Cloud.ru template, resource and registrar.
- `values:` is the ordinary `${secret:...}` pool.

> **Write the credential values verbatim inside `auth:`.** `${secret:...}`
> placeholders are **not expanded** in an `auth` block. SC refuses an unexpanded
> placeholder outright rather than sending it to IAM — which would otherwise fail
> halfway through a deploy with
> `key_id does not match required format`. That is why the key appears twice above:
> once as the credential, once in `values:` for anything that needs a
> `${secret:...}` reference to it (the k3s path's registry login, for instance).

On **Advanced**, the same block is:

```yaml
auth:
  cloudru:
    type: cloudru-access-key
    config:
      accessKey: "...."
      secretKey: "...."
      region: ru-moscow-1
```

Edit the file with `sc secrets reveal -F`, and **`sc secrets hide -F` before any
commit** — `secrets.yaml` is gitignored either way, but an interrupted edit leaves it
decrypted on disk.

---

# **Configuring Infrastructure Provisioning (`server.yaml`)**

## **Step 2A: the serverless path**

The whole parent stack for Container Apps:

```yaml
---
# File: "myproject/.sc/stacks/devops/server.yaml"

schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    state-storage:
      type: fs
      config:
        path: ${git:root}/.sc/state
    secrets-provider:
      type: passphrase
      config:
        passPhrase: ${secret:SC_STATE_PASSPHRASE}

templates:
  cloudru-app:
    type: cloudru-container-app
    config:
      credentials: "${auth:cloudru}"
      # Optional. When empty, SC creates (and reuses) one Artifact Registry named
      # after the client stack with the environment stripped, so every env of a
      # stack shares a registry and the repository inside it carries `--<env>`.
      # Set this to share one registry across several stacks.
      # registryName: myproject
```

There are no `resources:` to declare: the Container App, its revision and the registry
are all created from the client stack.

> Add `.sc/state` to `.gitignore`. The checkpoint under it carries the stack's inputs
> and outputs — including any generated key — encrypted with the passphrase above and
> no stronger than it.

## **Step 2B: a machine SC creates**

```yaml
templates:
  compose-on-vm:
    type: vm-compose
    config:
      vmResource: portal-vm
      # Streams the built image over the same SSH connection, so no registry has to
      # be reachable from the machine.
      imageDelivery: ssh-tar
      strategy: blue-green
      keepImages: 3

resources:
  resources:
    staging:
      template: compose-on-vm
      resources:
        portal-vm:
          type: vm
          config:
            provider: cloudru-evolution-vm
            credentials: ${auth:cloudru}
            # NO `region:` — refused, not optional. Placement is the zone.
            zones: [ru.AZ-1]
            publicIP: true             # required: SC deploys over SSH from outside
            image: ubuntu-2404-lts     # an SC alias, mapped per cloud
            size: { cores: 2, memory: 4Gi, disk: 40Gi }
            sshUser: sc                # the default; not root
            # hostKey: "45.132.179.77 ssh-ed25519 AAAA..."   # pin after first provision
            ingress:
              enable: true
              email: ops@example.ru
```

Field-by-field annotation:
[the vm-compose Evolution example](../examples/vm-compose/cloudru-evolution/README.md).

### **Three things follow from Evolution having no cloud-init**

Evolution's `POST /vms` refuses `user_data` in every spelling (measured 2026-10-24:
HTTP 422 `extra_forbidden`). That one fact explains most of what looks arbitrary on
this driver:

1. **Docker is installed over SSH after the machine exists**, not during first boot.
   Provision is correspondingly slower, and it needs egress to the docker repository
   (or a `packageMirror` / `registryMirrors` you name).
2. **The SSH host key is yours to pin.** SC cannot seed one and cannot read the real
   one back from the API, so the first connection has nothing to verify — and it
   carries the deploy key and every `${secret:...}` of every stack on the machine.
   After the first provision, `ssh-keyscan -t ed25519 <address>` and paste the line
   into `hostKey:`. SC warns loudly while it is unset. (On `cloudru-vm` and every
   other cloud-init cloud, `hostKey:` is **refused** — there SC pins it itself.)
3. **`extraSshKeys` is refused.** Extra keys are authorised by writing them into
   user-data, and the create call takes exactly one public key — the deploy key SC
   generates. Add one by hand, or keep console access for incidents.

### **Your own network is not available on Evolution**

The create call rejects `subnet_id`, `security_group_ids`, `interfaces`, `networks`,
`nics` and `floating_ip` alike, so the machine always lands on the availability zone's
**default** subnet and **default** security group; SC opens its ingress rules on
whichever group the machine actually joined, read back off its own interface.
`network:` is therefore refused rather than accepted and ignored. To place a machine on
your own network, create it in the console and declare it with `provider: byo`.

On **Advanced** (`cloudru-vm`) SC creates the VPC, subnet, security group, floating
address and key pair itself, and `region: ru-moscow-1` is required.

## **Step 2C: Kubernetes on that machine**

Add one resource and swap the template:

```yaml
templates:
  k3s-run:
    type: k3s-cloudrun
    config:
      k3sClusterResource: ru-cluster
      # Required: a kubelet can only PULL. There is no `ssh-tar` equivalent here.
      dockerRegistryURL: sc-registry.cr.cloud.ru
      dockerRegistryUsername: ${secret:CLOUDRU_KEY_ID}
      dockerRegistryPassword: ${secret:CLOUDRU_KEY_SECRET}

resources:
  resources:
    staging:
      template: k3s-run
      resources:
        cluster-vm:
          type: vm
          config: { provider: cloudru-evolution-vm, credentials: "${auth:cloudru}", zones: [ru.AZ-1], publicIP: true, image: ubuntu-2404-lts, size: { cores: 2, memory: 4Gi, disk: 40Gi } }
        ru-cluster:
          type: k3s-cluster
          config:
            vmResource: cluster-vm
```

Two things about the registry: it is `<registry-name>.cr.cloud.ru`, **SC does not create
it on this path** (the Container App template does, from `registryName`; `k3s-cloudrun`
does not), and a registry inside the same cloud is the only pull path here that does
not depend on egress to anywhere.

Full walkthrough: [k3s on a Cloud.ru machine](../examples/k3s/cloudru-evolution/README.md).

## **Step 3: Provision the Parent Stack**

```sh
sc provision -s devops
```

`sc provision` takes **no `-e`** — it rejects the flag. The environment comes from the
keys under `resources:`.

Measured on the live smoke (2026-10-24, 2 vCPU / 4 GiB, `ru.AZ-1`):

| step | time |
|---|---|
| VM created and `running` | ~45 s |
| the whole `vm` resource (11 resources) | ~1 min |
| `+ k3s installed`, `all nodes ready` | **7 m 33 s** |

> **Read provision output through a redactor, not with your eyes.** Four secret
> carriers pass through it on these paths: the generated deploy key, the registry
> credential, the k3s join token and the admin kubeconfig. Pipe it through `grep -c`,
> not a bare `grep`, and not a terminal you will scroll back through.

---

# **Deploying Services**

## **Serverless — a `single-image` client stack**

```yaml
---
# File: "myproject/.sc/stacks/myservice/client.yaml"

schemaVersion: 1.0

stacks:
  staging:
    type: single-image
    template: cloudru-app
    parent: myproject/devops
    config:
      image: { dockerfile: ${git:root}/Dockerfile }
      maxMemory: 256       # default 256 MB
      timeout: 10          # default 10 s
      env: { LOG_LEVEL: info }
      secrets:
        SOME_TOKEN: "${secret:MY_SERVICE_TOKEN}"
      cloudExtras:
        containerPort: 8080   # the one field a stack moving to this cloud must add
```

Three traps on this path, each of which fails in a way that does not name itself.
The full `cloudExtras` reference is in
[Supported Resources](../reference/supported-resources.md#cloudru-provider); the ones
that cost a round trip:

- **`PORT` is a reserved environment-variable name.** Setting it is not a warning and
  not a per-variable skip: the create/update call fails and **no revision is
  published** — `The container env variable name 'PORT' is reserved`. Use
  `cloudExtras.containerPort`. (Same reservation as YC Serverless Containers; the
  opposite of the `vm-compose` path, where SC passes `PORT` and it is ordinary.)
- **Names are regex-checked** against `^[a-z][a-z0-9-]{3,48}[a-z0-9]$` — 5 to 50
  characters. The five-character minimum catches the obvious container name `app`.
- **An unprivileged container cannot bind a low port or write to a root-owned path in
  its own image.** Plain `nginx` CrashLoops on
  `mkdir("/var/cache/nginx/client_temp") failed (13: Permission denied)`. Use an image
  that runs as a normal user, or grant `cloudExtras.privileged: true` deliberately — SC
  will not default it on, because that would hand root to every image it ever deploys
  here. `privileged` is also **create-only**: the platform's patch request has no such
  field, so changing it on a live app is a destroy-and-recreate.

Cold start after scale-to-zero is **~3.1 s** against ~0.68 s warm, so anything
interactive wants `scale.min: 1` rather than a longer `idleTimeout`.

## **One machine — the same `single-image` stack**

Point `template:` at `compose-on-vm` and nothing else changes. That is the intent: the
client stack never learns which cloud it lands in, which estate, or whether SC created
the machine.

## **Kubernetes — a `cloud-compose` stack**

`cloud-compose`, and it is not a free choice: `k3s-cloudrun` registers a cloud-compose
converter only. No Kubernetes template in SC accepts a `single-image` client stack
today, so a one-service compose file is how a single-container workload reaches a
cluster.

## **Deploy**

```sh
sc deploy -s myservice -e staging
```

---

# **Verifying the Deploy**

**Serverless** — the URL is a stack output:

```sh
sc stack outputs -s myservice -e staging    # <stack>-<env>-container-app-url
```

**One machine** — `<stack>-vm-compose-url` on the client stack, and the machine's
address is `<vm-resource>-vm-host-0-address` on the parent.

**Kubernetes** — the admin kubeconfig is a **secret output of the parent stack**, under
`<cluster-resource>--<env>-kubeconfig`:

```sh
umask 077
sc stack outputs -s devops > /tmp/outputs.txt     # NOTE: no -e on the parent stack
```

There is no `--show-secrets` and no `-o`: secret-marked outputs are printed **in the
clear, always**. Hence the `umask`. Redirect to a file you control, keep it out of your
scrollback and your shell history, and delete it when you are done.

## **The ports, and who opens them**

This used to be the most practical thing to know before a VM or k3s smoke, because SC
opened **22** plus the ingress's 80/443 and nothing else — so the k3s API port, the
NodePort range and a `vm-compose` host port had to be added in the console by hand. The
failure mode was the expensive kind: `sc provision` and `sc deploy` both report success,
`curl` from the node itself answers 200, `readyReplicas: 1`, and from outside the request
simply times out. **No log line anywhere says a port is shut.**

SC now derives the list instead. Three sources, in order of how little you have to say:

1. **Always** — 22/tcp.
2. **`ingress.enable: true`** — 80/tcp, 443/tcp, 443/udp (HTTP/3).
3. **Inferred from this machine's consumers in the same parent stack.** A `k3s-cluster`
   resource or a `vm-compose` template that names this `vm` resource is a declaration
   that those ports are needed, so SC reads it rather than asking you to repeat it:

   | port range | opened for | scope |
   |---|---|---|
   | `6443/tcp` | the k3s API — the exported kubeconfig points at the node, and `sc deploy` connects from wherever it runs | internet |
   | `30000-32767/tcp` | reaching a Service by node port | internet |
   | `20000-30000/tcp` | the host ports a `vm-compose` stack publishes without a `domain:` | internet |
   | `8472/udp` | flannel VXLAN (multi-node k3s) | the node's own subnet |
   | `10250/tcp` | kubelet — `kubectl logs`, `exec`, metrics | the node's own subnet |
   | `2379-2380/tcp` | etcd client + peer (embedded, multi-server k3s) | the node's own subnet |

4. **Anything inference cannot see** — a port a process of yours binds on the machine —
   goes on the `vm` resource and is opened verbatim:

   ```yaml
   ingressPorts:
     - port: 9100              # node_exporter
     - port: 7946
       protocol: udp
     - port: 32000
       portEnd: 32100
     - preset: http            # 80 + 443, without SC's own Caddy in front
   ```

   `preset` names a bundle (`vm-compose`, `k3s`, `http`) and is mutually exclusive with
   `port`. An unknown preset or an out-of-range port is **refused at validation**, not
   accepted and dropped.

Two things are specific to this cloud, and both apply to either driver — they resolve the
list with the same code, so they cannot drift apart:

- **A node-to-node range is permitted from the machine's own subnet, never from the
  internet.** SC does not choose the subnet on Evolution, so it resolves the CIDR after
  the create; if that lookup fails it **says so and leaves the port shut** rather than
  widening it to `0.0.0.0/0`.
- **A destroy closes only its own rules.** The project's security group is shared
  (Cloud.ru marks it `default: true`), so SC's rules carry a description naming the VM and
  the destroy sweep matches on that. Rules you added in the console are left alone — and
  are still permanent, so remove those yourself.

Check rather than assume: the resolved list is a stack output.

```sh
sc stack outputs -s devops | grep -- '-vm-open-ports'
```

A `vm-compose` deploy reads the same value and warns when the host port it publishes is
not among them.

---

# **DNS and domains**

SC has a `cloudru-dns` registrar for Cloud DNS, and it is **live-verified** against a
delegated apex: records are created, updated (including the TTL), and deleted from a
deploy, and the names resolve on the public internet. The service account needs the
**`cdns.admin`** role in the project — grant it under *Роли в проектах*, not at the
account level. See
[DNS Management](dns-management.md#4-cloudru-cloud-dns-registrar-cloudru-dns) for the
field shape and the two traps that matter: a record `name` is a label **relative** to
the zone (the apex is an empty name), and Cloud DNS has **no per-record route** — every
write replaces the zone's whole record set, so sibling records in one zone must be
serialised. SC does both for you; a hand-rolled script against this API will not.

What the registrar does and does not cover:

- **VM / k3s** — covered. A `domain:` on one of these paths becomes an A record at the
  machine's address, **not proxied**. The node answers the ACME challenge itself, so
  Cloudflare's orange cloud breaks it — which is why `domainProxied` is refused on these
  paths rather than accepted and ignored.
- **Serverless** — **not** covered, deliberately. A custom `domain:` on a Container App is
  attached by a field on the app itself and **requires** `cloudExtras.certificateId` from
  Cloud.ru's certificate service, which SC does not yet manage; the platform wildcard
  covers only its own `*.containerapps.ru` zone and there is no ACME on that path. So
  `ProvisionDomainForEndpoint` refuses outright instead of writing a CNAME that would
  answer with somebody else's certificate.
- On a `.ru` domain opened from inside Russia in a browser without foreign roots, read
  `ingress.acmeCa` (plus `acmeEabKeyId` / `acmeEabMacKey`) before relying on Let's
  Encrypt. The field exists so there is an option; whether a Russian Trusted CA speaks
  ACME is **unverified**.

---

# **Tearing It Down**

```sh
yes Y | sc destroy -s myservice -e staging
yes Y | sc destroy -s devops
```

> **`sc destroy` has no `--yes`, and it exits 0 on an unanswered prompt.** A scripted
> destroy that forgets to answer reports success and deletes nothing. Pipe `yes Y` into
> it, then **verify against Cloud.ru's own API or console** that the VM, its boot disk
> and its floating IP are gone. A floating IP is billed whether or not it is attached.

SC names the disk `<vm>-disk` and the address `<vm>-ip`, so a half-failed create is
still identifiable and still destroyable.

---

# **Summary**

| | Status |
|---|---|
| Container Apps (`cloudru-container-app`) | live-verified |
| `vm` + `vm-compose`, both drivers | live-verified, including a VM SC created itself |
| `vm` + `k3s-cluster` + `k3s-cloudrun` | live-verified |
| `cloudru-dns` registrar | live-verified on a delegated apex — needs the `cdns.admin` role in the project |
| Service ports beyond 22/80/443 | inferred from the consumers, or listed in `ingressPorts` — see above |
| Pulumi state in Cloud.ru | **no backend** — use `fs` or a bucket in another cloud |
| Advanced: SWR registry, EVS CSI for k3s | bridged in the provider, not wired into SC |

## **Related**

- [Supported Resources](../reference/supported-resources.md#cloudru-provider) — every
  Cloud.ru type, field and output.
- [Compose on a Cloud.ru machine](../examples/vm-compose/cloudru-evolution/README.md)
  — the `vm` resource annotated field by field.
- [k3s on a Cloud.ru machine](../examples/k3s/cloudru-evolution/README.md).
- [DNS Management](dns-management.md) — the `cloudru-dns` registrar.
- [Yandex Cloud](parent-yandex-cloud.md) — the other Russian cloud SC drives, and the
  one with managed DNS + an API Gateway edge that works today.

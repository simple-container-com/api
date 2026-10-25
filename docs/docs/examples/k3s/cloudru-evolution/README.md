# k3s on a Cloud.ru Machine SC Creates (`cloudru-evolution-vm`)

Declare a machine in `server.yaml`, and `sc provision` creates it in Cloud.ru
**Evolution**, installs a single-node Kubernetes cluster on it, and exports a
kubeconfig. Client stacks then deploy onto that cluster through SC's ordinary
Kubernetes path.

This is the full chain with nothing done by hand in a console:

```
sc provision  →  VM created  →  docker + k3s installed  →  kubeconfig exported
sc deploy     →  image pushed  →  Deployment + Service in the cluster
```

Use it when the cloud has no managed Kubernetes — which is the case on both Cloud.ru
estates, and is why this path exists. The alternative when you want to own the machine
yourself is [the `byo` version of this example](../byo-host/README.md), which is the
same two resources with `provider: byo`.

## Which Cloud.ru this is

Cloud.ru sells two unrelated platforms under one brand, one console and one bill. A key
pair from one does not authenticate against the other.

| | **Evolution** (this example) | **Advanced** (ex-SberCloud) |
|---|---|---|
| SC driver | `cloudru-evolution-vm` | `cloudru-vm` |
| Credential | service-account key id + secret | Keystone access key + secret |
| SC auth type | `cloudru-evolution-key` | `cloudru-access-key` |
| Regions | **none** — `region:` is refused | `region: ru-moscow-1` required |
| cloud-init | **no** — see below | yes |
| Registry for the kubelet to pull from | Artifact Registry, `<name>.cr.cloud.ru` | SWR (not wired into SC yet) |

Everything in this example works the same on `cloudru-vm`; the differences are the
credential type, `region:` being required rather than refused, and cloud-init existing
(so SC pins the host key itself and `hostKey:` is refused there).

## What is actually new here

Almost nothing above the kubeconfig, and that is the design. A cluster's contract with
everything above it is two stack outputs — a kubeconfig and an ingress address — so
`k3s-cluster` produces the same two outputs `gcp-gke-autopilot` does, under the same
names. The Deployment, the Service, `cloudExtras` (probes, VPA, topology spread,
security context), the helm operators: existing Kubernetes code, unchanged.

What is new is only that the machine under it is now also SC's:

| resource | what it does |
|---|---|
| `type: vm`, `provider: cloudru-evolution-vm` | creates a disk, a VM, a floating IP; opens its ingress rules; generates a deploy key; installs docker over SSH |
| `type: k3s-cluster` | installs k3s on that machine, exports the kubeconfig + the ingress address |
| `type: k3s-cloudrun` template | deploys a `cloud-compose` client stack through that kubeconfig |

`k3s-cloudrun` exists rather than `kubernetes-cloudrun` for one reason: that template
takes its kubeconfig as a literal string in config, and a cluster SC is about to create
has no kubeconfig to write down. It names the cluster resource instead. Same shape as
`gcp-gke-autopilot`'s `gkeClusterResource`.

## Files

| File | Goes to | What it is |
|---|---|---|
| `server.yaml` | `.sc/stacks/mycompany/server.yaml` | the parent stack: the `vm` resource, the `k3s-cluster` on it, the `k3s-cloudrun` template |
| `client.yaml` | `.sc/stacks/portal/client.yaml` | the application stack — an ordinary `type: cloud-compose` config |
| `docker-compose.yaml` | the repo root | the workload itself, the same compose file you run locally |

The directory name is the stack name (`-s mycompany`, `-s portal`); the keys under
`stacks:` in `client.yaml` are environments (`-e staging`).

Field-by-field annotation of the `vm` resource lives in
[the vm-compose Evolution example](../../vm-compose/cloudru-evolution/README.md) —
`server.yaml` here only comments what differs for k3s.

## Credentials

Two things are needed, and on this cloud they are the **same key**: the Evolution
service-account key authenticates the compute REST API *and* is the docker login for
the project's Artifact Registry. There is deliberately no separate registry credential.

```yaml
# .sc/stacks/mycompany/secrets.yaml   — gitignored; `sc secrets reveal -F` to edit
auth:
  cloudru:
    type: cloudru-evolution-key
    config:
      keyId: "a1b2c3d4-...."
      keySecret: "...."
      projectId: "11111111-2222-3333-4444-555555555555"
values:
  SC_STATE_PASSPHRASE: "a long random string"
  # The same key id/secret again, as the registry login. `${secret:...}` is not
  # expanded inside the `auth:` block, which is why these are two separate entries
  # rather than one referenced twice.
  CLOUDRU_KEY_ID: "a1b2c3d4-...."
  CLOUDRU_KEY_SECRET: "...."
  staging-portal-mongo-uri: "mongodb+srv://..."
```

> **Write the values verbatim.** `${secret:...}` placeholders are **not** expanded
> inside an `auth:` block. SC refuses an unexpanded placeholder outright rather than
> sending it to IAM, which would answer
> `key_id does not match required format` halfway through a deploy.

> **`projectId` is required and is not inferable from the key.** Every Evolution
> endpoint is project-scoped; a call that omits it answers **403 `non_authorized`** —
> which reads exactly like a missing role and sends you off granting permissions that
> were never the problem.

Edit the file, then re-encrypt before any commit:

```bash
sc secrets reveal -F
$EDITOR .sc/stacks/mycompany/secrets.yaml
sc secrets hide -F          # do this BEFORE you commit anything
```

## The registry has to exist first

`dockerRegistryURL` is required on `k3s-cloudrun` and refused when empty, because a
kubelet can only **pull**. The `vm-compose` path can push an image over the same SSH
connection (`imageDelivery: ssh-tar`); Kubernetes has no equivalent, and surfacing that
at config time is better than at image-pull time.

**SC does not create the registry on this path.** The `cloudru-container-app` template
creates one from its `registryName`; `k3s-cloudrun` does not. Either create it in the
Cloud.ru console, or deploy a Container App stack first and reuse the registry it made.
Then name it as `<registry-name>.cr.cloud.ru`.

A registry inside the same cloud is also the only pull path here that does not depend
on egress to anywhere. From inside Russia, a kubelet pulling `mongo:7` from Docker Hub
is a real question rather than a formality — and its failure mode is a pod in
`ImagePullBackOff` while every SC-visible signal stays green.

## Provision and deploy

```bash
sc provision -s mycompany              # creates the VM, installs docker + k3s
sc deploy    -s portal    -e staging   # pushes the image, deploys into the cluster
```

`sc provision` takes **no `-e`** — it rejects the flag. The environment comes from the
keys under `resources:` in `server.yaml`.

Measured on the live smoke (2026-10-24, 2 vCPU / 4 GiB, `ru.AZ-1`):

| step | time |
|---|---|
| VM created and `running` | ~45 s |
| docker installed over SSH | ~1 min |
| k3s installed, `all nodes ready` | **7 m 33 s** total for `sc provision` |
| `sc deploy` to `readyReplicas: 1` | ~2 min |

> **Read provision output through a redactor, not with your eyes.** Four secret
> carriers pass through it: the generated deploy key, the registry credential, the k3s
> join token and the admin kubeconfig. `sc provision … 2>&1 | grep -c ERROR` — not a
> bare `grep`, and not a terminal you will scroll back through.

## Pin the host key after the first provision

Evolution's `POST /vms` refuses `user_data` — every spelling of it (measured
2026-10-24: HTTP 422 `extra_forbidden`). So SC cannot seed an SSH host key into the
machine and cannot read the real one back from the API either. The first connection has
nothing to verify, and on this path that connection carries the deploy key and brings
the cluster's **admin kubeconfig** back out.

Close it after the first provision:

```bash
# the machine's address is a stack output: <vm-resource>-vm-host-0-address
umask 077 && sc stack outputs -s mycompany > /tmp/sc-outputs.txt
ssh-keyscan -t ed25519 <the address from that file>
```

or read the key off the machine's console output, and paste the line into `hostKey:` in
`server.yaml`. The next provision pins it. SC warns loudly while it is unset.

## The ports SC does not open

This is the one thing that will cost you a confused afternoon, and it is a known gap
rather than a configuration mistake.

The Cloud.ru VM driver opens **22**, plus **80/443/443-udp** when `ingress.enable:
true`. **Nothing else** — not the Kubernetes API port, not the NodePort range. And
`server.yaml` here leaves `ingress` off on purpose, so k3s can own 80/443.

The failure mode is the expensive kind: `sc provision` and `sc deploy` both report
success, `curl` from the node itself answers 200, `readyReplicas: 1`, and from outside
the request simply times out. No log line anywhere says a port is shut.

Until SC opens them itself, add these to the project's security group in the Cloud.ru
console:

| port range | needed for |
|---|---|
| `6443` | the kubeconfig SC exports — `kubectl`, and the Kubernetes provider `sc deploy` runs |
| `30000-32767` | the NodePort range, if you reach a Service by node port |
| `80`, `443` | only if you use `cloudExtras.serviceType: LoadBalancer` with `ingress` off, as this example does |

6443 is not optional on this path: the exported kubeconfig points at the machine's
floating address, and `sc deploy` connects to the API server from wherever it runs — a
CI runner, not the machine. Without that rule the deploy itself cannot reach the
cluster.

Remove the rules when you are done smoking. The project's security group is shared
(Cloud.ru marks it `default: true`), and SC has no `DeleteSecurityGroupRule` yet, so
anything left behind is permanent.

## Checking the cluster by hand

The kubeconfig is a secret stack output of the **parent** stack:

```bash
umask 077
sc stack outputs -s mycompany > /tmp/k3s-outputs.txt
```

Three things about that command, each of which costs a round trip to discover:

- It is `sc stack outputs`, not `sc outputs`.
- **Omit `-e`.** Passing an environment switches it to the child-stack reader, which
  then fails looking for `client.yaml` in the parent stack's directory.
- There is no `--show-secrets` and no `-o`. Secret-marked outputs are printed **in the
  clear**, always — hence the `umask`. Redirect to a file you control and keep it out of
  your scrollback, your shell history and any handoff.

The kubeconfig's key is `<cluster-resource>--<env>-kubeconfig`, so
`ru-cluster--staging-kubeconfig` here. Pull it into its own file:

```bash
chmod 600 /tmp/k3s.kubeconfig
KUBECONFIG=/tmp/k3s.kubeconfig kubectl get nodes
```

It is a **cluster-admin** credential with an embedded client key, and reaching the
cluster with it needs 6443 open (above). Delete both files when you are done, and do not
commit them or paste them into a handoff.

## What SC creates, and what it will not

Created on `sc provision`, and destroyed on `sc destroy`:

| | name | note |
|---|---|---|
| boot disk | `<vm>-disk` | from the mapped image; 30 GiB if `size.disk` is unset |
| VM | `<vm>` | on the availability zone's **default** subnet and **default** security group |
| floating IP | `<vm>-ip` | allocated, then attached after create — this cloud takes no address at create time |
| ingress rules | on whichever security group the machine joined | read back off the machine's own interface |
| deploy key | — | generated per stack, marked secret, in the Pulumi checkpoint |
| k3s | — | installed over SSH; the join token and kubeconfig are marked secret |

Refused rather than accepted and ignored:

| field | why |
|---|---|
| `region` | this cloud has none; placement is the zone |
| `hosts` | that is `provider: byo` |
| `network` | the create call rejects `subnet_id`/`networks`/`nics` — the machine lands on the zone's default |
| `extraSshKeys` | extra keys are authorised through user-data, which this cloud has none of; the create call takes exactly one key |
| more than one zone | this is a single machine |
| `publicIP: false` | every delivery step is an outbound SSH connection, and SC has no bastion |

To place a machine on your own network or security group, create it in the console and
declare it with `provider: byo` — [that example](../byo-host/README.md) is otherwise
identical.

## Known limits of a single node

Stated plainly, because `scale:` will happily let you believe otherwise:

- **One failure domain.** Replicas on one node survive a pod crash, not a machine
  reboot. `scale: {min: 1, max: 1}` is honesty, not a technical limit.
- **No multi-node yet.** `vmResource` resolves to exactly one host today; more than one
  is refused rather than silently using the first.
- **`local-storage` is the default storage class.** A PersistentVolume is a directory on
  that one machine, and it does not survive the machine. Cloud.ru's own block storage is
  not wired to a CSI driver here.
- **A destroy/provision cycle moves the address.** This driver allocates a new floating
  IP, so every saved kubeconfig and every DNS record points at the old one. Set
  `endpoint:` to a DNS name if that matters.
- **The control plane is pinned**, not tracking a release channel. Move it deliberately
  with `version:`.

## Teardown

```bash
yes Y | sc destroy -s portal -e staging
yes Y | sc destroy -s mycompany
```

> `sc destroy` has **no `--yes`**, and it **exits 0 on an unanswered prompt** — so a
> scripted destroy that forgets to answer reports success and deletes nothing. Pipe `yes
> Y` into it, then verify against Cloud.ru's own API or console that the VM, the disk and
> the floating IP are gone. A floating IP is billed whether or not it is attached.

## Related

- [Compose on a Cloud.ru machine](../../vm-compose/cloudru-evolution/README.md) — the
  same machine without Kubernetes, and where every `vm` field is annotated in full.
- [k3s on a machine you brought](../byo-host/README.md) — the same two resources with
  `provider: byo`, for a machine you created yourself or a cloud SC does not drive.
- [GKE Autopilot](../../gke-autopilot/index.md) — the same client stacks on managed
  Kubernetes. The client config is interchangeable.
- [Supported resources](../../../reference/supported-resources.md) — the Cloud.ru
  section, including the Container App (serverless) path.

# k3s on a Machine You Brought (`byo`)

Stand up a single-node Kubernetes cluster on one virtual machine over SSH, and deploy
client stacks onto it through SC's ordinary Kubernetes path.

Use this when the cloud you are deploying into has no managed Kubernetes — which is the
case across most of the Russian market, and is why this path exists. It is also the
cheapest way to get a real Kubernetes API in a cloud SC does not drive yet: you create
the machine in the console, declare it with `provider: byo`, and `k3s-cluster` installs
and exports a kubeconfig from there.

## What is actually new here

Almost nothing, and that is the point. A cluster's contract with everything above it is
two stack outputs — a kubeconfig and an ingress address — so `k3s-cluster` produces the
same two outputs `gcp-gke-autopilot` does, under the same names. The Deployment, the
Service, `cloudExtras` (probes, VPA, topology spread, security context), the helm
operators and the Caddy edge are the existing Kubernetes code, unchanged.

The one addition is the template type:

| | takes its kubeconfig from | use when |
|---|---|---|
| `kubernetes-cloudrun` | a literal `kubeconfig:` in the template | you already have a cluster and its kubeconfig |
| `k3s-cloudrun` | the named `k3s-cluster` resource's output | SC provisions the cluster in the same parent stack |

They differ in that and nothing else. A kubeconfig for a cluster SC is about to create
cannot be written into a config file, which is the whole reason for the second one.

## Files

| File | Goes to | What it is |
|---|---|---|
| `server.yaml` | `.sc/stacks/mycompany/server.yaml` | the parent stack: a `type: vm` resource (`byo`), a `k3s-cluster` on it, and the `k3s-cloudrun` template |
| `client.yaml` | `.sc/stacks/portal/client.yaml` | the application stack — an ordinary `type: cloud-compose` config |
| `docker-compose.yaml` | the repo root | the workload itself, the same compose file you run locally |

The directory name is the stack name (`-s mycompany`, `-s portal`); the keys under
`stacks:` in `client.yaml` are environments (`-e staging`).

## Prepare the machine

Nothing in SC creates it. In your cloud's console:

1. Create an Ubuntu 24.04 VM with a public IP. Reachable ports: **22** (deploys),
   **6443** (the Kubernetes API, from wherever `sc deploy` runs — a CI runner, not the
   machine), and **80/443** if you intend to serve traffic from the node.
2. Create a login for deploys and authorise a key pair generated for this purpose only:

   ```bash
   ssh-keygen -t ed25519 -f ./cluster_vm_deploy_key -N ''
   # add cluster_vm_deploy_key.pub to ~/.ssh/authorized_keys on the machine
   ```

   The user needs passwordless `sudo`: installing k3s is a root operation.

3. Record the host key, and compare it against the machine's console output before you
   trust it:

   ```bash
   ssh-keyscan -t ed25519 203.0.113.10
   ```

   A deploy carries the stack's whole env file and every `${secret:...}` in it, so this
   is the field whose absence has a real cost. Reading the key off the network is what
   it protects you from.

Docker is **not** needed on the machine — k3s brings containerd. Images are pulled from
the registry named in the template.

## Secrets

Four `${secret:...}` values are referenced by `server.yaml`:
`CLUSTER_VM_DEPLOY_KEY`, `RU_REGISTRY_USERNAME`, `RU_REGISTRY_PASSWORD` and
`SC_STATE_PASSPHRASE`. They live under `values:` in the parent stack's
`.sc/stacks/mycompany/secrets.yaml`, and **they are set by editing that file** —
there is no command that sets one. (`sc secrets add` is a different thing: it
registers a *file* in the encrypted registry. See
[Secrets Management](../../../guides/secrets-management.md).)

```bash
sc secrets reveal -F         # decrypt the registry in place
$EDITOR .sc/stacks/mycompany/secrets.yaml
sc secrets hide -F           # re-encrypt — do this BEFORE any commit
```

The deploy key is a PEM, so it has to be a YAML block scalar. Mind the
indentation: every line of the key is indented under the `|` and nothing else is.

```yaml
# .sc/stacks/mycompany/secrets.yaml
values:
  SC_STATE_PASSPHRASE: "a long random string"
  RU_REGISTRY_USERNAME: "robot$mycompany"
  RU_REGISTRY_PASSWORD: "..."
  CLUSTER_VM_DEPLOY_KEY: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gt
    ...
    -----END OPENSSH PRIVATE KEY-----
```

Confirm a value landed without printing the whole file:

```bash
sc stack secret-get CLUSTER_VM_DEPLOY_KEY -s mycompany | head -1
```

`sc secrets reveal` leaves plaintext on disk until `sc secrets hide` runs.
`secrets.yaml` is gitignored either way, but an interrupted edit leaves it
decrypted — re-run `sc secrets hide -F` before you commit anything.

## Provision and deploy

```bash
sc provision -s mycompany -e staging   # creates the cluster, exports the kubeconfig
sc deploy    -s portal    -e staging   # deploys onto it
```

A registry the machine can reach is **required**: a kubelet can only pull. The
vm-compose path can push an image over SSH with `imageDelivery: ssh-tar`; Kubernetes
cannot, and `dockerRegistryURL` is refused when empty rather than failing later at
image-pull time.

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
  clear**, always — hence the `umask` above. Redirect to a file you control and keep it
  out of your scrollback, your shell history and any handoff.

Then pull the kubeconfig out of it (the key is `<cluster-resource>--<env>-kubeconfig`,
so `ru-cluster--staging-kubeconfig` here) into its own file:

```bash
chmod 600 /tmp/k3s.kubeconfig
KUBECONFIG=/tmp/k3s.kubeconfig kubectl get nodes
```

It is a **cluster-admin** credential with an embedded client key. Delete both files when
you are done, and do not commit them or paste them into a handoff.

## The front door: one machine has one port 443

This is the only genuinely new coordination problem, and it is a port conflict rather
than anything conceptual. A k3s node wants 80/443 for its bundled LoadBalancer
implementation (klipper-lb, k3s's `servicelb`); the vm-compose path's Caddy container
wants the same two.

- **No `ingress` on the vm resource** (what `server.yaml` does) — k3s keeps the ports,
  `servicelb` stays enabled, and a Service with `cloudExtras.serviceType: LoadBalancer`
  binds them on the node.
- **`ingress.enable: true`** — the compose Caddy takes them, and SC disables `servicelb`
  automatically. Without that, every `LoadBalancer` Service would sit `<pending>`
  forever with nothing explaining why.

You do not coordinate this by hand: the `k3s-cluster` resource reads the machine's
ingress setting from the vm resource's output and adjusts. What you choose is which
arrangement you want.

There is a third contender for those ports that is neither of the above: **k3s's own
bundled traefik**, which SC disables by default for exactly this reason (`disable:` in
`server.yaml`). Leave that default alone unless you mean to run traefik as your edge —
with it installed, klipper-lb cannot bind 80/443, and the symptom is not an error but a
`Pending` `svclb-<service>` pod and a Service reporting `EXTERNAL-IP <pending>`
indefinitely.

The in-cluster Caddy edge is a third option and is **not wireable yet** — the
`kubernetes-caddy` resource takes its kubeconfig as a literal string, so it cannot be
pointed at a cluster provisioned in the same stack. `server.yaml` says so where you
would reach for it.

## Known limits of a single node

Stated plainly, because `scale:` will happily let you believe otherwise:

- **One failure domain.** Replicas on one node survive a pod crash, not a machine
  reboot. `scale: {min: 1, max: 1}` in `client.yaml` is honesty, not a technical limit.
- **No multi-node yet.** `vmResource` resolves to exactly one host today; more than one
  is refused rather than silently using the first.
- **`local-storage` is the default storage class.** A PersistentVolume is a directory
  on that one machine. It does not survive the machine.
- **The control plane is pinned**, not tracking a release channel — k3s's installer
  resolves a channel at install time, which would otherwise make the deployed version
  depend on the day the deploy ran. Move it deliberately with `version:`.

## Related

- [Compose on a VM](../../vm-compose/index.md) — the same machine without Kubernetes,
  and where the `vm` resource's fields are annotated in full.
- [GKE Autopilot](../../gke-autopilot/index.md) — the same client stacks on managed
  Kubernetes. The client config is interchangeable.

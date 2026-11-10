# k3s Cluster Examples

Standing up a Kubernetes cluster on machines you have, rather than consuming one a cloud
offers, and then deploying onto it through Simple Container's existing Kubernetes path.

Use this path when the cloud you are deploying into has no managed Kubernetes — which
describes most of the Russian market, and is the reason this exists — or when a managed
cluster is more cluster than the workload needs.

## Available Examples

### [k3s on a Machine You Brought (`byo`)](byo-host/README.md)

One VM created by hand in a cloud console, declared with `provider: byo`, running a
single-node k3s cluster and a `type: cloud-compose` client stack on it. The same file
with more `hosts` entries — each naming a `role`, plus a node-to-node address from
either `privateAddress` or `mesh` — is a multi-node cluster.

**Use case:** the first Kubernetes deployment into a cloud SC does not drive yet;
small services that want the Kubernetes API without a managed control plane's price.

```yaml
# .sc/stacks/mycompany/server.yaml  (excerpt)
templates:
  k3s-run:
    type: k3s-cloudrun
    config:
      k3sClusterResource: ru-cluster
      dockerRegistryURL: registry.example.ru/mycompany

resources:
  resources:
    staging:
      template: k3s-run
      resources:
        cluster-vm:
          type: vm
          config:
            provider: byo
            hosts:
              - address: 203.0.113.10
                user: ubuntu
                privateKey: ${secret:CLUSTER_VM_DEPLOY_KEY}
                hostKey: "203.0.113.10 ssh-ed25519 AAAAC3Nza…"

        ru-cluster:
          type: k3s-cluster
          config:
            vmResource: cluster-vm
```

### [k3s on a Cloud.ru Machine SC Creates (`cloudru-evolution-vm`)](cloudru-evolution/README.md)

The same two resources, except SC creates the machine too: `sc provision` makes the VM
in Cloud.ru Evolution, installs docker and k3s on it, and exports the kubeconfig. The
only console step left is pinning the host key — this cloud runs no cloud-init, so SC
cannot seed one.

**Use case:** a Kubernetes stand in a Russian cloud, end to end from config.

```yaml
# .sc/stacks/mycompany/server.yaml  (excerpt)
        cluster-vm:
          type: vm
          config:
            provider: cloudru-evolution-vm
            credentials: ${auth:cloudru}
            zones: [ru.AZ-1]        # no `region:` — this cloud has none
            publicIP: true
            image: ubuntu-2404-lts
            size: { cores: 2, memory: 4Gi, disk: 40Gi }

        ru-cluster:
          type: k3s-cluster
          config:
            vmResource: cluster-vm
```

Measured end to end on 2026-10-24: `sc provision` 7 m 33 s from nothing to
`all nodes ready`. The firewall needs nothing from you: a `k3s-cluster` naming the `vm`
resource is a declaration SC reads, so provisioning opens 6443 and the NodePort range to
the internet and the node-to-node ports (flannel, the kubelet, etcd) to the machine's
own subnet only. The example's README has the table, and `ingressPorts:` on the `vm`
resource is how you add anything else.

`provider: cloudru-vm` (the Advanced / ex-SberCloud estate) is the same file with a
different credential type and `region: ru-moscow-1` required rather than refused.

## How the pieces divide up

The machine is described by a `vm` resource — the same one the
[vm-compose](../vm-compose/index.md) path uses, with the same fields. `k3s-cluster`
names that machine and installs on it. The template names the cluster. Each layer
references the one below it **by name**, which is the split `gcp-gke-autopilot` uses for
`gkeClusterResource`.

| Belongs to the `vm` resource | Belongs to `k3s-cluster` | Belongs to the `k3s-cloudrun` template |
|---|---|---|
| `provider`, `hosts`/`credentials`, `sshUser` | `vmResource` | `k3sClusterResource` |
| `region`, `zones`, `image`, `size` | `version`, `endpoint`, `tlsSans` | `dockerRegistryURL` / username / password |
| `ingress` (one Caddy per machine; one machine, one port 443) | `disable`, `extraServerArgs` | `caddyResource`, `clientTls`, `originTls` |
| `hosts[].role`, `hosts[].privateAddress`, `mesh` | `servers` | |

Two of those are easy to confuse, and one of them used to be called `useSSL`:

| Field | Where | What it decides |
|---|---|---|
| `clientTls` (was `useSSL`) | `k3s-cloudrun` template, or the `kubernetes-caddy` resource | Clients reach us over HTTPS, whoever terminated it: send HSTS, and Caddy owns the HTTP→HTTPS redirect. Defaults **on**. Does *not* make anything serve TLS. |
| `originTls` | same two places, as the edge-wide default; `lbConfig.originTls` in a stack's own client.yaml to override | This edge terminates TLS itself — `https://<domain>` as the Caddy site address, so automatic HTTPS and ACME are armed. Defaults **off**: normally Cloudflare or a cloud load balancer terminates in front. |
| `ingress.originTls` | the `vm` resource | The same question for a *machine's* Caddy, which is the front door, so it defaults **on**. |

`useSSL` still works everywhere it worked before and means `clientTls`. Setting both to
different values is refused rather than resolved silently.

## More than one node

A multi-node cluster is the same two resources with more `hosts`. Each entry then names
a `role` — `k3s-server` or `k3s-agent` — the pool needs a node-to-node address, and
`k3s-cluster`'s `servers:` says how many machines run the control plane. The full shape,
annotated, is the commented block in [`byo-host/server.yaml`](byo-host/server.yaml); the
three rules worth knowing before you plan around it:

- **`role:` is all-or-none.** SC will not guess which of several machines runs etcd, and
  the guess decides whether a machine joins the cluster at all. Mixing `compose` with a
  `k3s-*` role in one `vm` resource is refused — one machine has one port 443, so a
  compose fleet and a k3s pool are two resources.
- **`servers:` defaults to 1**, which uses sqlite and has no etcd: losing the server
  loses the control plane but not the running workloads. Above 1 it opts into embedded
  etcd, and must be **odd** and no larger than the host count. etcd wants low RTT —
  across availability zones in one region is the intended topology, not across regions
  or hosters.
- **The pool needs a node-to-node address**, from `privateAddress` on every host or
  from `mesh.enable: true` on the resource — and the absence of both is refused rather
  than warned about, because the failure mode is a *success*. SC passes the address as
  `--node-ip`; with no value k3s advertises whichever address the machine's default
  route carries — the public one on a single-NIC VPS — putting etcd peer traffic and
  flannel's unencrypted VXLAN overlay on the public internet, on a `byo` host where SC
  owns no firewall. The cluster still comes up `Ready` with nothing saying so.

**If your machines share no private network** — a lot of cheap VPS hosting has none —
set `mesh.enable: true` on the `vm` resource. SC builds a WireGuard overlay across the
pool, generates each machine's key on the machine, and addresses every node on the
overlay: the join URL, `--node-ip`, `--advertise-address` and the node-to-node SANs all
become `10.88.0.x` rather than a public address. The cluster below is the shape proven
live on three single-NIC VPSs from three different hosters:

```yaml
        cluster-vm:
          type: vm
          config:
            provider: byo
            mesh:
              enable: true      # 10.88.0.0/24, 51820/udp — both overridable
            hosts:
              - address: 203.0.113.10        # no privateAddress anywhere
                role: k3s-server
                user: ubuntu
                privateKey: ${secret:CLUSTER_VM_DEPLOY_KEY}
              - address: 203.0.113.11
                role: k3s-server
                user: ubuntu
                privateKey: ${secret:CLUSTER_VM_DEPLOY_KEY}
              - address: 203.0.113.12
                role: k3s-server
                user: ubuntu
                privateKey: ${secret:CLUSTER_VM_DEPLOY_KEY}

        ru-cluster:
          type: k3s-cluster
          config:
            vmResource: cluster-vm
            servers: 3          # embedded etcd across all three
```

That cluster reports `INTERNAL-IP` `10.88.0.1/.2/.3`, binds etcd's client and peer
ports to the overlay address rather than the public NIC, and survives losing a
control-plane node. Where a cloud *does* give you a private network, keep using
`privateAddress`: it is cheaper, faster, and already trusted by that cloud's firewall.
Details and the field table: [the `mesh` reference](../../reference/supported-resources.md#mesh-an-sc-managed-wireguard-overlay).

A [compose fleet](../vm-compose/index.md) remains the lighter option when you do not
need the Kubernetes API at all — it has no node-to-node traffic, so it needs neither
a private network nor a mesh.

## Why there are two Kubernetes templates

A cluster's contract with everything above it is two stack outputs — a kubeconfig and an
ingress address. `k3s-cluster` produces the same two, under the same names,
`gcp-gke-autopilot` does, so the Deployment, the Service, `cloudExtras`, the probes, the
helm operators and the Caddy edge are all the existing code.

What does not carry over is how the deploy side *obtains* that kubeconfig:

| | takes its kubeconfig from | use when |
|---|---|---|
| `kubernetes-cloudrun` | a literal `kubeconfig:` in the template config | you already have a cluster and its kubeconfig |
| `k3s-cloudrun` | the named `k3s-cluster` resource's stack output | SC provisions the cluster in the same parent stack |

A kubeconfig for a cluster that does not exist yet cannot be written into a config file.
That is the entire difference between the two; everything else about them is identical.

## Related

- [Compose on a VM](../vm-compose/index.md) — the same machine without Kubernetes, and
  where every `vm` resource field is annotated in full.
- [GKE Autopilot](../gke-autopilot/index.md) — the same client stacks on managed
  Kubernetes. The client config is interchangeable.

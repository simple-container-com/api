# k3s Cluster Examples

Standing up a Kubernetes cluster on machines you have, rather than consuming one a cloud
offers, and then deploying onto it through Simple Container's existing Kubernetes path.

Use this path when the cloud you are deploying into has no managed Kubernetes — which
describes most of the Russian market, and is the reason this exists — or when a managed
cluster is more cluster than the workload needs.

## Available Examples

### [k3s on a Machine You Brought (`byo`)](byo-host/README.md)

One VM created by hand in a cloud console, declared with `provider: byo`, running a
single-node k3s cluster and a `type: cloud-compose` client stack on it.

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

## How the pieces divide up

The machine is described by a `vm` resource — the same one the
[vm-compose](../vm-compose/index.md) path uses, with the same fields. `k3s-cluster`
names that machine and installs on it. The template names the cluster. Each layer
references the one below it **by name**, which is the split `gcp-gke-autopilot` uses for
`gkeClusterResource`.

| Belongs to the `vm` resource | Belongs to `k3s-cluster` | Belongs to the `k3s-cloudrun` template |
|---|---|---|
| `provider`, `hosts`, `sshUser` | `vmResource` | `k3sClusterResource` |
| `region`, `zones`, `image`, `size` | `version`, `endpoint`, `tlsSans` | `dockerRegistryURL` / username / password |
| `ingress` (one machine, one port 443) | `disable`, `extraServerArgs` | `caddyResource`, `useSSL` |

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

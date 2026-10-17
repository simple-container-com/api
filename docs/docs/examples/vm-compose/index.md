# Docker Compose on a VM Examples

Deploying a client stack onto one virtual machine over SSH, as a generated docker
compose project. No Kubernetes, no managed container runtime, and — with the default
`ssh-tar` image delivery — no container registry the machine has to be able to reach.

Use this path when the cloud you are deploying into has no Simple Container driver yet,
or when a single machine is simply the right size for the workload.

## Available Examples

### [A Machine You Brought (`byo`)](byo-host/README.md)

One VM created by hand in a cloud console, declared with `provider: byo`, running a
`type: single-image` client stack.

**Use case:** the first deployment into a cloud SC does not drive yet; small
single-machine services; getting off a managed runtime without adopting Kubernetes.

```yaml
# .sc/stacks/mycompany/server.yaml  (excerpt)
templates:
  compose-on-vm:
    type: vm-compose
    config:
      vmResource: portal-vm
      imageDelivery: ssh-tar     # or `registry`
      strategy: blue-green       # or `recreate`

resources:
  resources:
    staging:
      template: compose-on-vm
      resources:
        portal-vm:
          type: vm
          config:
            provider: byo
            hosts:
              - address: 203.0.113.10
                user: ubuntu
                privateKey: ${secret:PORTAL_VM_DEPLOY_KEY}
                hostKey: "203.0.113.10 ssh-ed25519 AAAAC3Nza…"
```

## How the pieces divide up

The machine is described in the parent stack's `resources:`; the `vm-compose` template
holds delivery behaviour only and references the machine **by name** — the same split
`gcp-gke-autopilot` uses for `gkeClusterResource`. That is what lets several stacks share
one machine, and lets the delivery strategy change without touching the machine
description.

| Belongs to the `vm` resource | Belongs to the `vm-compose` template |
|---|---|
| `provider`, `region`, `zones`, `image`, `size` | `vmResource` |
| `sshUser`, `hosts`, `extraSshKeys` | `imageDelivery`, `strategy`, `keepImages` |
| `ingress` (one machine, one port 443) | `dockerRegistryURL` / username / password |
| `registryMirrors`, `packageMirror` | |

## Related

- [Shipping Images Without a Registry](../../reference/image-ship-command.md) — the
  `sc image ship` command behind `imageDelivery: ssh-tar`.

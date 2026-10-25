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

### [A Machine SC Creates in Cloud.ru (`cloudru-vm`)](cloudru/README.md)

The same delivery path with the machine created by SC: `sc provision` makes the VPC, the
subnet, the security group, the floating address, the SSH key pair and the instance in
**Cloud.ru Advanced** (ex-SberCloud), then connects to what it created.

**Use case:** a single-machine deployment in Russia, where a managed container runtime
either does not exist or cannot be reached from where the images are built.

```yaml
# .sc/stacks/mycompany/server.yaml  (excerpt — the template block is unchanged)
resources:
  resources:
    staging:
      template: compose-on-vm
      resources:
        portal-vm:
          type: vm
          config:
            provider: cloudru-vm
            credentials: ${auth:cloudru}
            region: ru-moscow-1
            publicIP: true             # required: SC deploys over SSH from outside
            image: ubuntu-2404-lts
            size: {cores: 2, memory: 4Gi, disk: 60Gi, diskType: SSD}
            ingress:
              enable: true
              email: ops@example.com
```

### [A Machine SC Creates in Cloud.ru Evolution (`cloudru-evolution-vm`)](cloudru-evolution/README.md)

The other Cloud.ru estate, and a different API end to end: a service-account key instead
of a Keystone pair, **no regions at all**, and **no cloud-init** — so docker is installed
over SSH after the machine exists, and the SSH host key is the one thing you pin by hand.

**Use case:** the same single-machine deployment in Russia, on the estate most Cloud.ru
projects are actually on today.

```yaml
# .sc/stacks/mycompany/server.yaml  (excerpt — the template block is unchanged)
        portal-vm:
          type: vm
          config:
            provider: cloudru-evolution-vm
            credentials: ${auth:cloudru}
            # No `region:` — this cloud has none, and the field is refused.
            zones: [ru.AZ-1]
            publicIP: true
            image: ubuntu-2404-lts
            size: {cores: 2, memory: 4Gi, disk: 40Gi}
            # hostKey: "45.132.179.77 ssh-ed25519 AAAAC3Nza…"   # pin after first provision
            ingress:
              enable: true
              email: ops@example.ru
```

**Cloud.ru sells two unrelated estates under one brand**, and which one you are on
decides the driver, the credential and whether `region:` is legal. A key pair from one
does not authenticate against the other.

| | **Advanced** (ex-SberCloud) | **Evolution** |
|---|---|---|
| driver | `cloudru-vm` | `cloudru-evolution-vm` |
| credential | Keystone access key + secret | service-account key id + secret |
| regions | `region: ru-moscow-1` required | **none** — `region:` refused |
| cloud-init | yes (SC pins the host key itself) | no (`hostKey:` is yours to pin) |
| your own network / security group | SC creates both | refused — the zone's defaults |

## How the pieces divide up

The machine is described in the parent stack's `resources:`; the `vm-compose` template
holds delivery behaviour only and references the machine **by name** — the same split
`gcp-gke-autopilot` uses for `gkeClusterResource`. That is what lets several stacks share
one machine, and lets the delivery strategy change without touching the machine
description.

| Belongs to the `vm` resource | Belongs to the `vm-compose` template |
|---|---|
| `provider`, `credentials`, `region`, `zones` | `vmResource` |
| `image`, `size`, `publicIP`, `network` | `imageDelivery`, `strategy`, `keepImages` |
| `sshUser`, `hosts`, `extraSshKeys` | `dockerRegistryURL` / username / password |
| `ingress` (one machine, one port 443) | |
| `registryMirrors`, `packageMirror` | |

## Related

- [Shipping Images Without a Registry](../../reference/image-ship-command.md) — the
  `sc image ship` command behind `imageDelivery: ssh-tar`.
- [k3s on the same machines](../k3s/index.md) — when the workload wants a Kubernetes API
  rather than a compose project. Both Cloud.ru machines above can run one.

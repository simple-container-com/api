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

3. Install Docker and the compose plugin, and give the deploy user access to the
   socket. (With a cloud driver this is cloud-init's job; with `byo` it is yours.)

   ```bash
   curl -fsSL https://get.docker.com | sh
   sudo usermod -aG docker "$(whoami)"
   ```

4. Read the machine's host key and pin it in `server.yaml`:

   ```bash
   ssh-keyscan -t ed25519 203.0.113.10
   ```

   Compare it against what the console shows before you trust it — reading it off the
   network is the thing pinning protects you from. Deploying without `hostKey` works and
   warns: SSH then authenticates the channel but nothing authenticates the *machine*, so
   the deploy key and every secret in the stack's env file go to whoever answers on that
   address. A CI runner has no `known_hosts` to fall back on, which is why there is no
   implicit default.

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
3. gets the image onto the machine (`ssh-tar` streams it; `registry` logs in and pulls),
4. renders a compose project and brings up the inactive slot,
5. gates on the ingress service being healthy, then stops the slot that was serving,
6. prunes images beyond `keepImages`.

## Where the stack is reachable

Until the Caddy/TLS slice lands, the ingress service is published on a deterministic
host port in the 20000–30000 range, derived from the stack and environment name, and
exported as `<stack>-vm-compose-url`:

```bash
sc stack outputs -s portal -e staging | jq -r '."portal-vm-compose-url"'
# http://203.0.113.10:29763
curl -sS http://203.0.113.10:29763   # answers the application
```

Two stacks on one machine get different ports, and a collision is detected at deploy
time when docker refuses to bind — reported with both stack names, not left to be
discovered later.

## What this template refuses

Every refusal below is deliberate: a field accepted and silently ignored is a future
incident in which the operator is certain the setting applied.

| Field | Why |
|---|---|
| `scale` with `min`/`max` other than 1 | one machine, one replica; a load balancer in front of several machines is the k3s path |
| `domainProxied` | a Cloudflare-proxied domain breaks the ACME challenge the machine answers itself |
| `clusterIPAddress` | there is no cluster — the stack is reachable at the machine's address |
| a `bind` mount in a compose file | its source is a path on the developer's machine, which does not exist on the VM |
| `region`, `size`, `image`, `publicIP`, `network`, `extraSshKeys` on a `byo` resource | SC did not create the machine and cannot apply them; configure them in the console |

`timeout` and `maxEphemeralStorage` are accepted and warned about rather than refused:
they describe a function runtime that is not present here.

## Two stacks, one machine

The machine lives in the parent stack's `resources:` and the delivery behaviour in
`templates:` — the same split `gcp-gke-autopilot` uses for `gkeClusterResource`. That is
what lets a second client stack reference the same `vm` resource (its own compose
project, its own env file, its own slots) and lets you switch `strategy` or
`imageDelivery` without touching the machine description.

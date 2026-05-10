# Kubernetes Namespace Layout

Simple Container's Kubernetes deployments automatically derive the target namespace from the stack and environment, so siblings never collide in the same physical namespace. This page documents the naming rule and what to expect on deploy, redeploy, and destroy.

## The rule

Given a stack definition with `stackName` (the stack directory name) and `stackEnv` (the environment under `stacks:` in `client.yaml`), and an optional `parentEnv` (when the stack inherits from a different parent environment):

| Stack shape | `parentEnv` | `stackEnv` | Resulting namespace |
|---|---|---|---|
| **Standard stack** — same env as parent | (unset) or equals `stackEnv` | `<env>` | `<stackName>` |
| **Custom stack** — sub-env under a different parent env | set, differs from `stackEnv` | `<env>` | `<stackName>-<stackEnv>` |

The namespace name is sanitized to RFC 1123 (lowercase, `_` → `-`, ≤63 chars with FNV-1a truncation hash if needed), so callers can pass arbitrary stack names without worrying about Kubernetes naming constraints.

## Worked example

Given this `client.yaml` deploying to a parent `infrastructure/myproject` with environments `staging` and `production`:

```yaml
# .sc/stacks/myapp/client.yaml
stacks:
  staging:
    type: cloud-compose
    parent: infrastructure/myproject
    # parentEnv defaults to "staging" → standard stack
    config: { ... }

  production:
    type: cloud-compose
    parent: infrastructure/myproject
    parentEnv: production          # standard stack (parentEnv == stackEnv)
    config: { ... }

  tenant-a:
    type: cloud-compose
    parent: infrastructure/myproject
    parentEnv: production          # custom stack (parentEnv != stackEnv)
    config: { ... }

  tenant-b:
    type: cloud-compose
    parent: infrastructure/myproject
    parentEnv: production          # custom stack (parentEnv != stackEnv)
    config: { ... }
```

Deployment names → namespaces:

| `sc deploy -s myapp -e ...` | Resulting namespace |
|---|---|
| `staging` | `myapp` (in the staging cluster) |
| `production` | `myapp` (in the production cluster) |
| `tenant-a` | `myapp-tenant-a` (in the production cluster) |
| `tenant-b` | `myapp-tenant-b` (in the production cluster) |

`tenant-a` and `tenant-b` are isolated from each other and from `production`, even though they all run under the same parent infrastructure.

## Why this matters

**Destroy safety.** Each custom stack owns its own namespace, so `sc destroy -s myapp -e tenant-a` only removes resources in `myapp-tenant-a`. The parent stack and other siblings are untouched.

**Tenant isolation.** Custom stacks no longer share a namespace, so namespace-scoped RBAC, NetworkPolicy, ResourceQuota, and Secret access are isolated by default.

**Caddy routing follows automatically.** Simple Container's Caddy ingress watches `--all-namespaces` and reads the `simple-container.com/caddyfile-entry` annotation off each Service. New namespaces are picked up on Caddy's next reconcile with no manual config.

## Migrating an existing custom stack

If you have a custom stack that was deployed before Simple Container introduced the per-stackEnv namespace, the next `sc deploy` (or `pulumi up`) will move it to its dedicated namespace. The flow is automatic:

1. Pulumi sees the namespace `metadata.name` change — schedules a Replace.
2. The new namespace is created (e.g. `myapp-tenant-a`).
3. The old shared namespace is **retained** (via `RetainOnDelete`) — the parent stack and any siblings still on the old namespace keep running.
4. All namespace-scoped resources owned by this stack (Deployment, Service, Secret, ConfigMap, HPA, VPA, ImagePullSecret, Jobs, and CloudSQL/init secrets) are Replaced in the new namespace.
5. Caddy auto-rebuilds its Caddyfile from the moved Service annotation; brief gap during cutover.

You can dry-run the migration with `sc deploy -P -s <stack> -e <env>` to inspect the diff before applying.

### After the last sibling migrates

If you eventually destroy every stack that ever referenced the old shared namespace, the namespace itself lingers (because of `RetainOnDelete`). Clean it up by hand once you've verified nothing depends on it:

```sh
kubectl get all -n <old-shared-namespace>      # should be empty
kubectl delete namespace <old-shared-namespace>
```

### PersistentVolumeClaim caveat

If your custom stack uses `persistentVolumes`, the namespace move triggers a Pulumi Replace on each PVC. Because PVCs are namespace-scoped and not movable, Pulumi creates the new PVC and deletes the old one. **If the StorageClass's `reclaimPolicy` is `Delete` (the default for most dynamic provisioners on AWS/GCP), the underlying PV and its data are destroyed along with the old PVC.**

Before migrating a stateful custom stack:

1. Patch the existing PV's reclaim policy to `Retain`:
   ```sh
   kubectl patch pv <pv-name> -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
   ```
2. Run `sc deploy -P` to confirm the diff matches expectations.
3. After the migration, clear `claimRef` on the retained PV and re-bind it to the new PVC if you want to preserve the data — or accept the data is dev-only and let it recreate.

Stacks that don't define `persistentVolumes` (the typical case where state lives in managed services like Cloud SQL, RDS, Memorystore, or external MongoDB Atlas) are unaffected.

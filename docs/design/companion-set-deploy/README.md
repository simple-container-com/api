# RFC: Companion-set deploy

**Status:** Draft
**Area:** `cloud-compose` deploy orchestration (GCP/Kubernetes; applies to any provider with cross-stack `dependencies`)

## Summary

Teach `sc deploy` to deploy a stack **together with its required companion stacks** as one
ordered, fail-closed transaction. The companion set is **inferred from the reverse
`dependencies.owner` edges Simple Container already loads** — so no new configuration is
needed for existing splits.

## Problem

Simple Container deploys are **imperative and per-stack**. A common pattern splits one service
into companion stacks that share a single datastore/queue via a cross-stack `dependencies`
reference — for example a web/API stack plus a **singleton** background-worker or scheduler
stack that consumes the same Postgres-backed queue:

```yaml
# owner stack (web) — owns the schema/migrations, provisions the DB
config:
  runs: [ web ]
  uses: [ postgres ]

# companion stack (worker) — shares the owner's queue, does NOT provision its own DB
config:
  runs: [ worker ]           # singleton: scale.min == scale.max == 1
  dependencies:
    - name: main-db
      owner: my-project/web   # <-- reverse edge: this stack is a companion of my-project/web
      resource: postgres
```

Because each stack is deployed independently, an operator (or a CI job) can deploy the **owner**
without its **companion**. When the split is introduced by migrating away from a single
combined pod, deploying the owner *removes* the co-located worker; if the companion deploy is
then forgotten or fails, the environment is left with **no queue consumer / no periodic
processor** — and this fails **silently**: the owner's HTTP health check stays green, and there
is no worker `Deployment` object for a "zero available replicas" alert to fire on. The gap can
persist unnoticed until a downstream symptom surfaces.

Pipeline-level guards (e.g. a CI workflow that chains the two deploys with `needs:`) mitigate
this for the two-tier case, but they only protect the CI path — a direct `sc deploy <owner>`
still strands the companion — and they become awkward for services with more than one companion
tier (one owner + several dependent workers form a small DAG, not a flat chain).

## Proposal

Make the invariant hold at the **platform** layer. `sc deploy <stack>` resolves the stack's
**required-companion closure** and deploys the whole set in dependency order as a single
transaction; a partial application is a non-zero exit, never "success."

### Resolution

1. `ReadStacks` already loads every stack under `.sc/stacks/*` and their `dependencies` edges.
   Build the **reverse edge map** `owner -> [dependents]`: a stack declaring
   `dependencies: [{ owner: X, resource: ... }]` is a companion of `X`.
2. For `sc deploy <stack> --env <env>`, compute the required-companion closure — the target
   plus every stack that (transitively) references it — restricted to stacks that declare
   `<env>`.
3. **Topologically order** owner-before-dependent. This matches the safe ordering for singleton
   companions: bringing a singleton worker/scheduler up *before* the owner drops the previous
   co-located instance would double-run its periodic work, so the owner must go first.
4. Deploy each stack in order via the existing single-stack path. **Fail closed:** any stack
   failing aborts the set with a non-zero exit; a partial set is never reported as success.

`ReconcileForDeploy` already iterates all stacks for an environment, so the iteration/ordering
surface exists. The core change is going from a single-stack `Deploy()` to an ordered
`DeploySet()` (a thin wrapper over the existing `Deploy()`).

### Surface / opt-in

- Behind a flag initially: `sc deploy <owner> --with-companions` (or `sc deploy-group <owner>`);
  later default-on for owners that have companions.
- Default resolution is **pure inference** from `dependencies.owner`, which covers every existing
  split with no YAML change. An optional explicit override on the owner
  (`deploy.companions: [ <stack>... ]`) is available for sets that inference cannot express.
- Symmetric guard: `sc deploy <companion>` alone may warn/refuse unless its owner is already
  deployed for the environment, preventing companion-without-owner as well.

## Non-goals

This guards the deploy **action**; it is **not** a continuous reconciler. It does not detect a
companion torn down out-of-band, or a run that failed and was abandoned. That residual tail is
the domain of external drift detection / alerting and is out of scope here.

## Rollout

1. Implement behind `--with-companions`; unit-test reverse-edge resolution, topological order,
   and fail-closed partial application.
2. Validate on a non-production environment: deploy an owner with a companion as one set; kill
   the companion mid-set and confirm the whole invocation exits non-zero.
3. Enable by default for owners that have companions; downstream CI can then drop bespoke
   two-deploy chaining.

## Alternatives considered

- **CI-only chaining** (`worker` job `needs: web` in one workflow): effective for two tiers but
  only guards the CI path and does not model multi-companion DAGs; suitable as an interim
  measure, not the durable fix.
- **Cluster admission policy** ("deny an owner whose companion is absent"): admission is
  edge-triggered on submitted objects; a never-submitted companion produces no event to
  intercept, so it cannot enforce this invariant.

# RFC: Keyless secret backend (KMS-wrapped recipients + OIDC federation)

**Status:** Draft / request for comments
**Area:** `pkg/api/secrets` (encryption envelope, recipients, CLI `secrets` commands)
**Relates to:** [`docs/SECRETS-POLICY.md`](../../SECRETS-POLICY.md), [`docs/SECURITY.md`](../../SECURITY.md)

## Summary

Today, decrypting an SC secret store in CI requires materializing the store's
**private key** (`SIMPLE_CONTAINER_CONFIG`) in plaintext on the runner — typically
as a CI secret. That value is long-lived, all-powerful (decrypts the entire store),
not rotated in practice, not audited, and not revocable after a leak.

This RFC proposes adding **pluggable recipient types whose private key never leaves a
remote authority** (a cloud KMS or Vault), and a **keyless CI mode** where the runner
proves its identity via OIDC federation and obtains a short-lived `Decrypt` permission
instead of holding any long-lived key. It is **cloud-agnostic** (AWS/GCP/Azure/Vault,
or none) and **backward-compatible** (the existing local-key recipient stays the
default; migration is additive, per-repo, and reversible).

The crypto primitives are not reinvented; the change is the **key-custody model** plus
a **v2 envelope format** required to support per-recipient key wrapping cleanly.

## Motivation

The SC envelope is already **multi-recipient** (`AddPublicKey`/`RemovePublicKey`,
CLI `secrets allow`/`disallow`). The limitation is that there is exactly **one
recipient type**: a raw asymmetric private key. Because decryption requires possessing
that private key, the key must be placed on every machine that decrypts — including
CI. There is no way to delegate the unwrap to an external authority.

Consequences of the single long-lived key in CI:

- **Exfiltration surface.** Any code that runs in the job (including a malicious
  transitive dependency on an untrusted PR build) can read the key and decrypt the
  whole store.
- **No rotation in practice.** Rotating means re-keying and redistributing the key to
  every consumer.
- **No audit.** Use of a local private key is not logged anywhere.
- **No revocation.** A leaked private key decrypts forever.

## Goals / non-goals

**Goals**
- Allow a store to be decrypted in CI with **no long-lived secret present** on the runner.
- Keep it **cloud-agnostic** and keep the **simple local-key path as the default**.
- Make access **scoped, audited, and revocable**.
- Provide an **additive, reversible, per-repo** migration.

**Non-goals**
- Reinventing cryptographic primitives.
- Replacing the on-disk format with SOPS/age (we keep SC's format + `allow`/`disallow` UX).
- Putting an external SaaS vault in the deploy hot path by default.
- Moving application runtime secrets out of the store (separate concern).

## Design

### 1. v2 envelope (prerequisite)

The current format encrypts each file independently for each recipient. To wrap keys
per recipient cleanly — and to keep KMS calls O(1) per deploy rather than O(files) —
introduce a **versioned v2 envelope**:

- A single random **data-encryption key (DEK)** encrypts the payload once with an
  AEAD (ChaCha20-Poly1305), per file-set (or per environment).
- The DEK is **wrapped once per recipient**.
- The AEAD includes **associated data (AAD)** binding the ciphertext to
  `{format-version, path, environment, recipient-id}` so a wrapped DEK or ciphertext
  cannot be transplanted into another context.
- Recipient wrapping standardizes on a **vetted public-key scheme** (an X25519-based
  KEM for asymmetric recipients) and **KMS Encrypt/Decrypt** for KMS recipients. Key
  material and algorithm identifiers are carried in **authenticated** headers.

`secrets.yaml` gains an explicit `version` and a typed `recipients[]` list.

### 2. Typed recipients + `KeyProvider`

```go
type KeyProvider interface {
    Wrap(ctx context.Context, dataKey []byte) (wrapped []byte, err error)
    Unwrap(ctx context.Context, wrapped []byte) (dataKey []byte, err error)
    Recipients() []RecipientRef // metadata only, no network, no decrypt
}
```

Recipient types:

| Type | Private key location | CI auth |
|---|---|---|
| `local` (default) | a local private key (today's `SIMPLE_CONTAINER_CONFIG`) | the key itself |
| `aws-kms://<key-arn>` | AWS KMS | OIDC → STS → `kms:Decrypt` |
| `gcp-kms://<resource>` | GCP KMS | OIDC → Workload Identity Federation → `decrypt` |
| `azure-kv://<key-id>` | Azure Key Vault | OIDC → federated credential |
| `vault://transit/<key>` | HashiCorp Vault | OIDC/JWT auth → transit decrypt |

`DecryptAll` selects the recipient by **authenticated recipient-id** (not "first that
succeeds") and unwraps via the matching provider. Any one recipient suffices; never all.

KMS recipients reference **immutable key identifiers** (not mutable aliases) and bind a
KMS **encryption context** matching the AAD above.

### 3. Keyless CI via OIDC

When a KMS recipient is configured and the platform exposes an OIDC token, SC exchanges
the token for short-lived credentials scoped to `Decrypt` on the recipient key. On
GitHub Actions the runner only needs `id-token: write`; no long-lived secret is stored.
Every unwrap is recorded in the cloud audit log under the federated identity.

To avoid handing the OIDC mint capability to arbitrary job steps, credential
acquisition should happen in **one isolated step** that passes only the resulting
short-lived credentials onward.

### 4. Federated deploy credentials

Independently of store decryption, the cloud credentials used by `sc deploy` can be
sourced from the same OIDC federation (`auth: { provider: oidc }`) instead of static
keys stored inside the secret store. This requires **first-class auth provider types**
that consume ambient federated credentials rather than deserializing static key
material. Static keys remain supported as a fallback.

### 5. Environment-scoped recipients

A recipient may be granted to a single environment. Combined with per-environment DEKs,
an untrusted/preview context can be limited to decrypting only its own environment and
never production. This is enforced by encrypting each environment under a distinct DEK
and never wrapping production's DEK to a preview-reachable recipient.

### 6. Decouple repository checkout from the encryption key

Where the store private key is currently reused as an SSH key to clone a private
parent/stacks repository, keyless mode needs a separate mechanism (e.g. a narrowly
scoped GitHub App installation token, or an existing deploy key). This is **orthogonal**
to encryption and is a **prerequisite** before the local key can be removed.

## Authorization model (OIDC trust)

The trust policy — not the runner — is the control. Hard-won requirements:

- **Per-stack / per-environment roles**, not one shared role. A single role shared by
  many consumers, trusted by `job_workflow_ref` alone, lets *any* repo that calls the
  shared workflow assume it.
- **Pin the concrete caller**: immutable repository id **and** `job_workflow_ref` (to a
  pinned ref), plus `aud`. **No wildcards** in the subject.
- **`ref` and `environment` subjects are mutually exclusive.** A job using a GitHub
  Environment emits `repo:ORG/REPO:environment:NAME` with **no** `ref` component; a job
  without one emits `...:ref:...`. You cannot pin both in one subject — choose per role.
- **Production = protected Environment with required reviewers.** OIDC trust alone is
  not sufficient to gate production.
- **Never `pull_request_target` with PR-head checkout** in any workflow that can mint an
  id-token or reach a recipient.
- **Attribution:** set a session name carrying repo + run id; enable cloud data-access
  logging where it is off by default (e.g. GCP KMS), so a shared role does not erase
  per-run attribution.

## Backward compatibility & format versioning

- The `local` recipient + existing config remain the **default**; nothing changes for
  current users until they opt in.
- **Forward-compat guard (ship first):** older clients ignore unknown YAML fields and
  rewrite only the fields they know, which would **silently drop** new recipients on the
  next `allow`/`disallow`. A version-aware client that **fails closed** on an unknown
  `version` must be released and rolled out **before** any store is written in v2.
- Migration uses existing verbs: `secrets allow --kms <uri>` (add a KMS recipient),
  verify keyless decrypt, then `secrets disallow <local-key>`. Multi-recipient means
  both work during overlap; per-repo; reversible.
- A `require-kms` (or equivalent) per-store setting disables the legacy path once a store
  is fully migrated, to prevent silent downgrade back to the local key.
- Cloud-agnostic: customers on no cloud keep the `local` recipient permanently — it is
  not a deprecation target.

## Revocation reality

Removing a recipient or rotating a KMS key does **not** revoke access to ciphertext
already committed to version control — history retains blobs decryptable by previously
valid recipients. Therefore:

- **Removing a recipient MUST be paired with rotating the underlying secret values**
  (and the upstream credential they represent), not just dropping a wrapper.
- The recipient list is effectively an **append-only audit surface**; changes to it
  should be review-gated (code owners).

## Operational considerations

- **Availability:** decryption now depends on cloud IAM/KMS. Decrypt **all** required
  material **up front**, before any infrastructure mutation, so a mid-deploy KMS error
  cannot leave a half-applied stack.
- **Throughput/cost:** one DEK per file-set ⇒ one `Decrypt` per deploy, not one per
  file. Use bounded retries with backoff + jitter; cache the unwrapped DEK in-process
  for the run only.
- **Locality:** pin the KMS region to the deploy region; document cross-account key
  policies.
- **Dependencies:** the cloud KMS SDKs become direct dependencies (today they are
  transitive), expanding the SCA/dependency-update surface.

## Threat model (summary)

**Mitigates:** leak of a long-lived master key from CI; secret theft by untrusted
PR-build code (via scoped trust + environment-scoped recipients); lateral movement
after a single job compromise (scope + short TTL); long-lived static cloud credentials;
absence of audit; departed-operator access on the CI path; replay of a stolen token in
another repo (pinned `aud`/subject).

**Does not fully mitigate (residual):** malicious code inside a *legitimate*
production job (gets short-lived scoped credentials during the run — bounded by scope,
TTL, branch protection, audit, and, for the highest-value targets, isolated runners);
application secrets being decrypted into the workload at deploy time; exfiltration over
otherwise-allowed network egress; compromise of the cloud account / IAM / KMS policy
itself; supply-chain compromise of the CLI or CI actions (mitigated by signed releases
+ digest pinning); phishing of operator cloud credentials.

## Migration phases

0. **Design + review** (this document).
1. **v2 envelope + version-aware fail-closed client**, released and rolled out before
   any v2 write. Modernize the asymmetric recipient scheme as part of v2.
2. **`KeyProvider` + first KMS provider + OIDC acquisition**, behind a feature flag.
3. **Canary** on one low-risk staging stack. Gate: `sc deploy` obtains cloud
   credentials from the federated environment, not the store; measured KMS latency;
   verified rollback runbook.
4. **Migration UX** (`allow --kms`, `secrets doctor`/recipient listing, `require-kms`),
   environment-scoped recipients, repo-checkout decoupling, docs.
5. **Federated deploy credentials** (`auth: { provider: oidc }`); drop static keys from
   the store.
6. **Staging bake**, then **production** behind protected Environments, stack by stack.
7. **Decommission**: remove the local recipient, **rotate secret values**, delete static
   credentials after a zero-usage soak. Add other cloud providers (GCP/Azure/Vault).

## Open questions

- Per-file-set vs per-environment DEK granularity (audit/scoping vs rewrap cost).
- How aggressively to enforce a minimum recipient-strength / forbidden-type policy.
- Whether `require-kms` is per-store, per-environment, or both.
- Provider parity: each KMS/Vault has distinct federation, audit, and key-versioning
  semantics; ship one provider first behind an explicit contract, then add others with
  per-provider review.

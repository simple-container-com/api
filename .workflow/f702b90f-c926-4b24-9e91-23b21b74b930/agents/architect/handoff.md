## 1. Commit + branch identity

No code mutations in this turn.

Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/f702b90f-c926-4b24-9e91-23b21b74b930`

Observed prior shipped baseline: commit `0aaefb04` is real and was verified via `read_commit_diff`; it added `pkg/clouds/pulumi/yandex/credentials.go`, `pkg/clouds/pulumi/yandex/credentials_test.go`, and modified `pkg/clouds/pulumi/yandex/provider.go` / `registrar.go` for Yandex secret marking. Do not reuse that SHA as this run’s commit.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---:|---|

## 3. Developer front-load: required implementation plan

### Architectural decision

**Structural concern:** provider credentials must be marked secret before they enter the Pulumi engine. Masking at log-summary time is only a fallback if smoke proves a residual leak; it is not the primary fix.

Apply `sdk.ToSecret(...)` at each credential-bearing provider argument boundary, and regression-test against Pulumi mock monitor inputs with explicit resource/property existence assertions so tests cannot pass vacuously.

### A. AWS provider credentials

Observed source evidence:

| Path | Finding | Required change |
|---|---|---|
| `pkg/clouds/pulumi/aws/provider.go` | `applyAWSProviderCreds` currently assigns `args.AccessKey = sdk.String(accessKey)` and `args.SecretKey = sdk.String(secretAccessKey)` for static credentials; ambient mode leaves credentials unset and sets `SkipCredentialsValidation`. | Keep ambient behavior unchanged. For static mode, mark `SecretKey` with `sdk.ToSecret`; keep `AccessKey` unmarked as an identifier unless a provider schema/test proves it must also be secret. |
| `pkg/clouds/pulumi/aws/cloudtrail_security_alerts.go` | `applyAWSProviderCreds(providerArgs, cfg.AccessKey, cfg.SecretAccessKey)` is used for the regional CloudTrail provider. | No separate helper fork. Fixing `applyAWSProviderCreds` must cover both the ordinary AWS provider and the regional provider. |
| `pkg/clouds/pulumi/aws/provider_test.go` | Existing tests inspect Go arg values directly; they do not prove what reaches Pulumi mock monitor as secret. | Add table-driven `pulumi.WithMocks` tests that register both ordinary and regional AWS providers, locate `pulumi:providers:aws`, assert the resource exists, assert the `secretKey` property exists, and assert `Input.IsSecret() == true`. Also assert ambient path does not create access/secret-key properties. |

Developer note: do not print the synthetic secret string or any resource input map in failures. Assert property names and boolean secretness only.

### B. GCP provider credentials

Observed source evidence:

| Path | Finding | Required change |
|---|---|---|
| `pkg/clouds/pulumi/gcp/provider.go` | `Provider` assigns explicit non-ambient credentials via `args.Credentials = sdk.String(creds)`. Ambient credentials are intentionally omitted. | Change explicit credentials to `sdk.ToSecret(sdk.String(creds)).(sdk.StringOutput)` or the equivalent typed input accepted by `gcp.ProviderArgs.Credentials`. Preserve ambient omission. |
| `pkg/clouds/pulumi/gcp/provider.go` | State-store functions set environment variables and run `gcloud`; those are outside Pulumi provider args. | Do not broaden this slice into env-var refactoring unless smoke proves a log leak. |

Required test:

- Table-driven `pulumi.WithMocks` test:
  - explicit credentials case: provider resource exists, `credentials` property exists, `IsSecret() == true`;
  - ambient credentials case: provider resource exists, `credentials` property absent.
- Use a synthetic JSON-ish credential fixture only; never echo it in test messages.

### C. MongoDB Atlas provider credentials

Observed source evidence:

| Path | Finding | Required change |
|---|---|---|
| `pkg/clouds/pulumi/mongodb/provider.go` | `PrivateKey: sdk.StringPtr(authCfg.PrivateKey)` is a credential-bearing provider input; `PublicKey` and `Region` are not secrets. | Secret-mark `PrivateKey`, preserving the pointer-input shape. Public key and region should remain non-secret. |

Required test:

- `pulumi.WithMocks` provider test locating `pulumi:providers:mongodbatlas`;
- assert provider exists;
- assert `privateKey` property exists and `IsSecret() == true`;
- assert `publicKey` remains non-secret.

### D. Cloudflare registrar provider token

Observed source evidence:

| Path | Finding | Required change |
|---|---|---|
| `pkg/clouds/pulumi/cloudflare/registrar.go` | `Registrar` constructs its own Cloudflare provider with `ApiToken: sdk.StringPtr(cfg.AuthConfig.Credentials.Credentials)`. It does not go through a normal cloud provider constructor. | Mark `ApiToken` with `sdk.ToSecret(...)` at this constructor. Preserve the provider name behavior based on `cfg.AccountId`. |

Required test:

- Short mock test for registrar construction.
- Locate the Cloudflare provider resource (`pulumi:providers:cloudflare`) created during registrar init.
- The resource naming path may include the provider preview suffix; management explicitly asked to confirm under `{providerName}.__preview`, so assert by provider type + `apiToken` property rather than by brittle full URN alone, and include a name assertion if the mock exposes it.
- Assert:
  - resource exists;
  - `apiToken` property exists;
  - `apiToken.IsSecret() == true`.

### E. Kubernetes kubeconfig provider constructors

Observed source evidence from repository search:

| Path | Finding | Required change |
|---|---|---|
| `pkg/clouds/pulumi/kubernetes/provider.go` | Direct Kubernetes provider uses `Kubeconfig: sdk.String(creds)`. | Use `sdk.ToSecret(sdk.String(creds))` before assigning. Preserve `EnableServerSideApply`. |
| `pkg/clouds/pulumi/gcp/compute_proc.go` | Parent-stack kubeconfig is converted with `sdk.String(kubeConfig)` for a computed Kubernetes provider. | Use `sdk.ToSecret(sdk.String(kubeConfig))`. |
| `pkg/clouds/pulumi/gcp/gke_autopilot_stack.go` | GKE Autopilot provider uses `Kubeconfig: sdk.String(kubeConfig)`. | Use `sdk.ToSecret(sdk.String(kubeConfig))`; if later constructing `kubeConfigOutput` for deployment patch, inspect whether that path reaches logs/state and mark if needed. |
| `pkg/clouds/pulumi/kubernetes/caddy.go` | Caddy-related Kubernetes provider uses `Kubeconfig: kubeconfig` where kubeconfig is an output/input. | Wrap the input with `sdk.ToSecret(kubeconfig)` while preserving its input/output type. |
| `pkg/clouds/pulumi/gcp/adopt_gke_autopilot.go` | Adopted GKE provider uses generated kubeconfig directly. | Wrap generated kubeconfig with `sdk.ToSecret(...)` before assigning. |

Required test minimum:

- Add at least one unit test under `pulumi.WithMocks` that builds a Kubernetes provider with synthetic kubeconfig and asserts:
  - `pulumi:providers:kubernetes` resource exists;
  - `kubeconfig` property exists;
  - `kubeconfig.IsSecret() == true`.
- Prefer a small shared helper for mock resource lookup so future constructor tests cannot pass if the provider/property is absent.
- If time allows, add table rows for direct provider + GKE constructor + Caddy constructor, but keep the slice minimal.

### F. Shared test helper pattern

Use the Yandex regression style from `pkg/clouds/pulumi/yandex/credentials_test.go` as the pattern, but make the existence checks mandatory:

```go
inputs := mocks.inputsWithProp(providerToken, propertyName)
Expect(inputs).NotTo(BeNil(), "provider with property was not created")
prop, ok := inputs[resource.PropertyKey(propertyName)]
Expect(ok).To(BeTrue(), "provider property was not recorded")
Expect(prop.IsSecret()).To(BeTrue())
```

Why: the Yandex tests explicitly guard against the “first provider wins” trap where a keyless provider appears before the credential-bearing provider.

### G. Documentation pre-check

Management specifically asked for a pre-check in `docs/TESTING.md`.

Required doc edit:

- Add a short subsection near “Test invocation” or “Security-sensitive paths” stating:
  - provider credentials and kubeconfigs must be marked with `sdk.ToSecret` before entering Pulumi;
  - regression tests should use `pulumi.WithMocks` and assert `Input.IsSecret()`;
  - deploy masking validation must use the prescribed CI/deploy path, not local `sc deploy`;
  - smoke reports may include run identity, outcome, aggregate secret-marker count, and yes/no leak detection only — no raw credential-bearing log excerpts.

### H. Preview/update summary and event-output risk

Observed source evidence:

| Path | Finding | Decision |
|---|---|---|
| `pkg/clouds/pulumi/preview.go` | `toPreviewResult` and `toUpdateResult` copy `result.StdOut` into API summaries. | This can forward provider-property values if they reach Pulumi unmarked. Primary fix remains secret-marking inputs. |
| `pkg/clouds/pulumi/deploy.go` | Deploy logs preview/update summaries with `p.toPreviewResult(...)` and `p.toUpdateResult(...)`. | Smoke must validate masked output. |
| `pkg/clouds/pulumi/events.go` | Event handlers log diagnostic messages and diff summaries. `diffSummary` emits property names and operation kinds, not values. Diagnostics could still contain provider/plugin messages. | Do not add broad redaction up front. If fresh CI preview/update proves a credential value still appears despite marked inputs, add a minimal redaction boundary at the demonstrated output path and test that boundary. |

### I. Prescribed smoke route

Do **not** run local `sc deploy`.

Developer/QA should use one of:

1. an isolated stack workflow, or
2. an independent service workflow that invokes `.github/actions/deploy-client-stack/action.yml`.

Report only:

| Field | Allowed content |
|---|---|
| Run identity | workflow name / run id / URL if available |
| Outcome | success/failure/cancelled |
| Secret marker count | aggregate count only |
| Leak detection | yes/no |
| Raw log lines | **Do not include if they may contain credentials** |

If the smoke requires operator-only credentials or workflow dispatch permissions, stop after code + tests + docs are committed and mark the deploy-smoke AC blocked with the exact missing grant. Do not hand-wave it as passed.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Audit `api/pkg/clouds/pulumi/*/provider.go` and associated provider constructors/registrars for credentials passed to Pulumi as plain property values. | PASS | Read/queried `pkg/clouds/pulumi/aws/provider.go`, `pkg/clouds/pulumi/gcp/provider.go`, `pkg/clouds/pulumi/mongodb/provider.go`, `pkg/clouds/pulumi/kubernetes/provider.go`, `pkg/clouds/pulumi/cloudflare/registrar.go`, and searched `Kubeconfig:` under `pkg/clouds/pulumi`; findings are itemized above. |
| AC2 | Fix every confirmed analogous path with `sdk.ToSecret` at the boundary and regression tests against mock monitor output. | FAIL | No code mutations in this architecture rerun. Required implementation and test matrix are specified above for AWS, GCP, MongoDB, Cloudflare, and Kubernetes. |
| AC3 | Reassess whether forwarding Pulumi preview/update summaries verbatim to stdout risks disclosing credential values; if needed, make minimal redaction without hiding useful diagnostics. | PASS for assessment / FAIL for validation | Assessed `pkg/clouds/pulumi/preview.go`, `pkg/clouds/pulumi/deploy.go`, and `pkg/clouds/pulumi/events.go`: summaries forward `StdOut`, while event diff summaries log property names/kinds. Redaction should be conditional on post-fix smoke evidence. |
| AC4 | Fresh preview/update verifies credentials remain masked; count secret markers only and never print key-bearing log lines. | FAIL | No CI/deploy smoke initiated in this architecture turn. Required CI smoke route and allowed reporting fields are specified above. |
| AC5 | Build/test and smoke at least one dependent Forge service or isolated stack through prescribed CI/deploy path, not local `sc deploy`. | FAIL | No build/test/smoke run in this turn. Developer/QA must run scoped Go tests plus the prescribed isolated/dependent stack workflow after implementation. |
| AC6 | Add `docs/TESTING.md` pre-check stating the masking CI requirement. | FAIL | No documentation mutation in this turn. Exact required doc content is specified above. |

## 5. Tests run

None. This was a read-only architecture rerun to enrich the developer handoff. The repository was inspected through authenticated repository tools; no source files were written and no test command was executed.

## 6. Developer checklist

1. Implement `sdk.ToSecret` at these boundaries:
   - `pkg/clouds/pulumi/aws/provider.go` — `applyAWSProviderCreds` static `SecretKey`;
   - `pkg/clouds/pulumi/gcp/provider.go` — explicit `Credentials`;
   - `pkg/clouds/pulumi/mongodb/provider.go` — `PrivateKey`;
   - `pkg/clouds/pulumi/cloudflare/registrar.go` — `ApiToken`;
   - Kubernetes kubeconfigs in:
     - `pkg/clouds/pulumi/kubernetes/provider.go`;
     - `pkg/clouds/pulumi/gcp/compute_proc.go`;
     - `pkg/clouds/pulumi/gcp/gke_autopilot_stack.go`;
     - `pkg/clouds/pulumi/kubernetes/caddy.go`;
     - `pkg/clouds/pulumi/gcp/adopt_gke_autopilot.go`.
2. Add table-driven `pulumi.WithMocks` tests with resource/property existence checks and `IsSecret()` assertions.
3. Add the `docs/TESTING.md` masking pre-check.
4. Run focused package tests first; then `welder run test` if runtime budget allows.
5. Run the prescribed CI/deploy smoke through an isolated stack or dependent service workflow using `.github/actions/deploy-client-stack/action.yml`; report only aggregate safe evidence.
6. Only if smoke shows a remaining leak despite marked inputs, add the minimal redaction boundary in `preview.go` / `deploy.go` / `events.go` proven by that smoke.

**Verdict:** signoff
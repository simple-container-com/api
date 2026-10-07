## 1. Commit + branch identity

No code mutations in this turn. Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/f702b90f-c926-4b24-9e91-23b21b74b930`.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|

## 3. Architecture decision and file-grounded audit

**Structural concern:** marking a credential secret *after* constructing a Pulumi provider is too late. The fix belongs on each credential-bearing provider argument, including providers constructed outside the registered `Provider` functions.

| Boundary inspected | Finding | Developer action |
|---|---|---|
| `pkg/clouds/pulumi/aws/provider.go`; `pkg/clouds/pulumi/aws/cloudtrail_security_alerts.go` | `applyAWSProviderCreds` assigns static `AccessKey` and `SecretKey` as plain `sdk.String`; the regional CloudTrail provider calls the same helper. Ambient mode deliberately leaves these arguments unset. Existing `pkg/clouds/pulumi/aws/provider_test.go` checks values, not mock-monitor secrecy. | Secret-mark the **static secret key** in the shared helper and test both ordinary and regional provider registration. Treat access-key ID as an identifier, consistently with the existing Yandex tests; retain ambient behavior. |
| `pkg/clouds/pulumi/gcp/provider.go` | Explicit `CredentialsValue()` is assigned to `gcp.ProviderArgs.Credentials` as plain `sdk.String`; ambient mode omits it. | Secret-mark explicit credentials only; test explicit and ambient paths. |
| `pkg/clouds/pulumi/mongodb/provider.go` | `AtlasConfig.PrivateKey` reaches `mongodbatlas.ProviderArgs.PrivateKey` as plain `sdk.StringPtr`; `PublicKey` and `Region` are separate fields. | Secret-mark `PrivateKey`, preserving its pointer-input type; test registered provider inputs. |
| `pkg/clouds/pulumi/cloudflare/registrar.go` | Registrar constructs its own provider with `ApiToken: sdk.StringPtr(...)`; it does not go through a registered cloud `Provider` function. | Secret-mark `ApiToken` at this constructor and test its monitor inputs. |
| `pkg/clouds/pulumi/kubernetes/provider.go`; `pkg/clouds/pulumi/gcp/compute_proc.go`; `pkg/clouds/pulumi/gcp/gke_autopilot_stack.go`; `pkg/clouds/pulumi/kubernetes/caddy.go`; `pkg/clouds/pulumi/gcp/adopt_gke_autopilot.go` | Multiple Kubernetes provider constructors pass kubeconfig through plain strings or outputs. Kubeconfig is credential-bearing when it embeds authentication material; its content cannot safely be inferred from the field name. | Mark kubeconfig secret at **each** constructor boundary, retaining the existing input/output type and not changing resource names. Test representative direct, parent-stack and output-derived paths; enumerate all constructors in the implementation audit. |
| `pkg/clouds/pulumi/yandex/provider.go`; `pkg/clouds/pulumi/yandex/registrar.go` | Existing `secretStringPtr` protects service-account document and static secret keys; `pkg/clouds/pulumi/yandex/credentials_test.go` asserts their mock-monitor secret properties. | **No repeat S0–S2 fix.** Keep these tests as the regression pattern. |

**Output decision:** `pkg/clouds/pulumi/preview.go` copies `PreviewResult.StdOut` and `UpResult.StdOut` into summaries; `pkg/clouds/pulumi/deploy.go` logs those summaries. `pkg/clouds/pulumi/events.go` separately logs diagnostic messages but its resource pre/output summaries show property names and operation kinds, not property values. Thus unmarked provider inputs can reach verbatim stdout summaries. **Do not strip all stdout or suppress useful diffs:** first fix and test the source markings, then verify fresh preview *and* update masking. If that smoke demonstrates a remaining credential-bearing summary or diagnostic despite marked inputs, make a narrow, tested redaction at the demonstrated output boundary; do not claim generic string replacement can identify unknown secrets.

**Implementation/test shape:** use `sdk.ToSecret` at each boundary, converting the resulting typed output where a `StringPtrInput` is required, as illustrated by `pkg/clouds/pulumi/yandex/credentials.go`. Run provider constructors under `pulumi.WithMocks`; inspect `MockResourceArgs.Inputs` for the intended property and assert `IsSecret()`. Assert the resource/property exists first so a missing provider cannot pass vacuously. Use synthetic fixtures only; never print a property map, credential, or key-bearing CI line. Preserve non-secret region, project, public-key and access-key-ID inputs where appropriate. Record the completed audit and design in a **real repository documentation path**, not solely in `.workflow/`.

**Verification route:** `docs/TESTING.md` identifies `welder run test` as canonical CI testing. The repository’s `.github/actions/deploy-client-stack/action.yml` exposes the client-stack deploy action and its configuration input. Developer/QA should identify an authorized dependent-service workflow or isolated stack that consumes the changed build, run its prescribed CI preview/update (not local `sc deploy`), and report only run identity, outcome, aggregate secret-marker counts and absence/presence checks—not raw key-bearing log lines. A unit-test-only result is insufficient.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Audit provider constructors/registrars with file-grounded findings. | PASS | Inspected the source paths and identified boundaries in the table above, including the Cloudflare registrar and additional Kubernetes constructors. |
| AC2 | Secret-mark confirmed analogous inputs and test mock-monitor output. | FAIL | Architecture turn made no code changes or new tests; implementation specified above. |
| AC3 | Assess verbatim preview/update forwarding and make minimal redaction if needed. | FAIL | Forwarding paths assessed in `preview.go`, `deploy.go` and `events.go`; whether marked values remain exposed requires the fresh smoke before deciding on a redaction edit. |
| AC4 | Fresh CI preview/update demonstrates masking without reproducing credentials. | FAIL | No CI deployment was initiated in this architecture turn. |
| AC5 | Build/test and smoke a dependent service or isolated stack via prescribed path; no local deploy. | FAIL | No build or dependent-stack smoke was run in this turn. |

## 5. Tests run

None; this was a read-only architecture turn. The earlier PM handoffs’ planned work is **not** test or implementation evidence.

**Verdict:** signoff
### 1. Commit + branch identity
1. No code mutations in this turn.
2. Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/f702b90f-c926-4b24-9e91-23b21b74b930`
3. Handoff metadata: `{"slice_id":"yc-provider-key-in-deploy-log"}`

### 2. Files touched
| Path | Lines added / removed | What changed |
|---|---|---|

### 3. Scope and decisions
1. **MUST — S3 only.** Audit `pkg/clouds/pulumi/*/provider.go` and associated constructors and registrars. Document each inspected credential path with file-grounded evidence; distinguish confirmed unmarked inputs from paths already protected.
2. **MUST — Smallest fix.** Apply `sdk.ToSecret` at the Pulumi boundary for every *confirmed* analogous credential input. Test secret markers in mock-monitor output without recording credential values.
3. **MUST — Summary review.** Determine whether verbatim preview/update summaries can expose credentials. Redact only if the risk is confirmed, preserving useful non-secret diagnostics.
4. **MUST — End-to-end evidence.** Use the prescribed CI/deploy path for a fresh preview/update masking smoke, plus build/test and a smoke of at least one dependent Forge service **or** an isolated stack. Count secret markers; do not print log lines containing keys. Do not run a local `sc deploy`.
5. **CUT — Already shipped S0–S2.** The brief identifies api PR #421 (`0aaefb04`, SC v2026.9.16) as covering Yandex `serviceAccountKeyFile`, `StorageSecretKey`, and `YmqSecretKey` in both constructors. Do not redo those fixes or repeat credential rotation. Never quote a credential value in code, tests, logs, or handoffs.
6. **CUT — Unrelated provider refactors.** Limit changes to confirmed leakage paths and the evidence needed to verify them.

### 4. Acceptance criteria verification
| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Document the provider-path audit with file-grounded findings. | OPEN/PENDING | No audit performed this PM turn; scoped for Architect and Developer. |
| AC2 | Secret-mark every confirmed analogous credential input at the boundary and test mock-monitor output. | OPEN/PENDING | No fix or regression test performed this PM turn; delegated downstream. |
| AC3 | Reassess verbatim preview/update summary forwarding; make minimal redaction only if needed. | OPEN/PENDING | No summary-path assessment performed this PM turn; delegated downstream. |
| AC4 | Verify masking in a fresh CI/deploy preview/update, counting markers without printing key-bearing log lines. | OPEN/PENDING | No deploy or smoke performed this PM turn; delegated downstream. |
| AC5 | Build/test and smoke a dependent Forge service or isolated stack through the prescribed path, with no local `sc deploy`. | OPEN/PENDING | No build, test, or smoke performed this PM turn; delegated downstream. |

### 5. Tests run
1. None; this is a planning-only turn.

### 6. Architect handoff
1. Map the credential-to-Pulumi boundaries and summary-output path before choosing edits. Give Developer a file-by-file list of confirmed gaps, a minimal test strategy, and the approved CI/deploy smoke path.
2. Keep evidence safe: assert marker presence or counts, not secret text; report aggregate smoke results, not potentially sensitive log excerpts. If the prescribed path needs an operator-only grant, identify it precisely before claiming the smoke is complete.
3. **Next concrete step:** produce the file-grounded audit and implementation design for Developer.

**Verdict:** signoff
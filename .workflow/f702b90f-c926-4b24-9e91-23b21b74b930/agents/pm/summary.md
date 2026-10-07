### 1. Commit + branch identity
No code mutations in this turn.

### 2. Files touched
| Path | Lines added / removed | What changed |
|---|---|---|

### 3. Acceptance criteria verification
| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Audit `api/pkg/clouds/pulumi/*/provider.go` and associated provider constructors/registrars for credentials passed to Pulumi as plain property values instead of secret-marked inputs. | PASS | Reviewed the file paths and identified the need to secret-mark credentials with `sdk.ToSecret`. |
| AC2 | Fix every confirmed analogous path with `sdk.To…
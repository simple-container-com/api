[thinking] # Handoff — Architect (Dan Johnson), run fadc2560-8262-4860-bab6-19a04d636919

## 0. Context recap

Roadmap item: `sc-stack-outputs-leaks-secrets-in-clear` (repo: `simple-container-com/api`, item file in `simple-container-com/forge` at `docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`). 2nd dispatch attempt — the 1st (run `32521357`) crashed Developer 3× on `exit status 143` and left no code. I pulled the actual current source (not guesswork) from `simple-container-com/api` at the current default-branch HEAD, commit `c374567212784a225a7fd69129fc58ad6496a5c5` (`fix: six bugs found by the first live smoke of the k3s/byo path (#453)`, 2026-10-18T07:05:21Z), to ground this design.

## 1. Commit + branch identity

No code mutations in this turn. (Architect role — design only.) All file paths and line-level claims below are READ from `simple-container-com/api` at commit `c374567212784a225a7fd69129fc58ad6496a5c5`, not invented — each is backed by a `read_repository_file`/`search_repository_code` call made in this turn.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|

(Empty — Architect turn produces no source mutations, per the handoff contract.)

## 3. Design

### 3.1 Where the leak actually is (confirmed from source)

`pkg/clouds/pulumi/preview.go`, `toOutputsResult`:

```go
func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap) *api.OutputsResult {
	return &api.OutputsResult{
		StackName: stackName,
		Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
			var res string
			if s, ok := value.Value.(string); ok {
				res = s
			} else if m, ok := value.Value.(map[string]any); ok {
				j, _ := json.Marshal(m)
				res = string(j)
			} else if value.Secret {
				j, _ := json.Marshal(value)
				res = string(j)
			}
			return res
		}),
	}
}
```

Three paths through this function, **none of which check `value.Secret` before emitting plaintext**:
1. string value → `res = s` (the kubeconfig/join-token case — `value.Secret` is read nowhere here).
2. map value → marshaled as-is.
3. everything else (only reached when not string/map) → marshals the **whole `OutputValue` struct**, which embeds `Value` in the clear — so even the one branch that *looks* like it's checking `Secret` leaks it anyway, it's just dead code as written for the detection, not the masking.

Caller chain (confirmed): `cmd_outputs.go: NewOutputsCmd` → `sCmd.Root.Provisioner.Outputs(ctx, sCmd.Params)` (`pkg/provisioner/preview.go: (p *provisioner) Outputs`) → `pv.OutputsStack(ctx, cfg, stack, params)` (interface in `pkg/api/mapping.go`, implemented by `pkg/clouds/pulumi/preview.go: (p *pulumi) OutputsStack`) → `p.toOutputsResult(...)`. `cmd_outputs.go` then does `json.Marshal(*res)` and `fmt.Println` — no masking layer exists anywhere in that chain today.

### 3.2 Flag threading

`api.StackParams` (`pkg/api/client.go`) is the struct already threaded end-to-end from CLI flag → provisioner → pulumi backend for `SkipRefresh`/`SkipPreview`/`DetailedDiff` (same struct, same pattern, confirmed in `pkg/cmd/root_cmd/root.go: RegisterStackFlags`). Add one field:

```go
// StackParams (pkg/api/client.go)
ShowSecrets bool `json:"showSecrets" yaml:"showSecrets"`
```

Register the flag in `pkg/cmd/cmd_stack/cmd_outputs.go` (NOT in `RegisterStackFlags`/`root_cmd.go` — that function is shared by `deploy`/`destroy`/`cancel`/`secret-get` too, and this flag is meaningful only for `outputs`; follow the `cmd_destroy.go` precedent of registering an operation-specific flag directly on its own `cobra.Command`, not polluting the shared registrar):

```go
cmd.Flags().BoolVar(&sCmd.Params.ShowSecrets, "show-secrets", false,
    "Reveal secret-marked resource outputs in plaintext (default: masked)")
```

This reuses the exact flag name the roadmap item's own prose and PM's AC2 already settled on (`--show-secrets`) — it does not exist in code today (confirmed: zero hits besides the roadmap/docs text), so there is no collision to resolve.

Threading: `StackParams.ShowSecrets` flows unchanged through `Outputs(ctx, params)` → `OutputsStack(ctx, cfg, stack, params)` (the interface signature in `pkg/api/mapping.go` already takes the full `StackParams`, so **no interface signature change is needed** — a real constraint-avoidance win, since changing the `Provisioner` interface would require touching every implementer, including `pkg/api/testhelpers_test.go: noopProvisioner` and `pkg/clouds/pulumi/mocks/pulumi_mock.go: PulumiMock`, both confirmed to implement `OutputsStack(ctx, cfg, stack, params)` already receiving `params`).

### 3.3 The masking function

`toOutputsResult` needs the reveal flag:

```go
// pkg/clouds/pulumi/preview.go
func (p *pulumi) OutputsStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) (*api.OutputsResult, error) {
	...
	return p.toOutputsResult(stackSource.Name(), res, params.ShowSecrets), nil
}

const secretOutputPlaceholder = "<secret:redacted>"

func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap, showSecrets bool) *api.OutputsResult {
	return &api.OutputsResult{
		StackName: stackName,
		Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
			if value.Secret && !showSecrets {
				return secretOutputPlaceholder
			}
			var res string
			if s, ok := value.Value.(string); ok {
				res = s
			} else if m, ok := value.Value.(map[string]any); ok {
				j, _ := json.Marshal(m)
				res = string(j)
			} else if value.Secret {
				j, _ := json.Marshal(value)
				res = string(j)
			}
			return res
		}),
	}
}
```

Decision: **mask by checking `value.Secret` first, before any type-branch resolves a string** — not a post-hoc string-replace over the resolved value. Rationale, and why AC3's "reuse `logger.Redact`?" question resolves to NO:

- `logger.Redact()` (`pkg/api/logger/redact.go`) is a **pattern-matching** redactor — it regex-hunts for PEM blocks, `private_key_id`, and base64 runs that decode to `"PRIVATE KEY"`. It has no way to know a k3s join token or a kubeconfig bearer token is sensitive — those aren't PEM-shaped and won't match any of its regexes (confirmed: `privateKeyBlock`, `privateKeyTail`, `privateKeyID`, `base64Run`/`holdsPrivateKey` — none fire on a bearer token string). Reusing it here would be a **false sense of coverage**: the exact credential this task exists to fix (join token) would sail through unmasked.
- The task's actual signal is structural, not textual: Pulumi's own secret-tracking (`auto.OutputValue.Secret`) already knows which outputs are sensitive — the engine marks them when the underlying resource output is wired as a `Secret` in the Pulumi program (same mechanism `credentials_test.go`'s `TestProvider_KubeconfigIsSecret` asserts on the provider-input side). Masking on `value.Secret` is cheap, precise, and zero-false-negative for every output the resource author already flagged — which is the entire point of AC1.
- This is the right call per AC3's own escape hatch ("if it can't [reuse the existing mechanism], that's fine, but note why") — noted above.

### 3.4 Doc update (AC7)

`docs/docs/examples/k3s/byo-host/README.md` — confirmed current text (read in full this turn):

> "There is no `--show-secrets` and no `-o`. Secret-marked outputs are printed **in the clear**, always — hence the `umask` above."

This sentence becomes false once AC1/AC2 ship. Developer should replace that bullet with something like: *"Secret-marked outputs are masked by default (`<secret:redacted>`); pass `--show-secrets` to see the real value. The `umask` guidance still applies when you do pass it."* Keep the rest of the "Checking the cluster by hand" section (the `-e` omission gotcha, the key-naming convention `<cluster-resource>--<env>-kubeconfig`) unchanged — those remain true.

### 3.5 Test plan (AC4)

Co-locate with the existing masking-adjacent tests, same package (`pkg/clouds/pulumi`), same style as `preview_redact_test.go`:

```go
// pkg/clouds/pulumi/preview_secrets_test.go  (new file)
func TestOutputsResult_MasksSecretOutputsByDefault(t *testing.T) {
	p := &pulumi{}
	outputs := auto.OutputMap{
		"kubeconfig": {Value: "apiVersion: v1\n...", Secret: true},
		"endpoint":   {Value: "https://1.2.3.4:6443", Secret: false},
	}
	masked := p.toOutputsResult("stack", outputs, false /* showSecrets */)
	if masked.Outputs["kubeconfig"] != secretOutputPlaceholder {
		t.Errorf("secret output leaked: %v", masked.Outputs["kubeconfig"])
	}
	if masked.Outputs["endpoint"] != "https://1.2.3.4:6443" {
		t.Errorf("non-secret output was altered: %v", masked.Outputs["endpoint"])
	}

	revealed := p.toOutputsResult("stack", outputs, true /* showSecrets */)
	if revealed.Outputs["kubeconfig"] != outputs["kubeconfig"].Value {
		t.Errorf("--show-secrets did not reveal real value: %v", revealed.Outputs["kubeconfig"])
	}
}
```

Covers AC4(a)/(b)/(c) in one test, same shape as `TestPreviewAndUpdateSummariesAreRedacted`'s "construct, assert never-leaks / assert-passes-through" structure. Developer should ALSO add a case for the map-valued branch (`value.Value.(map[string]any)` with `Secret:true`) since that's the second live-leaking branch — the k3s resource may export the join token as part of a structured output rather than a bare string; confirm against the actual `k3s-cluster` resource's output shape in `pkg/clouds/pulumi/k3s/cluster.go` before assuming it's always a flat string.

### 3.6 Scope boundaries (unchanged from PM, reaffirmed)

- Touch only `sc stack outputs`. Do not touch `secret-get` (different, intentional disclosure model — caller already names the one secret they want).
- Do not build a generic "secrets-aware serialization" framework. One placeholder constant, one bool flag, one `if` — this is a 15-line fix plus tests plus one doc paragraph. Resist the urge to add a `MaskingPolicy` interface or similar; there is exactly one call site and one output shape to cover.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Mask secret-marked outputs by default | DESIGNED, not yet implemented | Design in §3.3 — gate is `value.Secret && !showSecrets` checked before string/map resolution, closing all 3 current leak branches in `toOutputsResult` (confirmed via `read_repository_file` on `pkg/clouds/pulumi/preview.go`, commit `c374567212784a225a7fd69129fc58ad6496a5c5`). |
| AC2 | `--show-secrets` opt-in flag reveals plaintext | DESIGNED | §3.2 — new `StackParams.ShowSecrets` field + flag registered in `cmd_outputs.go`; confirmed zero existing hits for this flag name in code (only in docs/roadmap prose), so no naming collision. |
| AC3 | Reuse existing redaction mechanism if reasonable, else justify | ANALYZED — justified NOT reusing `logger.Redact` | §3.3 — `Redact()` is pattern-based (PEM/base64-private-key detection per `pkg/api/logger/redact.go`, read in full this turn) and has zero chance of catching a bearer token/join token; `value.Secret`-gated masking is the correct, precise mechanism here. |
| AC4 | Unit test proves default-mask + non-secret-untouched + opt-in-reveal | DESIGNED, not yet implemented | Test plan in §3.5, modeled on `pkg/clouds/pulumi/preview_redact_test.go`'s `TestPreviewAndUpdateSummariesAreRedacted` (read in full this turn) and `pkg/clouds/pulumi/kubernetes/credentials_test.go`'s `TestProvider_KubeconfigIsSecret`. |
| AC5 | Live smoke confirms masked-by-default / revealed-with-flag | NOT ATTEMPTED — Architect has no live k3s/byo stack access | Out of scope for this role; DevOps/QA-owned per PM's handoff guidance, contingent on live infra credentials this run may or may not hold. |
| AC6 | Roadmap item status updated appropriately | NOT YET — no code landed this turn | Item file confirmed at `forge:docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md` (read in full this turn); still `status: in-progress`, correct for a design-only turn. Developer/DevOps must flip to `code-complete` (code lands, AC5 not smoked) or `shipped` (AC5 live-verified) — not before. |
| AC7 | `docs/docs/examples/k3s/byo-host/README.md` updated to reflect new behavior | DESIGNED, not yet implemented | Exact current misleading sentence quoted verbatim in §3.4 (read in full this turn, path confirmed real — not the `docs/design/` design-doc tree, a different directory); replacement text proposed. |

No AC is marked PASS — this is design-only; implementation is Developer's turn.

## 5. Tests run

None — no code mutated, per Architect role boundary. All claims above are backed by `read_repository_file`/`search_repository_code`/`list_repository_commits` calls made in this turn against `simple-container-com/api`, commit `c374567212784a225a7fd69129fc58ad6496a5c5` on the default branch (`main`).

## 6. Handoff guidance for Developer

1. **Resource-exhaustion guard (carried forward, now doubly important — this is the 2nd crash risk for this exact slice):** scope every build/test invocation. Per this repo's own `CLAUDE.md` (read in full this turn): use `make check` (2.3 GB, vet-only, full-repo-safe) as your default gate, NOT `go build ./...`/`go test ./...`. For the touched packages specifically:
   ```sh
   GOMAXPROCS=1 GOMEMLIMIT=6GiB go vet ./pkg/clouds/pulumi/ ./pkg/cmd/cmd_stack/... ./pkg/api/
   GOMAXPROCS=1 GOMEMLIMIT=6GiB go test ./pkg/clouds/pulumi/ ./pkg/cmd/cmd_stack/...
   ```
   Do not link `cmd/sc` locally (the link alone needs ~13 GB per the Makefile's own measured table) — let CI build the binary. If a scoped command still dies with `137`/`143` and empty stderr, that is the documented OOM signature — report it honestly as non-completion, do not retry the same command, do not silently pass.
2. Exact files to touch: `pkg/api/client.go` (add `ShowSecrets` field to `StackParams`), `pkg/clouds/pulumi/preview.go` (`toOutputsResult` signature + masking logic, new `secretOutputPlaceholder` const), `pkg/cmd/cmd_stack/cmd_outputs.go` (register `--show-secrets` flag), new test file `pkg/clouds/pulumi/preview_secrets_test.go`, and `docs/docs/examples/k3s/byo-host/README.md` (the one misleading sentence).
3. Check the k3s `join token` output's actual Go type in `pkg/clouds/pulumi/k3s/cluster.go` before assuming the map-valued test case in §3.5 is representative — read the resource's actual output map construction, don't guess.
4. Run `golangci-lint`/`gofumpt` scoped to touched files only (per `CLAUDE.md`'s own guidance), not the whole module.
5. Roadmap item update (AC6) and live smoke (AC5) are downstream of Developer — likely QA/DevOps territory, contingent on live k3s/byo credentials that turn may or may not hold. If unavailable, land as `status: code-complete`, not `shipped` — do not overclaim.

**Verdict:** signoff
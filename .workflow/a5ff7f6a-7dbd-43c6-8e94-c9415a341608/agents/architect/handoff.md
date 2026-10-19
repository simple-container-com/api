[thinking] # Architect Handoff — `sc-stack-outputs-leaks-secrets-in-clear` (3rd dispatch)

**No code mutations in this turn.** This is an Architect planning turn — design only, no commits. The PM's plan (handoffs #1/#2) is sound and verified against the live code; this document turns it into an implementation-ready design with exact signatures, call sites, and test skeleton so the Developer can land it in one short, OOM-safe turn.

## 1. Commit + branch identity

No code mutations in this turn. (Architect role — design only.)

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | none (design only this turn) |

## 3. Verified current state (read directly from `main` this turn, not assumed)

- `pkg/api/client.go` — `StackParams` (full struct, confirmed verbatim):
  ```go
  type StackParams struct {
  	StacksDir    string   `json:"stacksDir" yaml:"stacksDir"`
  	StackDir     string   `json:"stackDir" yaml:"stackDir"`
  	Profile      string   `json:"profile" yaml:"profile"`
  	StackName    string   `json:"stack" yaml:"stack"`
  	Environment  string   `json:"environment" yaml:"environment"`
  	ParentEnv    string   `json:"parentEnv" yaml:"parentEnv"`
  	SkipRefresh  bool     `json:"skipRefresh" yaml:"skipRefresh"`
  	SkipPreview  bool     `json:"skipPreview" yaml:"skipPreview"`
  	DetailedDiff bool     `json:"detailedDiff" yaml:"detailedDiff"`
  	Version      string   `json:"version" yaml:"version"`
  	Timeouts     Timeouts `json:",inline" yaml:",inline"`
  	Parent       bool     `json:"parent" yaml:"parent"`
  }
  ```
  No `ShowSecrets` field exists today — PM's finding confirmed, zero hits for `ShowSecrets`/`show-secrets` outside `docs/` prose.

- `pkg/api/client.go` — `OutputsResult` (confirmed verbatim):
  ```go
  type OutputsResult struct {
  	StackName string         `json:"stackName" yaml:"stackName"`
  	Outputs   map[string]any `json:"outputs" yaml:"outputs"`
  }
  ```
  Shape stays untouched — AC5 satisfied by construction if we only change values, not keys.

- `pkg/clouds/pulumi/preview.go` — the exact function to change (confirmed verbatim):
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
  Only ONE call site: `OutputsStack` (same file, line ~70) calls `p.toOutputsResult(stackSource.Name(), res), nil` — no other caller in the repo (confirmed via code search, single hit).

- `pkg/clouds/pulumi/preview.go` — `OutputsStack` signature (confirmed verbatim):
  ```go
  func (p *pulumi) OutputsStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) (*api.OutputsResult, error) {
  	if params.Environment != "" && params.StackName != "" {
  		stack = toChildStack(stack, params)
  	}
  	s, err := p.selectStack(ctx, cfg, stack)
  	...
  	res, err := stackSource.Outputs(ctx)
  	...
  	return p.toOutputsResult(stackSource.Name(), res), nil
  }
  ```
  `params` is already in scope here — threading `params.ShowSecrets` into `toOutputsResult` is a one-line change at this call site, no plumbing elsewhere needed.

- `pkg/cmd/cmd_stack/cmd_outputs.go` — confirmed verbatim, 23 lines, no flags registered today:
  ```go
  func NewOutputsCmd(sCmd *stackCmd) *cobra.Command {
  	cmd := &cobra.Command{
  		Use:   "outputs",
  		Short: "Displays outputs of a stack defined in stacks directory",
  		RunE: func(cmd *cobra.Command, args []string) error {
  			cmd.SetContext(sCmd.Root.Logger.Silent(cmd.Context()))
  			if res, err := sCmd.Root.Provisioner.Outputs(cmd.Context(), sCmd.Params); err != nil {
  				return err
  			} else if j, err := json.Marshal(*res); err != nil {
  				return err
  			} else {
  				fmt.Println(string(j))
  			}
  			return nil
  		},
  	}
  	return cmd
  }
  ```

- `pkg/cmd/cmd_stack/cmd_stack.go` — confirmed verbatim: `stackCmd{Root, Params api.StackParams}`; `NewStackCmd` calls `root_cmd.RegisterStackFlags(cmd, &sCmd.Params, true)` (persistent flags shared across `secret-get` + `outputs`) then `AddCommand(NewSecretGetCmd, NewOutputsCmd)`. **Confirms PM's call: a flag registered on the shared parent `stack` command would leak onto `secret-get` too** — the new flag must be registered directly on the `outputs` sub-command object returned by `NewOutputsCmd`, not via `RegisterStackFlags`.

- `pkg/cmd/root_cmd/root.go` — `RegisterStackFlags(cmd *cobra.Command, p *api.StackParams, persistent bool)` confirmed verbatim; registers `--profile`, `--stack`, `--env`, `--dir`, `--skip-refresh`, `--skip-preview`, `--diff`/`--detailed-diff`. Confirms: do NOT add `ShowSecrets` registration here — it's shared by `cancel`/`destroy`/`stack` parent too.

- `pkg/provisioner/preview.go` — `(*provisioner).Outputs(ctx, params)` confirmed verbatim: routes to `pv.OutputsStack(ctx, cfg, *stack, params)` for child-stack reads, and `pv.OutputsStack(ctx, cfg, stack, params)` for parent-stack reads — both pass `params` through untouched today, so no change needed in this file; `params.ShowSecrets` already arrives at `OutputsStack` once added to `StackParams`.

- Pulumi SDK fact-check (fetched `sdk/go/auto` from `pulumi/pulumi@master` this turn): `auto.OutputValue{Value any; Secret bool}` is the shape already being read at `value.Value` / `value.Secret` in `toOutputsResult` — confirms no SDK-side change needed, the masking purely intercepts after `.Secret` is already inspected.

- `pkg/clouds/pulumi/preview_redact_test.go` (confirmed verbatim, existing precedent file) shows the repo's existing style for testing `toPreviewResult`/`toUpdateResult` directly by constructing `&pulumi{}` and calling the method with fake `auto.*Result` values — directly reusable pattern for the new `toOutputsResult` test (no need for a running Pulumi backend).

## 4. Design — exact diff shape for the Developer

### 4a. `pkg/api/client.go` — add one field to `StackParams`

```go
type StackParams struct {
	StacksDir    string   `json:"stacksDir" yaml:"stacksDir"`
	StackDir     string   `json:"stackDir" yaml:"stackDir"`
	Profile      string   `json:"profile" yaml:"profile"`
	StackName    string   `json:"stack" yaml:"stack"`
	Environment  string   `json:"environment" yaml:"environment"`
	ParentEnv    string   `json:"parentEnv" yaml:"parentEnv"`
	SkipRefresh  bool     `json:"skipRefresh" yaml:"skipRefresh"`
	SkipPreview  bool     `json:"skipPreview" yaml:"skipPreview"`
	DetailedDiff bool     `json:"detailedDiff" yaml:"detailedDiff"`
	ShowSecrets  bool     `json:"showSecrets" yaml:"showSecrets"` // NEW: reveal secret-marked stack outputs in the clear (default: masked)
	Version      string   `json:"version" yaml:"version"`
	Timeouts     Timeouts `json:",inline" yaml:",inline"`
	Parent       bool     `json:"parent" yaml:"parent"`
}
```

### 4b. `pkg/clouds/pulumi/preview.go` — thread the bool into `toOutputsResult`, add the mask branch

```go
const secretOutputPlaceholder = "<secret:redacted>"

func (p *pulumi) OutputsStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.StackParams) (*api.OutputsResult, error) {
	...
	res, err := stackSource.Outputs(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get outputs")
	}
	return p.toOutputsResult(stackSource.Name(), res, params.ShowSecrets), nil // CHANGED: pass showSecrets
}

func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap, showSecrets bool) *api.OutputsResult { // CHANGED signature
	return &api.OutputsResult{
		StackName: stackName,
		Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
			if value.Secret && !showSecrets {
				return secretOutputPlaceholder // NEW: mask-by-default branch, checked FIRST
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

Rationale for checking `value.Secret && !showSecrets` **before** the existing type-switch rather than inside it: the existing branch order already special-cases `value.Secret` last (as a fallback when the value isn't a string/map) — masking must apply uniformly to *every* secret-marked output regardless of its underlying Go type, not just the fallback case. Putting the check first is also what makes the function trivially testable against AC1–AC3 with one table-driven test covering string-valued secrets, map-valued secrets, and non-secret values of both shapes.

This is the **only** call site of `toOutputsResult` in the repo (confirmed above), so the signature change has exactly one caller to update.

### 4c. `pkg/cmd/cmd_stack/cmd_outputs.go` — register the flag on `outputs` only

```go
func NewOutputsCmd(sCmd *stackCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outputs",
		Short: "Displays outputs of a stack defined in stacks directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			...
		},
	}
	cmd.Flags().BoolVar(&sCmd.Params.ShowSecrets, "show-secrets", sCmd.Params.ShowSecrets, "Reveal secret-marked stack outputs in the clear (default: masked)") // NEW
	return cmd
}
```
This is additive to the shared `sCmd.Params` struct already registered by `RegisterStackFlags` on the parent `stack` command — registering `--show-secrets` directly on the `outputs` sub-command (not via `RegisterStackFlags`) means `secret-get` does not gain the flag, matching PM scope item #2.

### 4d. Docs (nice-to-have, same PR if time allows)
`docs/docs/examples/k3s/byo-host/README.md`, the paragraph starting "There is no `--show-secrets` and no `-o`. Secret-marked outputs are printed **in the clear**, always" is now factually wrong post-fix. Replace with: outputs are masked by default (`<secret:redacted>`); `--show-secrets` reveals plaintext; the `umask 077` + redirect-to-file guidance should move to apply specifically to the `--show-secrets` invocation.

## 5. Acceptance criteria — design-level verification

| AC# | Criterion | Design status | Where it's satisfied |
|---|---|---|---|
| AC1 | Default invocation masks secret-marked outputs to a fixed placeholder | By design | `toOutputsResult`'s new first branch: `if value.Secret && !showSecrets { return secretOutputPlaceholder }`; `showSecrets` defaults to Go zero-value `false` |
| AC2 | `--show-secrets` reveals plaintext (today's behavior unchanged) | By design | `showSecrets=true` skips the mask branch entirely, falls through to the pre-existing type-switch byte-for-byte |
| AC3 | Non-secret outputs print in plaintext in both invocations | By design | Mask branch is gated on `value.Secret`; non-secret values never enter it regardless of `showSecrets` |
| AC4 | Unit test proving (a)(b)(c) against `toOutputsResult` with a fake `auto.OutputMap` | Specified, not yet written | New `_test.go` in `pkg/clouds/pulumi`, pattern below — Developer to write and run |
| AC5 | No change to `OutputsResult`'s JSON shape | By design | `OutputsResult{StackName, Outputs}` untouched; only `Outputs` map *values* change for secret keys |
| AC6 | `make check` lint/vet-clean on touched packages | Not yet run (no code written this turn) | Developer must run `make check` + scoped `go test` per OOM discipline below |

## 6. Test skeleton for the Developer (AC4)

New file `pkg/clouds/pulumi/outputs_mask_test.go`, following the `preview_redact_test.go` precedent (construct `&pulumi{}`, call the method directly, no live backend):

```go
// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func TestToOutputsResult_MasksSecretsByDefaultAndRevealsOnOptIn(t *testing.T) {
	result := auto.OutputMap{
		"kubeconfig": auto.OutputValue{Value: "apiVersion: v1\nkind: Config", Secret: true},
		"joinToken":  auto.OutputValue{Value: "K10abc::server:xyz", Secret: true},
		"ingressIP":  auto.OutputValue{Value: "203.0.113.10", Secret: false},
	}
	p := &pulumi{}

	masked := p.toOutputsResult("mycompany", result, false /* showSecrets */)
	if masked.Outputs["kubeconfig"] != secretOutputPlaceholder || masked.Outputs["joinToken"] != secretOutputPlaceholder {
		t.Errorf("secret outputs not masked by default: %+v", masked.Outputs)
	}
	if masked.Outputs["ingressIP"] != "203.0.113.10" {
		t.Errorf("non-secret output altered: %+v", masked.Outputs)
	}

	revealed := p.toOutputsResult("mycompany", result, true /* showSecrets */)
	if revealed.Outputs["kubeconfig"] != "apiVersion: v1\nkind: Config" || revealed.Outputs["joinToken"] != "K10abc::server:xyz" {
		t.Errorf("--show-secrets did not reveal plaintext: %+v", revealed.Outputs)
	}
	if revealed.Outputs["ingressIP"] != "203.0.113.10" {
		t.Errorf("non-secret output altered on reveal: %+v", revealed.Outputs)
	}
}
```
(Credential-shaped strings above — `K10abc::server:xyz`, the kubeconfig snippet — are illustrative test fixtures, not real secrets; they match the two leak types named in the brief for test realism.)

## 7. Mandatory build discipline for the Developer (carried forward — this is the 3rd dispatch attempt; both priors died here with zero commits)

- **Never** run `go build ./...`, `go test ./...`, `make build`, or `make build-all` — the link step alone measures ~13GB RSS in this module (confirmed in `CLAUDE.md`, read this turn) against a 12GB guest with no swap.
- Use only: `make check` (capped `go vet ./...`, measured 2.3GB) and narrowly-scoped `go test ./pkg/clouds/pulumi/...` and `go test ./pkg/cmd/cmd_stack/...`.
- Lint scoped to touched files only: `bin/gofumpt -l -w ./pkg/clouds/pulumi/ ./pkg/cmd/cmd_stack/ ./pkg/api/` then `bin/golangci-lint run --fix ./pkg/clouds/pulumi/... ./pkg/cmd/cmd_stack/... ./pkg/api/...` — not the whole-module `make lint`.
- Keep the turn short: commit before resume 5–10 (guest is not recycled between resumes; free RAM decays even for `make check`-class commands across a long multi-resume turn).
- If any command appears to hang or spikes memory, kill it immediately — do not retry the same command; `signal: killed` / `exit status 143` with empty stderr is the guest OOM signature, not a code problem (per `docs/roadmap/items/api-repo-slices-oom-the-guest.md` in the `forge` repo, referenced in the dispatch brief).
- DCO sign-off required on every commit (`git commit -s`) — enforced by `.github/workflows/dco.yml`, confirmed in `CLAUDE.md`.

## 8. Out-of-scope (unchanged from PM plan, re-confirmed against code)

- `RegisterStackFlags` in `pkg/cmd/root_cmd/root.go` stays untouched — confirmed it's shared by `cancel`/`destroy`/the parent `stack` command; widening it would leak `--show-secrets` onto unrelated subcommands.
- Pulumi preview/update summary redaction (`logger.Redact`, `toPreviewResult`/`toUpdateResult`) is a separate, already-solved code path (see `preview_redact_test.go`) — not touched.
- `sc secrets reveal`/`sc secrets hide` (repo-committed secrets store mechanism) — unrelated, brief asks only to mirror the UX pattern, not share code.
- No new output format (`-o`) or structural JSON change — `OutputsResult` shape is fixed per AC5.

## 9. Handoff guidance for Developer

Four files to touch, in this order, committing after each if resume budget is tight:
1. `pkg/api/client.go` — add `ShowSecrets bool` field (§4a).
2. `pkg/clouds/pulumi/preview.go` — change `toOutputsResult` signature + mask branch, update its one call site in `OutputsStack` (§4b).
3. `pkg/cmd/cmd_stack/cmd_outputs.go` — register `--show-secrets` on the `outputs` subcommand only (§4c).
4. New test file `pkg/clouds/pulumi/outputs_mask_test.go` per §6, run `go test ./pkg/clouds/pulumi/... -run TestToOutputsResult_MasksSecretsByDefaultAndRevealsOnOptIn`.
Then `make check`, scoped lint (§7), docs update (§4d) if time allows, commit with `-s`, open PR.

**Verdict:** signoff
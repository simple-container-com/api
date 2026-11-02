// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// Collector is the plugin-side implementation of
// pApi.ComputeContextCollector. Every SC plugin's Provision and
// ComputeProcess uses it to record what its in-plugin driver emits — env
// variables, template extensions, and the sdk.Output chains those are
// resolved from — and (in ComputeProcess) hand that back to SC core as a
// scpluginv1.ComputeProcessResponse.
//
// # Why this type exists in a shared package (not re-written per plugin)
//
// sc-plugin-aws's per-plugin collector had `AddOutput(_ sdk.Output) {}`
// as a no-op. Every ApplyT-wrapped env mutation it ran was discarded.
// ComputeProcess returned "emitted 0 env var(s)" with no error, the
// deploy went green, consumer Lambdas booted without their `uses:` env,
// health checks passed, uploads failed. Live-caught on
// agent-native-pdf-rendering run f4355798 (2026-11-02). See
// forge/docs/roadmap/items/sc-plugin-aws-s3-compute-process-drops-env-silently.md.
//
// sc-plugin-mongodb had the correct capture + drain. The fix never
// reached aws because capturingCollector was per-plugin and the Slice B/C
// runtime port lifted drain / harvest / Settle / parent-env swap into
// shared code but left this one behind. sc-plugin-yandex has the
// identical defect — `ObjectStorageBucketComputeProcessor`
// (plugin/internal/compute_proc.go:82) wraps every S3_* env mutation in
// an ApplyT and the plugin's AddOutput is a no-op. Hasn't bitten only
// because no stack currently declares `uses: [<yc-bucket>]`;
// forge-atrium's S6 (YC Mongo + RU storage) is the trigger.
//
// # How this type closes the failure mode
//
// Hoisting the collector code alone would not close trap 16 — a plugin
// author who forgets to call `AwaitOutputs` between the processor and
// `ToResponse` reproduces the aws bug exactly, with a shared
// implementation behind it. Call-site discipline isn't the fix.
//
// So the drain is MANDATORY by construction: the methods that marshal a
// response and drain the outputs are PACKAGE-PRIVATE
// (`toResponse` / `awaitOutputs`), and the only exported path to a
// response is `RunComputeProcess`, which owns the fn → drain → marshal
// sequence. A plugin's ComputeProcess handler never touches
// `ToResponse` or `AwaitOutputs` directly, so it cannot omit the drain
// even by accident.
//
// # The incoming/outgoing pool split (trap 14)
//
// A plugin that runs BOTH Provision (reads INCOMING env vars from the
// consuming stack's `uses:` resources via
// ProvisionRequest.compute_env_vars) AND ComputeProcess (writes OUTGOING
// env vars from its own resources) in the same deploy must separate the
// pools or confuse the two. See
// forge/docs/design/2026-11-01/plugin-extraction-playbook/ trap 14 for
// the forge-notifier incident that produced this split.
//
// `NewProvisionCollector(req)` seeds the incoming pool from the request;
// `RunComputeProcess` constructs an empty one (ComputeProcess has no
// incoming context — the consuming stack's env context arrives through
// `ProvisionRequest`, not through `ComputeProcessRequest`).
type Collector struct {
	mu sync.Mutex

	// Incoming — the consuming stack's `uses:` env, seeded from
	// ProvisionRequest.compute_env_vars by NewProvisionCollector. Read
	// by the in-plugin template driver (Lambda / ECS / static-site /
	// Serverless Container) via EnvVariables() / SecretEnvVariables()
	// to populate the deployed service's env.
	incomingEnvVars []pApi.ComputeEnvVariable
	incomingSecrets []pApi.ComputeEnvVariable

	// Outgoing — what the plugin's own compute processors added via
	// AddEnvVariableIfNotExist / AddSecretEnvVariableIfNotExist /
	// AddResourceTplExtension / AddDependencyTplExtension during a
	// ComputeProcess RPC. `toResponse` packages these into the response
	// SC core carries back to the deploying stack's own compute context.
	outgoingEnvVars []pApi.ComputeEnvVariable
	resourceTpl     []resourceTplEntry
	dependencyTpl   []dependencyTplEntry

	// Captured sdk.Output chains the in-plugin processor passed to
	// AddOutput. The env-variable mutations above are typically added
	// FROM INSIDE an ApplyT on an output in this slice — e.g. mongodb
	// adds every MONGO_* var from inside an ApplyT on the DatabaseUser
	// resource's password Output. `awaitOutputs` drains this list
	// before `toResponse` or the mutations are lost silently.
	outputs []sdk.Output
}

type resourceTplEntry struct {
	resName string
	values  map[string]string
}

type dependencyTplEntry struct {
	depName string
	resName string
	values  map[string]string
}

// NewProvisionCollector builds a Collector seeded from a Provision
// request's compute_env_vars so EnvVariables() / SecretEnvVariables()
// return what the consuming stack's `uses:` resources contributed.
//
// Pass nil for a Provision context that genuinely has no incoming env
// (`type: static` templates with no `uses:`, for instance); the
// incoming pool stays empty.
//
// Provision never calls `ToResponse` — the ProvisionResponse that
// crosses the bus carries HarvestExports output, not collector
// mutations. So this constructor is write-safe for the plugin's
// template driver and does not need the drain/marshal path.
func NewProvisionCollector(req *scpluginv1.ProvisionRequest) *Collector {
	c := &Collector{}
	if req == nil {
		return c
	}
	for _, v := range req.GetComputeEnvVars() {
		// Defensive: a nil entry or one with no name is a wire artefact
		// (unset proto field, protobuf's nil-safe getters would otherwise
		// synthesise an empty env var in the pool).
		if v == nil || v.GetName() == "" {
			continue
		}
		entry := pApi.ComputeEnvVariable{
			Name:         v.GetName(),
			Value:        v.GetValue(),
			Secret:       v.GetSecret(),
			ResourceName: v.GetResourceName(),
			ResourceType: v.GetResourceType(),
			StackName:    v.GetStackName(),
		}
		if v.GetSecret() {
			c.incomingSecrets = append(c.incomingSecrets, entry)
		} else {
			c.incomingEnvVars = append(c.incomingEnvVars, entry)
		}
	}
	return c
}

// --- pApi.ComputeContextCollector surface -----------------------------
//
// The methods below satisfy the pApi.ComputeContextCollector interface
// that in-plugin drivers read and write through. All are exported;
// what is NOT exported is `toResponse` + `awaitOutputs` — the
// drain-and-marshal path that must stay in RunComputeProcess's hands.

// EnvVariables returns the incoming (non-secret) compute env vars.
// Readers: the per-plugin template driver that populates a deployed
// container's env.
func (c *Collector) EnvVariables() []pApi.ComputeEnvVariable {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]pApi.ComputeEnvVariable(nil), c.incomingEnvVars...)
}

// SecretEnvVariables returns the incoming SECRET compute env vars. Same
// reader shape as EnvVariables; the Secret flag drives the template to
// resolve the value via a secrets manager rather than baking it into
// the env directly.
func (c *Collector) SecretEnvVariables() []pApi.ComputeEnvVariable {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]pApi.ComputeEnvVariable(nil), c.incomingSecrets...)
}

// AddEnvVariableIfNotExist appends to the OUTGOING pool. The name is
// historical (pApi's interface shape) — this implementation does not
// deduplicate, since the SC-core sidecar's own IfNotExist semantics on
// the consuming stack's collector handle that.
func (c *Collector) AddEnvVariableIfNotExist(name, value, resType, resName, stackName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outgoingEnvVars = append(c.outgoingEnvVars, pApi.ComputeEnvVariable{
		Name: name, Value: value, ResourceType: resType, ResourceName: resName, StackName: stackName, Secret: false,
	})
}

// AddSecretEnvVariableIfNotExist appends to the outgoing pool with Secret=true.
func (c *Collector) AddSecretEnvVariableIfNotExist(name, value, resType, resName, stackName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outgoingEnvVars = append(c.outgoingEnvVars, pApi.ComputeEnvVariable{
		Name: name, Value: value, ResourceType: resType, ResourceName: resName, StackName: stackName, Secret: true,
	})
}

// AddResourceTplExtension appends to the resource-template extension list.
func (c *Collector) AddResourceTplExtension(resName string, values map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resourceTpl = append(c.resourceTpl, resourceTplEntry{resName: resName, values: values})
}

// AddDependencyTplExtension appends to the dependency-template extension list.
func (c *Collector) AddDependencyTplExtension(depName, resName string, values map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dependencyTpl = append(c.dependencyTpl, dependencyTplEntry{depName: depName, resName: resName, values: values})
}

// AddOutput records an sdk.Output whose resolution mutates this
// collector. RunComputeProcess drains the captured slice before
// marshalling, so an ApplyT callback that adds env vars is guaranteed to
// have run before the response is built.
//
// This is the fix for trap 16 — the per-plugin implementations had
// this as `AddOutput(_ sdk.Output) {}`, so every env mutation written
// from inside an ApplyT was silently discarded.
func (c *Collector) AddOutput(_ *sdk.Context, o sdk.Output) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outputs = append(c.outputs, o)
}

// Outputs returns a copy of the captured output slice. Exposed for
// pApi.ComputeContextCollector; RunComputeProcess uses `awaitOutputs`
// directly instead.
func (c *Collector) Outputs() []sdk.Output {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]sdk.Output(nil), c.outputs...)
}

// The pApi.ComputeContextCollector methods below carry Pulumi
// sdk.Resource handles or Go closures that do not cross the plugin
// bus. A plugin that needs them applies them against its own
// sdk.Context before Build. See api/pkg/plugin/compute.go for the
// general rule.
func (c *Collector) AddDependency(sdk.Resource)                         {}
func (c *Collector) Dependencies() []sdk.Resource                       { return nil }
func (c *Collector) ResolvePlaceholders(any) error                      { return nil }
func (c *Collector) AddPreProcessor(any, pApi.PreProcessor)             {}
func (c *Collector) AddPostProcessor(any, pApi.PostProcessor)           {}
func (c *Collector) GetPreProcessors(any) ([]pApi.PreProcessor, bool)   { return nil, false }
func (c *Collector) GetPostProcessors(any) ([]pApi.PostProcessor, bool) { return nil, false }
func (c *Collector) RunPreProcessors(any, any) error                    { return nil }
func (c *Collector) RunPostProcessors(any, any) error                   { return nil }
func (c *Collector) ResourceOutputs() pApi.ResourcesOutputs             { return nil }

// --- Internal drain + marshal (unexported by design) ------------------

// awaitOutputs blocks until every output added via AddOutput has
// resolved, so the mutations their ApplyT callbacks produced are
// recorded in the outgoing pool. An output that resolves to an error
// fails the compute process rather than silently dropping its env.
//
// Drains until the slice stops growing: an ApplyT callback that itself
// calls AddOutput (e.g. two sequential StackReference reads where the
// second needs a value from the first) appends after the first drain
// pass, and a single `for _, o := range c.Outputs()` snapshot would
// silently drop the nested ones. The outer loop reads a fresh snapshot
// each iteration and stops only when a pass adds no new outputs. No
// current processor does this (aws, yandex and mongodb each have one
// non-nested AddOutput), but kill the class — this is the same
// silent-loss shape as trap 16 one level down.
//
// This is unexported: the only correct caller is RunComputeProcess
// below, which brackets the drain between the plugin's handler and
// toResponse. Making this public would reproduce the opt-in failure
// mode this file exists to eliminate.
func (c *Collector) awaitOutputs(ctx context.Context) error {
	drained := 0
	for {
		outs := c.Outputs()
		if len(outs) == drained {
			return nil
		}
		for _, o := range outs[drained:] {
			if _, err := internals.UnsafeAwaitOutput(ctx, o); err != nil {
				return errors.Wrap(err, "await compute-context output")
			}
		}
		drained = len(outs)
	}
}

// toResponse builds the ComputeProcessResponse the plugin bus carries
// back to SC core. Only the outgoing mutations are shipped — the
// incoming pool is per-Provision context and does not re-cross the bus.
//
// Unexported for the same reason as awaitOutputs — a direct call
// skipping the drain would silently return an empty response.
func (c *Collector) toResponse() *scpluginv1.ComputeProcessResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp := &scpluginv1.ComputeProcessResponse{}
	for _, v := range c.outgoingEnvVars {
		resp.EnvVars = append(resp.EnvVars, &scpluginv1.ComputeEnvVariable{
			Name:         v.Name,
			Value:        v.Value,
			Secret:       v.Secret,
			ResourceName: v.ResourceName,
			ResourceType: v.ResourceType,
			StackName:    v.StackName,
		})
	}
	for _, t := range c.resourceTpl {
		resp.TplExtensions = append(resp.TplExtensions, &scpluginv1.TemplateExtension{
			Kind:    scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_RESOURCE,
			ResName: t.resName,
			Values:  t.values,
		})
	}
	for _, t := range c.dependencyTpl {
		resp.TplExtensions = append(resp.TplExtensions, &scpluginv1.TemplateExtension{
			Kind:    scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_DEPENDENCY,
			DepName: t.depName,
			ResName: t.resName,
			Values:  t.values,
		})
	}
	return resp
}

// --- ComputeProcess wrapper (the only path to a response) -------------

// ComputeProcessHandler is the signature every in-plugin compute
// processor has — same shape as pApi.ComputeProcessorFunc, lifted here
// so plugins don't need to import pApi in their dispatch glue.
type ComputeProcessHandler func(
	ctx *sdk.Context,
	stack api.Stack,
	input api.ResourceInput,
	collector pApi.ComputeContextCollector,
	params pApi.ProvisionParams,
) (*api.ResourceOutput, error)

// RunComputeProcess owns the ComputeProcess sequence end-to-end:
//
//  1. Build a fresh Collector (empty incoming pool — ComputeProcess
//     has no `uses:` context on the wire).
//  2. Call the plugin's handler, which emits via the collector's
//     exported pApi.ComputeContextCollector methods.
//  3. Drain the captured sdk.Output chains so every ApplyT-wrapped env
//     mutation is in the outgoing pool.
//  4. Marshal the outgoing pool into a ComputeProcessResponse.
//
// The plugin's handler cannot see the response marshal step, so it
// cannot omit the drain. This is the only exported path to a
// ComputeProcessResponse from this package, by design.
//
// The urn field of the returned response is left to the caller to set
// — the plugin's dispatch layer typically fills it from the handler's
// *api.ResourceOutput, so RunComputeProcess returns both for the
// caller to compose.
func RunComputeProcess(
	stdctx context.Context,
	rctx *Context,
	stack api.Stack,
	input api.ResourceInput,
	params pApi.ProvisionParams,
	handler ComputeProcessHandler,
) (*scpluginv1.ComputeProcessResponse, *api.ResourceOutput, error) {
	if rctx == nil || rctx.Pulumi == nil {
		return nil, nil, errors.New("runtime.RunComputeProcess: nil Context — call runtime.NewContext first")
	}
	if handler == nil {
		return nil, nil, errors.New("runtime.RunComputeProcess: nil handler")
	}

	collector := &Collector{}
	out, err := handler(rctx.Pulumi, stack, input, collector, params)
	if err != nil {
		return nil, nil, err
	}
	if err := collector.awaitOutputs(stdctx); err != nil {
		return nil, nil, err
	}
	return collector.toResponse(), out, nil
}

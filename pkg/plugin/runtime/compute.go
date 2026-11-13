// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// ParentInfoFromRequest unmarshals the ParentInfoJson field a compute-process
// request carries into a ParentInfo struct. Returns a nil ParentInfo when
// the request has no parent (every non-compute-process RPC).
//
// IMPORTANT: ParentInfo is serialised by SC core via json.Marshal with the
// struct's capitalised Go field names (StackName / FullReference), and the
// struct has no yaml tags. Using json.Unmarshal here matches what the wire
// actually carries. A yaml.Unmarshal would quietly land every field empty.
func ParentInfoFromRequest(req *scpluginv1.ComputeProcessRequest) (*pApi.ParentInfo, error) {
	raw := req.GetParentInfoJson()
	if len(raw) == 0 {
		return nil, nil
	}
	parent := &pApi.ParentInfo{}
	if err := json.Unmarshal(raw, parent); err != nil {
		return nil, errors.Wrap(err, "decode ParentInfo from compute-process request")
	}
	return parent, nil
}

// ParentInfoFromProvisionRequest decodes the ParentInfoJson field a plugin's
// Provision RPC carries when the deploying stack has a parent. Returns a nil
// ParentInfo when the field is empty — either no parent (a parent-stack
// provision itself, not a child) or an older SC core that pre-dates the
// field (`sc-plugin-provisionrequest-missing-parent-info` roadmap item).
//
// Plugin templates that reach into `params.ParentStack.*` MUST nil-check
// the returned pointer AND the pointer they set on ProvisionParams. The
// two guardrails ship together: the wire path is only half the fix; the
// template body's nil check is the other half, because an older plugin
// binary talking to a newer core, or a preview-only client with no
// parent, both produce a nil ParentStack that nil-derefs in the
// template's internal/* body otherwise.
//
// Same ParentInfo shape (serialised Go-field-named JSON) as the compute
// process path.
func ParentInfoFromProvisionRequest(req *scpluginv1.ProvisionRequest) (*pApi.ParentInfo, error) {
	raw := req.GetParentInfoJson()
	if len(raw) == 0 {
		return nil, nil
	}
	parent := &pApi.ParentInfo{}
	if err := json.Unmarshal(raw, parent); err != nil {
		return nil, errors.Wrap(err, "decode ParentInfo from provision request")
	}
	return parent, nil
}

// DependencyOutputsFromProvisionRequest unmarshals the
// DependencyOutputsJson field a provision request carries into the shape a
// plugin's Provision body reads via ProvisionParams.ResourceOutputs.
// Returns a nil map when the request carries no dependencies (every
// resource whose config does NOT implement api.WithParentDependencies, or
// an older SC core that pre-dates the field).
//
// Shape contract (api PR adding this helper): SC core's
// plugin_sidecars.go packs the map[resourceName]*api.PluginResourceOutput
// from params.ResourceOutputs via json.Marshal. Plugin side decodes into
// the same shape and lifts each entry onto a `*api.ResourceOutput` with
// `Ref: *api.PluginResourceOutput`, which is exactly what sibling
// resources' Provision bodies already type-assert on post-#569. An
// in-tree dep whose Ref was a typed Go struct is NOT packed on the
// sidecar side (the receiving plugin can't unmarshal unknown struct
// shapes); the plugin body reads that as absent and errors normally.
//
// Live-caught 2026-11-08 on sc-plugin-gcp tier-2 smoke: `gcp-docker-image-push`
// read `params.ResourceOutputs[registryResource].Ref.(*ArtifactRegistryOut)`
// and failed with `expected registry resource %q to be present in outputs`
// because this field was declared on the proto (`dependency_outputs = 5`)
// but never populated by any sidecar. The flat string map was too narrow
// to carry PluginResourceOutput; a JSON bytes field matches the shape #569
// introduced.
//
// See forge/docs/roadmap/items/sc-plugin-dependency-outputs-not-populated.md.
func DependencyOutputsFromProvisionRequest(req *scpluginv1.ProvisionRequest) (map[string]*api.ResourceOutput, error) {
	raw := req.GetDependencyOutputsJson()
	if len(raw) == 0 {
		return nil, nil
	}
	var decoded map[string]*api.PluginResourceOutput
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, errors.Wrap(err, "decode DependencyOutputs from provision request")
	}
	result := make(map[string]*api.ResourceOutput, len(decoded))
	for name, plOut := range decoded {
		if plOut == nil {
			continue
		}
		plOutCopy := *plOut
		result[name] = &api.ResourceOutput{Ref: &plOutCopy}
	}
	return result, nil
}

// DependencyOutputsFromComputeProcessRequest unmarshals the
// DependencyOutputsJson field a ComputeProcess request carries into the
// shape a plugin's ComputeProcessor reads. Mirrors
// DependencyOutputsFromProvisionRequest (same JSON shape, same decoding
// semantics) — different request side, same wire contract.
//
// Added as the ComputeProcess-side symmetry of the Provision-side
// dep-outputs fix. Lights up sc-component-backup's slice 1d scoped-user
// plumbing (sc-component-backup 06721b1, design doc §5.2) end-to-end: a
// ComputeProcessor can now see the consumer's sibling-resource outputs
// (mongo-uri, database name, etc.) at `uses:` time + mint per-consumer
// scoped credentials.
//
// Returns a nil map when the request carries no dependencies (older SC
// core, or a resource whose consumers don't opt in).
func DependencyOutputsFromComputeProcessRequest(req *scpluginv1.ComputeProcessRequest) (map[string]*api.ResourceOutput, error) {
	raw := req.GetDependencyOutputsJson()
	if len(raw) == 0 {
		return nil, nil
	}
	var decoded map[string]*api.PluginResourceOutput
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, errors.Wrap(err, "decode DependencyOutputs from compute-process request")
	}
	result := make(map[string]*api.ResourceOutput, len(decoded))
	for name, plOut := range decoded {
		if plOut == nil {
			continue
		}
		plOutCopy := *plOut
		result[name] = &api.ResourceOutput{Ref: &plOutCopy}
	}
	return result, nil
}

// ComponentRawFromComputeProcessRequest unmarshals the ComponentRaw bytes
// a ComputeProcessRequest carries when the ComputeProcess call fires on
// an SC Component resource (whose plugin advertises
// SchemaResponse.supports_component). Mirrors ComponentRawFromProvisionRequest:
// same JSON shape, same decoding semantics, different RPC side.
//
// Lights up the component plugin's compute path: a Component's
// `databases:` + `atlasAdmin:` + ... config was already reachable from
// its Provision via the Provision-side side-channel; the ComputeProcess
// side had no equivalent carrier until the companion proto field landed
// on ComputeProcessRequest. Without this, sc-component-backup's
// `maybeProvisionScopedUser` was a no-op in production because the
// compute processor received an empty Config.Raw.
//
// Opt-in via `SchemaResponse.supports_component` advertise. Nil/empty
// bytes return `(nil, nil)`; that's every non-component ComputeProcess
// call.
func ComponentRawFromComputeProcessRequest(req *scpluginv1.ComputeProcessRequest) (map[string]any, error) {
	raw := req.GetComponentRaw()
	if len(raw) == 0 {
		return nil, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, errors.Wrap(err, "decode ComponentRaw from compute-process request")
	}
	return decoded, nil
}

// ComponentRawFromProvisionRequest unmarshals the ComponentRaw bytes a
// ProvisionRequest carries when SC core dispatched this plugin's template
// via a Component resource's `deployTo:` (slice 1a.1 of
// sc-components-platform; design doc §#19 path (b)).
//
// The returned map carries the operator-declared component config
// (image, env, ingress, signingKey, …) as it appeared in server.yaml.
// Plugin's ProvisionFunc overlays these onto its typed input struct
// before building the cloud resource. Returns nil when the resource was
// NOT dispatched as a Component — every non-component Provision call
// hits this + ignores it.
//
// Only plugins that advertise supports_component in Schema should call
// this; absent advertise + non-empty ComponentRaw is a SC-core parse-time
// refusal, so this helper is only reached after the plugin opted in.
//
// Memory cross-ref: interface_assertion_against_plugin_owned_value_fails_in_prod.
func ComponentRawFromProvisionRequest(req *scpluginv1.ProvisionRequest) (map[string]any, error) {
	raw := req.GetComponentRaw()
	if len(raw) == 0 {
		return nil, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, errors.Wrap(err, "decode ComponentRaw from provision request")
	}
	return decoded, nil
}

// ResourceInputForComputeProcess builds the api.ResourceInput a plugin
// compute-process handler hands to the driver. Critically, it swaps the
// child environment for the parent's when ParentInfo.ParentEnv is non-empty:
// compute processors read their parent's exports from the parent stack root,
// and those export keys use the parent's env suffix
// (`forge-storage--staging--bucket-name`), not the child's.
//
// input.ToResName (api/pkg/api/server.go) prefers StackParams.ParentEnv over
// StackParams.Environment, so pre-swapping Environment here lets the compute
// processor's own code stay env-agnostic.
//
// Live-caught 2026-11-01 on forge-storage isolated deploy run 41290064279:
// client env `108b3f` built lookup key `forge-storage--108b3f--bucket-name`
// which did not exist in the parent (parent exports under `--staging`).
// Fixed in sc-plugin-aws v2026.11.01-92f5d1b; this is the extracted shape.
func ResourceInputForComputeProcess(req *scpluginv1.ComputeProcessRequest, parent *pApi.ParentInfo, cfg any) api.ResourceInput {
	env := req.GetHeader().GetEnvironment()
	if parent != nil && parent.ParentEnv != "" {
		env = parent.ParentEnv
	}
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type:   req.GetResourceType(),
			Name:   req.GetResourceName(),
			Config: api.Config{Config: cfg},
		},
		StackParams: &api.StackParams{
			StackName:   req.GetHeader().GetStackName(),
			Environment: env,
		},
	}
}

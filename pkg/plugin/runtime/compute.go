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

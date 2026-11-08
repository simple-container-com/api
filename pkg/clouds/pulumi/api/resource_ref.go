// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"sort"

	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

// SiblingOutputs is implemented by an in-tree resource's output type to publish
// values a SIBLING resource may read through an `${resource:<name>.<key>}`
// reference (api.ResourceRef).
//
// An interface and not a map of outputs on api.ResourceOutput because the values
// are Pulumi Outputs, and api.ResourceOutput lives in pkg/api, which must not
// import the Pulumi SDK. It also has to be declared HERE rather than beside any
// one implementation: pkg/clouds/pulumi/k3s imports pkg/clouds/pulumi/kubernetes
// (for the export-name helpers), so the consumer cannot import the producer.
//
// Keys are the logical field names an operator writes, lowercase — `kubeconfig`,
// `endpoint` — not the stack-export names, which carry the resource name and the
// environment and are a different namespace.
type SiblingOutputs interface {
	// OutputValue returns one published value. The boolean is "this resource
	// publishes that key at all".
	OutputValue(key string) (sdk.StringOutput, bool)
	// PublishedOutputKeys lists every key this output publishes, for the error
	// message on a miss. The realistic failure is not a typo but a reference to
	// a value the resource type does not expose, and an operator cannot guess
	// the right key from "unknown output".
	PublishedOutputKeys() []string
}

// ResolveResourceRef reads the value an `${resource:...}` reference names out of
// the outputs of the resources provisioned before this one.
//
// `outs` is ProvisionParams.ResourceOutputs. A reference only finds its target if
// the referencing resource's config declares the dependency via
// api.WithParentDependencies — which, for a kubeconfig, k8s.KubernetesConfig does
// automatically from the reference itself, so "not provisioned before this one"
// here means the resource is genuinely absent or misnamed.
//
// Two shapes of producer are supported, and the order matters only in that the
// typed one is cheaper:
//
//  1. an in-tree resource whose output implements SiblingOutputs (k3s),
//  2. a plugin-owned resource, whose published values core already harvested onto
//     api.PluginResourceOutput — so a managed cluster from a plugin is
//     referenceable with no extra plumbing, provided the plugin publishes the key.
//
// A value the plugin flagged secret is re-wrapped with sdk.ToSecret. Losing that
// flag on the way in is how a credential reaches a preview log in cleartext.
func ResolveResourceRef(ref api.ResourceRef, outs ResourcesOutputs) (sdk.StringOutput, error) {
	out, found := outs[ref.ResourceName]
	if !found || out == nil {
		return sdk.StringOutput{}, errors.Errorf(
			"%s names resource %q, which was not provisioned before this one. It must be a resource of the SAME "+
				"parent stack and environment%s",
			ref, ref.ResourceName, availableResourcesHint(outs),
		)
	}

	switch producer := out.Ref.(type) {
	case SiblingOutputs:
		value, ok := producer.OutputValue(ref.Key)
		if !ok {
			return sdk.StringOutput{}, errors.Errorf(
				"%s: resource %q publishes no output %q (it publishes: %v)",
				ref, ref.ResourceName, ref.Key, producer.PublishedOutputKeys(),
			)
		}
		return value, nil
	case *api.PluginResourceOutput:
		// StringOf covers both misses — key not published, and published as
		// something other than a string — and its error already lists the keys
		// that WERE published.
		str, err := producer.StringOf(ref.Key)
		if err != nil {
			return sdk.StringOutput{}, errors.Wrapf(err, "%s", ref)
		}
		value, _ := producer.Lookup(ref.Key) // present: StringOf just succeeded
		if value.Secret {
			return sdk.ToSecret(sdk.String(str)).(sdk.StringOutput), nil
		}
		return sdk.String(str).ToStringOutput(), nil
	default:
		return sdk.StringOutput{}, errors.Errorf(
			"%s: resource %q publishes no outputs a sibling can read (its output is %T)",
			ref, ref.ResourceName, out.Ref,
		)
	}
}

// availableResourcesHint names what WAS provisioned before this resource, sorted.
//
// Without it the operator is told their reference missed and nothing about what
// it could have hit — and the single most likely cause is a resource that is in a
// different environment's block of the same server.yaml, which looks adjacent in
// the file and is invisible here.
func availableResourcesHint(outs ResourcesOutputs) string {
	if len(outs) == 0 {
		return " (no sibling resource had been provisioned at that point)"
	}
	names := make([]string, 0, len(outs))
	for name := range outs {
		names = append(names, name)
	}
	sort.Strings(names)
	return " — provisioned before this one: " + joinQuoted(names)
}

func joinQuoted(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += "\"" + n + "\""
	}
	return out
}

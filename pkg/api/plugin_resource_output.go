// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"
)

// PluginOutputValue is one value a plugin published from its ProvisionResponse,
// carrying the plugin's own `secret` flag alongside it.
//
// The flag is not decoration. A consumer that renders a value into a config file,
// a command line or a Pulumi input has to know whether to wrap it in
// `sdk.ToSecret` first: an HMAC secret key or a database password that arrives
// here unflagged lands in preview and apply logs in cleartext, and in the state
// bucket readable by anything that can read state. The flag is set by the plugin
// on InputValue.secret (plugin.proto field 10) and is honoured both on the parent
// stack root's exports and here, on the in-memory sibling path.
type PluginOutputValue struct {
	Value  any
	Secret bool
}

// PluginResourceOutput is what a plugin-owned parent-stack resource leaves on
// api.ResourceOutput.Ref, so that a SIBLING resource provisioned after it can read
// what it produced.
//
// Why this type exists: resources that declare WithParentDependencies are
// provisioned after their dependencies and receive the accumulated
// `map[string]*ResourceOutput` (see pkg/clouds/pulumi/provision.go). An in-tree
// resource puts its typed output on `.Ref` and the consumer type-asserts it —
// that is how a k3s cluster learns its machines' addresses. A plugin-owned
// resource used to put the bare string `plugin:<name>:<urn>` there, so a sibling
// learned the URN and nothing else, even though core had just harvested every
// value the plugin published. Post plugin-extraction most resources are
// plugin-owned — buckets, databases, registries — which is exactly the set a new
// sibling dependency wants to read.
//
// String() still renders `plugin:<name>:<urn>` so anything that logged or
// formatted the old placeholder reads identically. Values are deliberately NOT
// part of that rendering: some of them are secret.
type PluginResourceOutput struct {
	// PluginName is the plugin that owns the resource (e.g. `gcp`).
	PluginName string
	// Urn is the Pulumi URN the plugin registered the resource under.
	Urn string
	// Outputs is what the plugin published, keyed exactly as the plugin exported
	// it — the same keys that appear on the parent stack root. Values a plugin
	// sent in a shape core cannot represent as a plain Go value are absent rather
	// than zeroed.
	Outputs map[string]PluginOutputValue
}

// String renders the resource the way the pre-typed placeholder did.
//
// Nil-safe because PluginOutputsOf's error paths hand back a nil output alongside
// the error, and a caller that logs and carries on would otherwise panic inside the
// logging rather than at the mistake.
func (o *PluginResourceOutput) String() string {
	if o == nil {
		return "plugin:<none>:<none>"
	}
	return fmt.Sprintf("plugin:%s:%s", o.PluginName, o.Urn)
}

// Lookup returns one published value and whether the plugin published it at all.
func (o *PluginResourceOutput) Lookup(key string) (PluginOutputValue, bool) {
	if o == nil {
		return PluginOutputValue{}, false
	}
	v, found := o.Outputs[key]
	return v, found
}

// StringOf reads one published value as a string.
//
// The error lists the keys that WERE published, because the realistic failure is
// not a typo: it is a plugin version that predates the key being asked for, or a
// resource whose export names are keyed off its own name and so differ per
// declaration. Without the list, the operator sees "no output X" and has no way
// to find out what to ask for instead.
func (o *PluginResourceOutput) StringOf(key string) (string, error) {
	v, found := o.Lookup(key)
	if !found {
		return "", errors.Errorf("%s published no output %q (it published: %s)",
			o.String(), key, o.publishedKeys())
	}
	s, ok := v.Value.(string)
	if !ok {
		return "", errors.Errorf("%s published output %q as %T, not a string", o.String(), key, v.Value)
	}
	return s, nil
}

// publishedKeys is the sorted key list used in error messages. Sorted because an
// error that reorders itself between runs is an error nobody can diff.
func (o *PluginResourceOutput) publishedKeys() string {
	if o == nil || len(o.Outputs) == 0 {
		return "nothing"
	}
	keys := make([]string, 0, len(o.Outputs))
	for k := range o.Outputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprintf("%v", keys)
}

// PluginOutputsOf resolves a plugin-owned sibling's outputs out of the outputs of
// the resources provisioned before this one.
//
// `outs` is ProvisionParams.ResourceOutputs. A consumer only receives a sibling in
// there if its own config implements WithParentDependencies and names that
// resource — so "not found" almost always means the dependency was never declared,
// and the error says so rather than reporting an absence.
func PluginOutputsOf(outs map[string]*ResourceOutput, resourceName string) (*PluginResourceOutput, error) {
	out, found := outs[resourceName]
	if !found || out == nil {
		return nil, errors.Errorf(
			"resource %q was not provisioned before this one. It must be a resource of the SAME parent stack "+
				"and environment, and this resource's config must name it in DependsOnResources()",
			resourceName,
		)
	}
	pluginOut, ok := out.Ref.(*PluginResourceOutput)
	if !ok {
		return nil, errors.Errorf(
			"resource %q is not provisioned by a plugin (its output is %T), so it publishes no plugin outputs",
			resourceName, out.Ref,
		)
	}
	return pluginOut, nil
}

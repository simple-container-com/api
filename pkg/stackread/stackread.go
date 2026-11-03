// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package stackread is the two-pass reader that discovers plugin-owned
// types BEFORE server.yaml's full parse runs.
//
// The pre-split flow was: SC core imports each plugin's /api module at
// Go-link time, those init()s populate pkg/api.providerConfigMapping,
// and pkg/api.ReadServerConfigs resolves every `type: X` against that
// static map. Adding a new type required rebuilding + releasing SC CLI,
// bumping the forge pin, and operator-visible SIMPLE_CONTAINER_VERSION.
//
// This package replaces api.ReadServerDescriptor as the entry point.
// It unmarshals the raw server.yaml to grab the plugins: block, then
// for each declared PluginSpec it loads the plugin's SchemaResponse —
// cached under ~/.sc/plugins/<repo>/<version>/.schema-cache.json so
// repeat parses skip the handshake — and calls
// api.RegisterPluginOwnedType for every type the Schema advertises.
// api.ReadServerConfigs then runs with full type knowledge; the three
// `strings.Count(X, ":") >= 2` syntactic escape hatches in read.go
// become authoritative Schema-driven ownership checks.
//
// This package lives OUTSIDE pkg/api precisely because pkg/plugin
// already imports pkg/api (fetcher.go, runtime/*). Putting the
// pre-parse discovery in pkg/api would create an import cycle; putting
// it in pkg/plugin would widen pkg/plugin's blast radius. The leaf
// package pattern keeps both intact.
package stackread

import (
	"context"
	"os"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// ReadServerDescriptor is the plugin-aware replacement for
// api.ReadServerDescriptor. Call-sites previously using the pkg/api
// entry point should switch to this one; the pkg/api entry stays as a
// no-plugins-aware fallback for callers that do not want the pre-parse
// discovery cost.
//
// Order of operations:
//  1. Raw YAML unmarshal of the server.yaml file (api.ReadDescriptor)
//     — gives us descriptor.Plugins without yet validating types.
//  2. DiscoverPluginTypes walks descriptor.Plugins; for each spec it
//     tries the per-version Schema cache first, falls back to spawning
//     the plugin + calling Schema + caching the result. Every type
//     advertised by any plugin gets a RegisterPluginOwnedType entry so
//     providerConfigMapping has the opaque decoder for the full-parse.
//  3. api.ReadServerConfigs — full parse-time validation. Types no
//     longer need a Go-link-time entry.
func ReadServerDescriptor(ctx context.Context, path string) (*api.ServerDescriptor, error) {
	descriptor, err := api.ReadDescriptor(path, &api.ServerDescriptor{})
	if err != nil {
		return descriptor, errors.Wrapf(err, "failed to read server descriptor from %q", path)
	}
	if err := descriptor.ValidateSchemaVersion(); err != nil {
		return descriptor, errors.Wrapf(err, "server descriptor %q", path)
	}
	if err := DiscoverPluginTypes(ctx, descriptor); err != nil {
		return descriptor, errors.Wrapf(err, "failed to discover plugin types from %q", path)
	}
	res, err := api.ReadServerConfigs(descriptor)
	if err != nil {
		return descriptor, errors.Wrapf(err, "failed to read server configs for %s", path)
	}
	return res, nil
}

// DiscoverPluginTypes walks descriptor.Plugins and installs an opaque
// decoder for every type any declared plugin advertises via Schema.
// Idempotent — calling twice is harmless, each type is registered
// against the SAME opaque decoder (api.RegisterPluginOwnedType is a
// map upsert plus a guarded providerConfigMapping entry).
//
// A plugin whose Schema cannot be loaded (binary missing, download
// failed, Schema RPC errored) fails the parse immediately with a
// pointer at the plugin name — the operator can then fix the
// `plugins:` block instead of debugging a downstream "unknown template
// type" that lies about the real cause.
func DiscoverPluginTypes(ctx context.Context, descriptor *api.ServerDescriptor) error {
	if descriptor == nil || len(descriptor.Plugins) == 0 {
		return nil
	}
	for name, spec := range descriptor.Plugins {
		schema, err := loadSchema(ctx, name, spec)
		if err != nil {
			return errors.Wrapf(err, "load schema for plugin %q", name)
		}
		registerSchemaTypes(name, schema)
		// Record the spec so convert.go's A2 dialler can re-spawn by
		// name when PrepareStaticForDeploy / PrepareCloudComposeForDeploy
		// hits a plugin-owned template type. Keyed by the server.yaml
		// plugin name (not the SchemaResponse's plugin_version string).
		recordSpecForConverter(name, spec)
	}
	return nil
}

// loadSchema returns the plugin's SchemaResponse, preferring the
// per-version cache file. On miss, spawns the plugin, calls Schema,
// writes the cache (best-effort), and closes the client. The pre-parse
// does not keep the plugin alive — the preengine respawns its own
// clients during ProvisionStack.
func loadSchema(ctx context.Context, name string, spec api.PluginSpec) (*scpluginv1.SchemaResponse, error) {
	if cached, ok := scPlugin.CachedSchema(spec, scPlugin.DefaultCacheRoot); ok {
		return cached, nil
	}
	cli, schema, err := scPlugin.ResolveAndSchema(ctx, name, spec, scPlugin.DefaultCacheRoot, nil, nil)
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	if werr := scPlugin.WriteCachedSchema(spec, scPlugin.DefaultCacheRoot, schema); werr != nil {
		// Best-effort — a cache write failure must not fail the parse.
		// The operator sees it as a one-time stderr line and the next
		// parse re-fetches.
		_, _ = os.Stderr.WriteString("stackread: cache schema for " + name + ": " + werr.Error() + "\n")
	}
	return schema, nil
}

// registerSchemaTypes maps every type-token-carrying field of a
// SchemaResponse to the matching PluginOwnedKind. Keep in sync with
// scpluginv1.SchemaResponse's shape — a new type kind means a new
// branch here and a new PluginOwnedKind constant in pkg/api.
func registerSchemaTypes(pluginName string, schema *scpluginv1.SchemaResponse) {
	if schema == nil {
		return
	}
	// Templates and resources both live under ResourceTypes today (per
	// plugin.proto §SchemaResponse.resource_types — plugins fold both
	// in). The parse-time distinction is drawn by WHERE server.yaml
	// uses the type (templates: vs resources: section), not by Schema.
	// Register all as Resource; the client.go template-detect branches
	// in A2 will refine via usage context.
	for _, t := range schema.GetResourceTypes() {
		api.RegisterPluginOwnedType(t, api.PluginOwnedKindResource, pluginName)
	}
	for _, t := range schema.GetRegistrarTypes() {
		api.RegisterPluginOwnedType(t, api.PluginOwnedKindRegistrar, pluginName)
	}
	for _, t := range schema.GetStateStoreTypes() {
		api.RegisterPluginOwnedType(t, api.PluginOwnedKindStateStore, pluginName)
	}
	for _, t := range schema.GetSecretsProviderTypes() {
		api.RegisterPluginOwnedType(t, api.PluginOwnedKindSecretsProvider, pluginName)
	}
	for _, t := range schema.GetComputeProcessorTypes() {
		api.RegisterPluginOwnedType(t, api.PluginOwnedKindComputeProcessor, pluginName)
	}
	for _, driver := range schema.GetVmDrivers() {
		if driver == nil {
			continue
		}
		api.RegisterPluginOwnedType(driver.GetDriver(), api.PluginOwnedKindVmDriver, pluginName)
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// ResolveAndSchema fetches the plugin binary (or verifies the path:
// pointer), spawns the plugin process, and calls Schema. The returned
// Client is live — the caller OWNS it: start its LogStream if desired,
// register it in any dispatch map, and Close() it when done.
//
// Previously inline in pkg/clouds/pulumi/plugin_preengine.go:loadOne.
// Factored here so BOTH the preengine (at ProvisionStack time) and the
// pre-parse type-discovery pass (pkg/stackread, at parse time) use the
// same resolve → verify → spawn → Schema sequence. One source of truth
// for the ordering + the hclog.Debug level choice that lets plugin
// panic traces propagate (live-caught 2026-10-30, forge-baas).
//
// For the pre-parse pass that only needs type names, callers should
// prefer CachedSchema first and only fall through to ResolveAndSchema
// on cache miss — spawning a plugin per parse costs a 200-400 ms
// handshake per plugin that CachedSchema sidesteps.
func ResolveAndSchema(
	ctx context.Context,
	name string,
	spec api.PluginSpec,
	cacheRoot string,
	logger hclog.Logger,
	peer PeerDispatcher,
) (cli *Client, schema *scpluginv1.SchemaResponse, err error) {
	resolvedPath, err := ResolveBinaryPath(spec, cacheRoot)
	if err != nil {
		return nil, nil, errors.Wrap(err, "resolve plugin binary")
	}
	spec.Path = resolvedPath
	if err := VerifySHA256(spec); err != nil {
		return nil, nil, err
	}
	if logger == nil {
		// Mirror the preengine's choice: Debug surfaces plugin panic
		// traces via hashicorp/go-plugin's stderr-at-Debug logging
		// (client.go:1186). A Warn-level logger discards every byte
		// and turns panics into `exit status 2` with no further
		// diagnostics. See loadOne comment.
		logger = hclog.New(&hclog.LoggerOptions{
			Name:   "sc-plugin-" + name,
			Level:  hclog.Debug,
			Output: os.Stderr,
		})
	}
	cli, err = NewClient(ClientConfig{
		Path:    resolvedPath,
		Managed: true,
		Logger:  logger,
		Peer:    peer,
	})
	if err != nil {
		return nil, nil, err
	}
	schema, err = cli.Schema(ctx, &scpluginv1.RequestHeader{})
	if err != nil {
		cli.Close()
		return nil, nil, errors.Wrap(err, "schema")
	}
	return cli, schema, nil
}

// schemaCacheFileName is the per-plugin-version cache artefact next to
// the cached binary. Only populated for github-release specs; path:
// specs skip the cache (the binary location is dev-driven and not
// content-addressed by version).
const schemaCacheFileName = ".schema-cache.json"

func schemaCachePath(spec api.PluginSpec, cacheRoot string) (string, bool) {
	if spec.Source == nil || spec.Source.Repository == "" || spec.Source.Version == "" {
		return "", false
	}
	root, err := expandHome(cacheRoot)
	if err != nil {
		return "", false
	}
	return filepath.Join(root, spec.Source.Repository, spec.Source.Version, schemaCacheFileName), true
}

// CachedSchema returns the schema recorded for (repository, version) by
// a previous WriteCachedSchema call, or (nil, false) if the cache has
// no entry (fresh install, path: spec, cache cleared, cache file
// corrupt). A hit means the pre-parse can skip the plugin spawn
// entirely.
func CachedSchema(spec api.PluginSpec, cacheRoot string) (*scpluginv1.SchemaResponse, bool) {
	cachePath, ok := schemaCachePath(spec, cacheRoot)
	if !ok {
		return nil, false
	}
	payload, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}
	// Stored as a JSON-wrapped protobuf wire blob so a future schema
	// field addition doesn't invalidate every operator's cache — proto
	// is forward-compatible, JSON wrapping lets us add a schema-cache
	// version tag later without migration.
	var env schemaCacheEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, false
	}
	if env.Version != schemaCacheEnvelopeVersion {
		return nil, false
	}
	schema := &scpluginv1.SchemaResponse{}
	if err := proto.Unmarshal(env.Payload, schema); err != nil {
		return nil, false
	}
	return schema, true
}

// WriteCachedSchema persists schema for (repository, version). Best-effort
// — a write failure logs and returns nil, so the next parse just re-fetches
// instead of failing the deploy.
func WriteCachedSchema(spec api.PluginSpec, cacheRoot string, schema *scpluginv1.SchemaResponse) error {
	cachePath, ok := schemaCachePath(spec, cacheRoot)
	if !ok {
		return nil
	}
	payload, err := proto.Marshal(schema)
	if err != nil {
		return errors.Wrap(err, "marshal schema for cache")
	}
	env := schemaCacheEnvelope{
		Version: schemaCacheEnvelopeVersion,
		Payload: payload,
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return errors.Wrap(err, "wrap schema cache envelope")
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return errors.Wrap(err, "mkdir schema cache dir")
	}
	return os.WriteFile(cachePath, raw, 0o644)
}

const schemaCacheEnvelopeVersion = 1

type schemaCacheEnvelope struct {
	Version int    `json:"v"`
	Payload []byte `json:"schema"`
}

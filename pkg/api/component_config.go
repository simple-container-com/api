// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"regexp"

	"github.com/pkg/errors"
)

// ComponentConfig wraps *PluginFieldConfig with typed accessors that read
// its `.Raw` map, for the SC Components class of plugins (slice 1a of the
// sc-components-platform milestone).
//
// Why a .Raw-reading wrapper rather than a Go interface satisfied by the
// plugin's typed config struct: the production `sc` binary reaches plugins
// exclusively via hashicorp/go-plugin gRPC and does NOT link plugin `/api`
// modules. A plugin-owned config value arrives at core as opaque
// *PluginFieldConfig, carrying the operator's YAML in `.Raw`. A Go
// interface defined in core cannot be satisfied by a type living in a
// plugin repo — the assertion is structurally guaranteed to fail in prod,
// even when a local test (which links the plugin's `/api`) shows it
// passing. 7+ exposures of that trap in the first fortnight of the
// plugin-split work; see the operator memory
// `interface_assertion_against_plugin_owned_value_fails_in_prod`
// + the parent `local_shape_not_production_shape`.
//
// Full design: forge/docs/design/2026-11-10/sc-components-platform/ §P1
// (Path A — Schema-advertised + .Raw-reading).
type ComponentConfig struct {
	raw *PluginFieldConfig
}

// secretRefPattern matches the `${secret:<alias>}` shape SC core resolves
// at config-read time. Permissive enough to allow any alias characters
// SC itself accepts.
var secretRefPattern = regexp.MustCompile(`^\$\{secret:[A-Za-z0-9._-]+\}$`)

// NewComponentConfig wraps cfg as a ComponentConfig.
//
// Accepted inputs:
//
//   - *PluginFieldConfig — the production path. `cfg.Raw` is the source of
//     truth for all accessor reads.
//   - map[string]any — the test path. Lifts the map into a synthetic
//     *PluginFieldConfig so test fixtures can skip the plugin-bus round
//     trip.
//   - nil — returns a typed error rather than a nil-wrapped zero value.
//
// Any other concrete type is a programmer error and returns an error
// naming the actual Go type.
func NewComponentConfig(cfg any) (*ComponentConfig, error) {
	if cfg == nil {
		return nil, errors.New("component config: cfg is nil")
	}
	switch c := cfg.(type) {
	case *ComponentConfig:
		return c, nil
	case *PluginFieldConfig:
		if c == nil {
			return nil, errors.New("component config: *PluginFieldConfig is nil")
		}
		return &ComponentConfig{raw: c}, nil
	case map[string]any:
		return &ComponentConfig{raw: &PluginFieldConfig{Raw: c}}, nil
	default:
		return nil, errors.Errorf("component config: unexpected type %T, want *api.PluginFieldConfig or map[string]any", cfg)
	}
}

// Raw returns the underlying `.Raw` map. Callers must treat it as
// read-only; use RawCopy for a defensive copy.
func (c *ComponentConfig) Raw() map[string]any {
	if c == nil || c.raw == nil {
		return nil
	}
	return c.raw.Raw
}

// RawCopy returns a shallow defensive copy of the underlying `.Raw` map
// so callers (e.g. the sidecar's merge logic) can mutate without
// disturbing the wrapped config.
func (c *ComponentConfig) RawCopy() map[string]any {
	src := c.Raw()
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// DeployTo reads `.Raw["deployTo"]` as a non-empty string.
func (c *ComponentConfig) DeployTo() (string, error) {
	return c.readString("deployTo", true /*required*/)
}

// Image reads `.Raw["image"]` as a non-empty string.
func (c *ComponentConfig) Image() (string, error) {
	return c.readString("image", true /*required*/)
}

// SigningKeyRef reads `.Raw["signingKey"]`, validates it matches the
// `${secret:...}` reference shape. Absent key returns "" + nil (a
// component without a signingKey is a valid shape; the plugin's
// ComputeProcess will refuse to mint JWTs without it, but that's a
// runtime concern).
func (c *ComponentConfig) SigningKeyRef() (string, error) {
	v, present, err := c.readOptionalString("signingKey")
	if err != nil {
		return "", err
	}
	if !present {
		return "", nil
	}
	if !secretRefPattern.MatchString(v) {
		return "", errors.Errorf("component config: signingKey %q is not a ${secret:...} reference", v)
	}
	return v, nil
}

// ServiceEnv reads `.Raw["env"]` as a map[string]string. Absent key
// returns an empty map + nil. Values are coerced to string from
// map[string]any; non-string values are a shape error.
func (c *ComponentConfig) ServiceEnv() (map[string]string, error) {
	raw := c.Raw()
	if raw == nil {
		return map[string]string{}, nil
	}
	v, ok := raw["env"]
	if !ok || v == nil {
		return map[string]string{}, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.Errorf("component config: env is %T, want map[string]any", v)
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			return nil, errors.Errorf("component config: env[%q] is %T, want string", k, val)
		}
		out[k] = s
	}
	return out, nil
}

// IngressHostname reads `.Raw["ingress"].(map)["hostname"]` as a string.
// Absent hostname (or absent ingress block) returns "" + nil.
func (c *ComponentConfig) IngressHostname() (string, error) {
	return c.readIngressField("hostname")
}

// IngressRegistrar reads `.Raw["ingress"].(map)["registrar"]` as a string.
// Absent returns "" + nil (component defaults to the parent's default
// registrar).
func (c *ComponentConfig) IngressRegistrar() (string, error) {
	return c.readIngressField("registrar")
}

func (c *ComponentConfig) readIngressField(name string) (string, error) {
	raw := c.Raw()
	if raw == nil {
		return "", nil
	}
	ing, ok := raw["ingress"]
	if !ok || ing == nil {
		return "", nil
	}
	m, ok := ing.(map[string]any)
	if !ok {
		return "", errors.Errorf("component config: ingress is %T, want map[string]any", ing)
	}
	v, ok := m[name]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.Errorf("component config: ingress.%s is %T, want string", name, v)
	}
	return s, nil
}

func (c *ComponentConfig) readString(key string, required bool) (string, error) {
	raw := c.Raw()
	if raw == nil {
		if required {
			return "", errors.Errorf("component config: %s required, config is empty", key)
		}
		return "", nil
	}
	v, ok := raw[key]
	if !ok || v == nil {
		if required {
			return "", errors.Errorf("component config: %s is required", key)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.Errorf("component config: %s is %T, want string", key, v)
	}
	if required && s == "" {
		return "", errors.Errorf("component config: %s is empty", key)
	}
	return s, nil
}

func (c *ComponentConfig) readOptionalString(key string) (value string, present bool, err error) {
	raw := c.Raw()
	if raw == nil {
		return "", false, nil
	}
	v, ok := raw[key]
	if !ok || v == nil {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", true, errors.Errorf("component config: %s is %T, want string", key, v)
	}
	return s, true, nil
}

// ComponentConfigTopLevelKeys lists the keys the Component class treats as
// well-known on the resource's config. Any additional top-level key an
// operator or plugin tries to set that is NOT also listed in the per-type
// allowlist is a parse-time error (plugin_requirements.go).
//
// Kept as a var (not const slice) so test fixtures and future components
// can extend it if they need to. Changes here are plugin-facing-contract
// changes — review against the design doc before touching.
var ComponentConfigTopLevelKeys = []string{
	"deployTo",
	"image",
	"signingKey",
	"env",
	"ingress",
}

// IsComponentConfigTopLevelKey reports whether the given key is one of the
// well-known Component config keys above.
func IsComponentConfigTopLevelKey(key string) bool {
	for _, k := range ComponentConfigTopLevelKeys {
		if k == key {
			return true
		}
	}
	return false
}

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

// applyComponentSecretEnvMap rewrites `${secret:...}` references on the
// Component's top-level fields into entries on `merged["secrets"]` — a
// generic replacement for a template plugin's per-field hardcoded
// signingKey → cfg.Secrets["SERVICE_JWT_SECRET"] block. Called from
// `dispatchComponentTemplate` after defaults + placeholder resolution
// and before mergeComponentIntoTemplate.
//
// Mapping source: the Component's owning plugin's advertised
// `SchemaResponse.component_secret_env_map` — pApi.ComponentSecretEnvMap
// at this point in the deploy. Empty map for a pre-field plugin binary
// → helper is a no-op, operator's existing `secrets:` stays untouched,
// and any template-side hardcoded fallback keeps working unchanged.
//
// Operator wins: an operator-written `secrets:` entry (if the plugin's
// allowlist permits it) is preserved against the plugin's declared
// mapping. Collisions between fields are first-write-wins at the ENV
// name (map iteration order is unstable; a plugin that declares two
// fields mapping to the SAME env is a config bug).
//
// Nil-ok for both arguments: unknown resource type OR nil merged map
// returns without panicking.
func applyComponentSecretEnvMap(resourceType string, merged map[string]any) {
	if merged == nil {
		return
	}
	secretEnvMap := pApi.GetComponentSecretEnvMap(resourceType)
	if len(secretEnvMap) == 0 {
		return
	}
	secrets, _ := merged["secrets"].(map[string]any)
	if secrets == nil {
		secrets = map[string]any{}
	}
	for field, envVar := range secretEnvMap {
		raw, ok := merged[field]
		if !ok || raw == nil {
			continue
		}
		s, ok := raw.(string)
		if !ok || s == "" {
			continue
		}
		if _, present := secrets[envVar]; present {
			continue
		}
		secrets[envVar] = s
	}
	if len(secrets) > 0 {
		merged["secrets"] = secrets
	}
}

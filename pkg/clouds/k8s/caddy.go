// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeCaddy = "kubernetes-caddy"

type CaddyResource struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	*CaddyConfig      `json:",inline" yaml:",inline"`
	// UseResourceName names every object of this Caddy <resource name>-<env> instead of
	// caddy-<env>, so a second Caddy fits in an environment that already has one. Switching
	// it on an existing Caddy renames it, which makes Pulumi replace it.
	UseResourceName bool `json:"useResourceName,omitempty" yaml:"useResourceName,omitempty"`
}

func CaddyReadConfig(config *api.Config) (api.Config, error) {
	cfg, err := api.ConvertConfig(config, &CaddyResource{})
	if err != nil {
		return cfg, err
	}
	// Normalize empty slices to nil — yaml.Unmarshal into inline pointer
	// structs can produce []string{} instead of nil for absent fields.
	if res, ok := cfg.Config.(*CaddyResource); ok && res.CaddyConfig != nil {
		if len(res.CaddyConfig.TrustedProxies) == 0 {
			res.CaddyConfig.TrustedProxies = nil
		}
	}
	return cfg, nil
}

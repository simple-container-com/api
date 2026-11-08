// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const ResourceTypeCaddy = "kubernetes-caddy"

type CaddyResource struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	*CaddyConfig      `json:",inline" yaml:",inline"`
	// UseResourceName names every object of this Caddy <resource name>-<env> instead of
	// caddy-<env>, so a second Caddy fits in an environment that already has one.
	//
	// Switching it on an existing Caddy renames it, which makes Pulumi REPLACE it — and
	// that is not confined to the parent stack. The replacement swaps the Caddy's
	// LoadBalancer Service, so the parent exports a new ingress IP, while the A records
	// pointing at the old one were written by the CHILD stacks and live in their state:
	// they are rewritten only on the next child deploy. Flipping this on a live Caddy
	// therefore blackholes every domain it serves until each child stack is redeployed.
	//
	// A second Caddy on the SAME cluster also needs its own namespace: — this flag renames
	// the objects, not the namespace they live in (it defaults to "caddy" either way).
	UseResourceName bool `json:"useResourceName,omitempty" yaml:"useResourceName,omitempty"`
}

func CaddyReadConfig(config *api.Config) (api.Config, error) {
	cfg, err := api.ConvertConfig(config, &CaddyResource{})
	if err != nil {
		return cfg, err
	}
	res, ok := cfg.Config.(*CaddyResource)
	if !ok {
		return cfg, nil
	}
	// Normalize empty slices to nil — yaml.Unmarshal into inline pointer
	// structs can produce []string{} instead of nil for absent fields.
	if res.CaddyConfig != nil {
		if len(res.CaddyConfig.TrustedProxies) == 0 {
			res.CaddyConfig.TrustedProxies = nil
		}
	}
	// A Caddy edge is the first and most common consumer of the
	// `${resource:<cluster>.kubeconfig}` reference, and it does NOT go through
	// ReadKubernetesConfig — this type has its own reader. Refuse a malformed
	// reference here too, rather than letting it reach the provider as a string
	// that merely fails to connect.
	if _, _, err := res.KubernetesConfig.KubeconfigRef(); err != nil {
		return cfg, errors.Wrap(err, "invalid kubeconfig")
	}
	return cfg, nil
}

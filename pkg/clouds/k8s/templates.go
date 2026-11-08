// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/docker"
)

const (
	TemplateTypeKubernetesCloudrun = "kubernetes-cloudrun"
)

type CloudrunTemplate struct {
	KubernetesConfig           `json:",inline" yaml:",inline"`
	docker.RegistryCredentials `json:",inline" yaml:",inline"`
	// TlsConfig carries `clientTls` / `originTls` and the deprecated `useSSL`.
	// Inline, so `useSSL` keeps its place in an existing server.yaml and
	// `templateCfg.UseSSL` keeps compiling through field promotion.
	TlsConfig     `json:",inline" yaml:",inline"`
	CaddyResource *string `json:"caddyResource,omitempty" yaml:"caddyResource,omitempty"` // name of the caddy resource in base stack
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &CloudrunTemplate{})
}

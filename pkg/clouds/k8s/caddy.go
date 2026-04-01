package k8s

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeCaddy = "kubernetes-caddy"

type CaddyResource struct {
	*KubernetesConfig `json:",inline" yaml:",inline"`
	*CaddyConfig      `json:",inline" yaml:",inline"`
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

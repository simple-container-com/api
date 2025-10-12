package github

import "github.com/simple-container-com/api/pkg/api"

const CiCdTypeGithubActions = "github-actions"

// Legacy ActionsCiCdConfig for backward compatibility
type ActionsCiCdConfig struct {
	AuthToken string `json:"auth-token" yaml:"auth-token"`
}

// ReadCiCdConfig reads CI/CD configuration, supporting both legacy and enhanced formats
func ReadCiCdConfig(config *api.Config) (api.Config, error) {
	// Try to convert to enhanced config first
	enhancedConfig := &EnhancedActionsCiCdConfig{}
	if convertedConfig, err := api.ConvertConfig(config, enhancedConfig); err == nil {
		// If enhanced config conversion succeeds, set defaults and return
		enhancedConfig.SetDefaults()
		return convertedConfig, nil
	}

	// Fall back to legacy config for backward compatibility
	return api.ConvertConfig(config, &ActionsCiCdConfig{})
}

// ReadEnhancedCiCdConfig specifically reads enhanced CI/CD configuration
func ReadEnhancedCiCdConfig(config *api.Config) (api.Config, error) {
	enhancedConfig := &EnhancedActionsCiCdConfig{}
	convertedConfig, err := api.ConvertConfig(config, enhancedConfig)
	if err != nil {
		return api.Config{}, err
	}

	enhancedConfig.SetDefaults()
	return convertedConfig, nil
}

func (r *ActionsCiCdConfig) CredentialsValue() string {
	return r.AuthToken
}

func (r *ActionsCiCdConfig) ProjectIdValue() string {
	return "" // todo: figure out
}

func (r *ActionsCiCdConfig) ProviderType() string {
	return ProviderType
}

// Enhanced config also implements the same interface
func (r *EnhancedActionsCiCdConfig) CredentialsValue() string {
	return r.AuthToken
}

func (r *EnhancedActionsCiCdConfig) ProjectIdValue() string {
	return r.Organization.Name
}

func (r *EnhancedActionsCiCdConfig) ProviderType() string {
	return ProviderType
}

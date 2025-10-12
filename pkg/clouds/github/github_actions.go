package github

import "github.com/simple-container-com/api/pkg/api"

const CiCdTypeGithubActions = "github-actions"

// Legacy ActionsCiCdConfig for backward compatibility
type ActionsCiCdConfig struct {
	AuthToken string `json:"auth-token" yaml:"auth-token"`
}

// ReadCiCdConfig reads CI/CD configuration using SC's standard pattern
func ReadCiCdConfig(config *api.Config) (api.Config, error) {
	// Try to convert to strongly typed GitHub Actions configuration first
	convertedConfig, err := api.ConvertConfig(config, &GitHubActionsCiCdConfig{})
	if err != nil {
		// Fall back to legacy config for backward compatibility
		return api.ConvertConfig(config, &ActionsCiCdConfig{})
	}

	// Set defaults for any missing required fields
	if gitHubConfig, ok := convertedConfig.Config.(*GitHubActionsCiCdConfig); ok {
		if gitHubConfig.Organization == "" {
			gitHubConfig.Organization = "simple-container-org"
		}
		if len(gitHubConfig.Environments) == 0 {
			gitHubConfig.Environments = map[string]GitHubEnvironmentConfig{
				"staging":    {Type: "staging"},
				"production": {Type: "production"},
			}
		}
		if len(gitHubConfig.WorkflowGeneration.Templates) == 0 {
			gitHubConfig.WorkflowGeneration.Templates = []string{"deploy", "destroy"}
		}
		if gitHubConfig.WorkflowGeneration.CustomActions == nil {
			gitHubConfig.WorkflowGeneration.CustomActions = map[string]string{}
		}
	}

	return convertedConfig, nil
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

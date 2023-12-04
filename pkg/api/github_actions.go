package api

const CiCdTypeGithubActions = "github-actions"

type GithubActionsCiCdConfig struct {
	AuthToken string `json:"auth-token" yaml:"auth-token"`
}

func GithubACtionsReadCiCdConfig(config any) (any, error) {
	return ConvertDescriptor(config, &GithubActionsCiCdConfig{})
}

package api

import "github.com/pkg/errors"

const SecretsSchemaVersion = "1.0"

// SecretsDescriptor describes the secrets schema
type SecretsDescriptor struct {
	SchemaVersion string                    `json:"schemaVersion" yaml:"schemaVersion"`
	Auth          map[string]AuthDescriptor `json:"auth" yaml:"auth"`
	Values        map[string]string         `json:"values" yaml:"values"`
}

type AuthDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

func (a *AuthDescriptor) AuthValue() (string, error) {
	c, ok := a.Config.Config.(AuthConfig)
	if !ok {
		return "", errors.Errorf("auth config %q does not implement AuthConfig", a)
	}
	return c.AuthValue(), nil
}

package api

const SecretsSchemaVersion = "1.0"

// SecretsDescriptor describes the secrets schema
type SecretsDescriptor struct {
	SchemaVersion string                    `json:"schemaVersion" yaml:"schemaVersion"`
	Auth          map[string]AuthDescriptor `json:"auth" yaml:"auth"`
	Values        map[string]string         `json:"values" yaml:"values"`
}

type AuthDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  any    `json:"config" yaml:"config"`
	Inherit `json:",inline" yaml:",inline"`
}

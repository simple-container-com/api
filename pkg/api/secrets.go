package api

const SecretsSchemaVersion = "1.0"

// SecretsDescriptor describes the secrets schema
type SecretsDescriptor struct {
	SchemaVersion string                    `json:"schemaVersion"`
	Auth          map[string]AuthDescriptor `json:"auth"`
	Values        map[string]string         `json:"values"`
}

type AuthDescriptor struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

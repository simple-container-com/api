package api

const ServerSchemaVersion = "1.0"

// ServerDescriptor describes the server schema
type ServerDescriptor struct {
	SchemaVersion string                                 `json:"schemaVersion"`
	Provisioner   ProvisionerDescriptor                  `json:"provisioner"`
	Secrets       SecretsConfigDescriptor                `json:"secrets"`
	Templates     map[string]StackDescriptor             `json:"templates"`
	Resources     map[string]PerStackResourcesDescriptor `json:"resources"`
	Variables     map[string]VariableDescriptor          `json:"variables"`
}

type VariableDescriptor struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type PerStackResourcesDescriptor struct {
	Registrar *RegistrarDescriptor          `json:"registrar"`
	Inherit   *string                       `json:"inherit"`
	Resources map[string]ResourceDescriptor `json:"resources"`
}

type ResourceDescriptor struct {
	Type   string `json:"type"`
	Config any    `json:"config"`
}

type RegistrarDescriptor struct {
	Type    *string `json:"type"`
	Inherit *string `json:"inherit"`
}

type StackDescriptor struct {
	Type string `json:"type"`
}

type SecretsConfigDescriptor struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// ProvisionerDescriptor describes the provisioner schema
type ProvisionerDescriptor struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

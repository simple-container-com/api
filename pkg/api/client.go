package api

const ClientSchemaVersion = "1.0"

// ClientDescriptor describes the client schema
type ClientDescriptor struct {
	SchemaVersion string                           `json:"schemaVersion"`
	Stacks        map[string]StackClientDescriptor `json:"stacks"`
}

type StackClientDescriptor struct {
	Stack    string      `json:"stack"`
	Template string      `json:"template"`
	Domain   string      `json:"domain"`
	Config   StackConfig `json:"config"`
}

type StackConfig struct {
	DockerComposeFile string   `json:"docker-compose-file"`
	Uses              []string `json:"uses"`
	Runs              []string `json:"runs"`
}

package api

const ClientSchemaVersion = "1.0"

// ClientDescriptor describes the client schema
type ClientDescriptor struct {
	SchemaVersion string                           `json:"schemaVersion" yaml:"schemaVersion"`
	Stacks        map[string]StackClientDescriptor `json:"stacks" yaml:"stacks"`
}

type StackClientDescriptor struct {
	Stack       string      `json:"stack" yaml:"stack"`
	Environment string      `json:"environment" yaml:"environment"`
	Domain      string      `json:"domain" yaml:"domain"`
	Config      StackConfig `json:"config" yaml:"config"`
}

type StackConfig struct {
	DockerComposeFile string   `json:"docker-compose-file" yaml:"docker-compose-file"`
	Uses              []string `json:"uses" yaml:"uses"`
	Runs              []string `json:"runs" yaml:"runs"`
}

type DeployParams struct {
	RootDir     string         `json:"rootDir" yaml:"rootDir"`
	Profile     string         `json:"profile" yaml:"profile"`
	Stack       string         `json:"stack" yaml:"stack"`
	Environment string         `json:"environment" yaml:"environment"`
	Vars        VariableValues `json:"vars" yaml:"vars"`
}

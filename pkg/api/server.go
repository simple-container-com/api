package api

const ServerSchemaVersion = "1.0"

// ServerDescriptor describes the server schema
type ServerDescriptor struct {
	SchemaVersion string                        `json:"schemaVersion" yaml:"schemaVersion"`
	Provisioner   ProvisionerDescriptor         `json:"provisioner" yaml:"provisioner"`
	Secrets       SecretsConfigDescriptor       `json:"secrets" yaml:"secrets"`
	CiCd          CiCdDescriptor                `json:"cicd" json:"cicd"`
	Templates     map[string]StackDescriptor    `json:"templates" yaml:"templates"`
	Resources     PerStackResourcesDescriptor   `json:"resources" yaml:"resources"`
	Variables     map[string]VariableDescriptor `json:"variables" yaml:"variables"`
}

func (sd *ServerDescriptor) ValuesOnly() *ServerDescriptor {
	return &ServerDescriptor{
		SchemaVersion: sd.SchemaVersion,
		Provisioner:   sd.Provisioner.ValuesOnly(),
		Secrets:       sd.Secrets,
		CiCd:          sd.CiCd,
		Templates:     sd.Templates,
		Resources:     sd.Resources,
		Variables:     sd.Variables,
	}
}

type Inherit struct {
	Inherit string `json:"inherit" yaml:"inherit"`
}

type Config struct {
	Config any `json:"config" yaml:"config"`
}

func (i Inherit) IsInherited() bool {
	return i.Inherit != ""
}

type CiCdDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

type VariableDescriptor struct {
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

type PerStackResourcesDescriptor struct {
	Registrar RegistrarDescriptor                  `json:"registrar" yaml:"registrar"`
	Resources map[string]PerEnvResourcesDescriptor `json:"resources" yaml:"resources"`
}

type PerEnvResourcesDescriptor struct {
	Template  string                        `json:"template" yaml:"template"`
	Resources map[string]ResourceDescriptor `json:"resources" yaml:"resources"`
	Inherit   `json:",inline" yaml:",inline"`
}

type ResourceDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

type RegistrarDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

type StackDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

type SecretsConfigDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

// ProvisionerDescriptor describes the provisioner schema
type ProvisionerDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`

	provisioner Provisioner
}

func (pd *ProvisionerDescriptor) GetProvisioner() Provisioner {
	return pd.provisioner
}

func (pd *ProvisionerDescriptor) ValuesOnly() ProvisionerDescriptor {
	return ProvisionerDescriptor{
		Type:    pd.Type,
		Config:  pd.Config,
		Inherit: pd.Inherit,
	}
}

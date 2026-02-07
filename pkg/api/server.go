package api

import (
	"fmt"

	"github.com/samber/lo"
)

const ServerSchemaVersion = "1.0"

type ProvisionParams struct {
	StacksDir    string   `json:"rootDir" yaml:"rootDir"`
	Profile      string   `json:"profile" yaml:"profile"`
	Stacks       []string `json:"stacks" yaml:"stacks"`
	SkipRefresh  bool     `json:"skipRefresh" yaml:"skipRefresh"`
	SkipPreview  bool     `json:"skipPreview" yaml:"skipPreview"`
	DetailedDiff bool     `json:"detailedDiff" yaml:"detailedDiff"` // Enable detailed diff output for granular change visibility
	Timeouts     Timeouts `json:",inline" yaml:",inline"`
}

// ServerDescriptor describes the server schema
type ServerDescriptor struct {
	SchemaVersion string                        `json:"schemaVersion" yaml:"schemaVersion"`
	Provisioner   ProvisionerDescriptor         `json:"provisioner" yaml:"provisioner"`
	Secrets       SecretsConfigDescriptor       `json:"secrets" yaml:"secrets"`
	CiCd          CiCdDescriptor                `json:"cicd" yaml:"cicd"`
	Templates     map[string]StackDescriptor    `json:"templates" yaml:"templates"`
	Resources     PerStackResourcesDescriptor   `json:"resources" yaml:"resources"`
	Variables     map[string]VariableDescriptor `json:"variables" yaml:"variables"`
}

// ValuesOnly returns copy of descriptor without additional state (e.g. provisioner reference etc.)
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
	Name    string `json:"name" yaml:"name"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

type ResourceInput struct {
	Descriptor  *ResourceDescriptor `json:"descriptor" yaml:"descriptor"`
	StackParams *StackParams        `json:"deployParams" yaml:"deployParams"`
}

// ToResName adds environment suffix if environment is specified in the resource input
func (r *ResourceInput) ToResName(resName string) string {
	env := r.StackParams.Environment
	if r.StackParams.ParentEnv != "" { // if parentEnv is specified, we use it instead of stack's env
		env = r.StackParams.ParentEnv
	}
	return fmt.Sprintf("%s%s", resName,
		lo.If(env != "", "--"+env).Else(""))
}

type ResourceOutput struct {
	Ref any `json:"ref" yaml:"ref"`
}

type StackDescriptor struct {
	Type        string `json:"type" yaml:"type"`
	ParentStack string `json:"parentStack" yaml:"parentStack"`
	Config      `json:",inline" yaml:",inline"`
	Inherit     `json:",inline" yaml:",inline"`
}

type WithDependsOnResources interface {
	DependsOnResources() []StackConfigDependencyResource
}

type WithParentDependencies interface {
	DependsOnResources() []ParentResourceDependency
}

type ResourceAware interface {
	Uses() []string
}

type DnsConfigAware interface {
	OverriddenBaseZone() string
}

type CloudComposeDescriptor struct {
	StackName       string `json:"stackName" yaml:"stackName"`
	StackDescriptor `json:",inline" yaml:",inline"`
}

type SecretsConfigDescriptor struct {
	Type          string                    `json:"type" yaml:"type"`
	SecretsConfig *EnvironmentSecretsConfig `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
	Config        `json:",inline" yaml:",inline"`
	Inherit       `json:",inline" yaml:",inline"`
}

// EnvironmentSecretsConfig defines environment-specific secret filtering
type EnvironmentSecretsConfig struct {
	// Mode defines how secrets are filtered: include, exclude, or override
	Mode string `json:"mode" yaml:"mode"`

	// Secrets defines the secret mapping and filtering rules
	Secrets SecretsConfigMap `json:"secrets" yaml:"secrets"`

	// InheritAll when true includes all secrets except those explicitly excluded
	InheritAll bool `json:"inheritAll,omitempty" yaml:"inheritAll,omitempty"`
}

// SecretsConfigMap defines the mapping of secret references
type SecretsConfigMap map[string]SecretReference

// SecretReference defines how a secret is referenced
// Three patterns are supported:
// 1. Direct reference: "~" - use secret with same name from secrets.yaml
// 2. Mapped reference: "${secret:KEY}" - use secret named KEY from secrets.yaml
// 3. Literal value: Any other string - use the literal value directly
type SecretReference string

const (
	// SecretsConfigModeInclude filters secrets to only include those specified
	SecretsConfigModeInclude = "include"
	// SecretsConfigModeExclude filters secrets to exclude those specified
	SecretsConfigModeExclude = "exclude"
	// SecretsConfigModeOverride uses specified secrets and includes all others
	SecretsConfigModeOverride = "override"
)

// DirectSecretReference is the constant for direct secret references (~)
const DirectSecretReference = SecretReference("~")

// ProvisionerDescriptor describes the provisioner schema
type ProvisionerDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`

	provisioner Provisioner
}

func (s *ProvisionerDescriptor) GetProvisioner() Provisioner {
	return s.provisioner
}

func (s *ProvisionerDescriptor) SetProvisioner(p Provisioner) {
	s.provisioner = p
}

// ValuesOnly returns copy of descriptor without provisioner reference
func (s *ProvisionerDescriptor) ValuesOnly() ProvisionerDescriptor {
	return ProvisionerDescriptor{
		Type:    s.Type,
		Config:  s.Config,
		Inherit: s.Inherit,
	}
}

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
	Type          string `json:"type" yaml:"type"`
	Config        `json:",inline" yaml:",inline"`
	Inherit       `json:",inline" yaml:",inline"`
	SecretsConfig *EnvironmentSecretsConfig `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
}

// EnvironmentSecretsConfig defines environment-specific secret filtering rules
type EnvironmentSecretsConfig struct {
	// Mode determines how secrets are filtered: "include", "exclude", or "override"
	Mode string `json:"mode" yaml:"mode"`

	// Secrets is a map of environment names to their secret configurations
	Secrets map[string]SecretsConfigMap `json:"secrets" yaml:"secrets"`
}

// SecretsConfigMap defines secret reference patterns for an environment
type SecretsConfigMap struct {
	// InheritAll when true, all secrets from the parent are inherited (for exclude mode)
	InheritAll bool `json:"inheritAll,omitempty" yaml:"inheritAll,omitempty"`

	// Include lists secrets to make available (for include mode)
	Include []string `json:"include,omitempty" yaml:"include,omitempty"`

	// Exclude lists secrets to hide (for exclude mode)
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`

	// Override provides literal values or mappings for secrets (for override mode)
	Override map[string]string `json:"override,omitempty" yaml:"override,omitempty"`
}

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

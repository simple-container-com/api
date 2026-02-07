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
	Type         string                               `json:"type" yaml:"type"`
	SecretsConfig *EnvironmentSecretsConfigDescriptor `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
	Config       `json:",inline" yaml:",inline"`
	Inherit      `json:",inline" yaml:",inline"`
}

// EnvironmentSecretsConfigDescriptor configures environment-specific secret filtering in parent stacks
type EnvironmentSecretsConfigDescriptor struct {
	// Mode defines how secrets are filtered: "include", "exclude", or "override"
	// include: Only specified secrets are available
	// exclude: All secrets except specified ones are available (use with inheritAll: true)
	// override: All secrets are available, with specified values overriding secrets.yaml
	Mode string `json:"mode" yaml:"mode"`

	// InheritAll when true, all secrets from secrets.yaml are inherited (default: false)
	InheritAll bool `json:"inheritAll,omitempty" yaml:"inheritAll,omitempty"`

	// Secrets is a map of secret references with three patterns:
	// 1. Direct: "DATABASE_PASSWORD" -> uses DATABASE_PASSWORD from secrets.yaml
	// 2. Mapped: "DATABASE_PASSWORD" -> "DATABASE_PASSWORD_STAGING" (mapped to env-specific key)
	// 3. Literal: "DATABASE_PASSWORD" -> "${secret:DATABASE_PASSWORD_STAGING}" (explicit reference)
	Secrets SecretsConfigMap `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

// SecretsConfigMap defines how secrets are mapped/filtered for a specific environment
// Key: The secret name as referenced in client stacks
// Value: Either the actual secret key in secrets.yaml (for mapping) or a ${secret:KEY} reference
type SecretsConfigMap map[string]string

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

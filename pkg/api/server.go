// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

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

	// Plugins advertises out-of-tree cloud plugins SC core should spawn and
	// route resource dispatch to. Phase 0 Step 4 of sc-plugin-architecture;
	// details in forge/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md.
	Plugins map[string]PluginSpec `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

// PluginSpec configures one SC plugin that will be loaded at provision time.
// Operators give either a local path (dev / air-gapped) or a source block that
// SC resolves at load time against a GitHub Release.
type PluginSpec struct {
	// Path to the plugin binary on disk. Mutually exclusive with Source — if
	// both are set, Path wins. For dev loops + air-gapped operators.
	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	// Source resolves to a downloaded + cached binary. Set this for the
	// standard operator flow — points at a plugin repo's release.
	Source *PluginSource `json:"source,omitempty" yaml:"source,omitempty"`

	// SHA256 hex digest of the binary. When set, SC refuses to spawn a
	// binary whose hash does not match — the integrity guard from D10.
	// For Source-based specs, SC additionally cross-checks against the
	// release's sha256sums.txt asset if present. When empty, SC logs a
	// WARN but proceeds.
	SHA256 string `json:"sha256,omitempty" yaml:"sha256,omitempty"`
}

// PluginSource describes where SC should fetch a plugin binary from.
// Only "github-release" is supported in v1; "oci" is a planned addition
// (see 03 §6.2).
type PluginSource struct {
	// Type of the source. v1: "github-release".
	Type string `json:"type" yaml:"type"`

	// Repository identifier. For github-release: "owner/repo" (e.g.
	// "simple-container-com/sc-plugin-cloudflare").
	Repository string `json:"repository" yaml:"repository"`

	// Version tag — a specific release name. "latest" is NOT supported in
	// v1: operators pin a version explicitly so deploys are reproducible.
	Version string `json:"version" yaml:"version"`

	// BinaryName overrides the default binary filename resolution. Default
	// derivation: "sc-plugin-<name>-<goos>-<goarch>[.exe]" where <name> is
	// the repo name minus the "sc-plugin-" prefix (e.g.
	// simple-container-com/sc-plugin-cloudflare → "sc-plugin-cloudflare").
	BinaryName string `json:"binaryName,omitempty" yaml:"binaryName,omitempty"`
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
		Plugins:       sd.Plugins,
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
	// Registrar is the legacy single-registrar form. Mutually exclusive with Registrars.
	Registrar RegistrarDescriptor `json:"registrar" yaml:"registrar"`
	// Registrars allows several registrars to coexist, each serving its own DNS zones.
	// The one that handles a given domain is picked by zone suffix. See AllRegistrars.
	Registrars map[string]RegistrarDescriptor       `json:"registrars,omitempty" yaml:"registrars,omitempty"`
	Resources  map[string]PerEnvResourcesDescriptor `json:"resources" yaml:"resources"`
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

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"fmt"

	"github.com/samber/lo"
)

const (
	// ServerSchemaVersion is the DEFAULT value SC uses when writing a new
	// server.yaml. Not a required pin — an empty schemaVersion field is
	// treated as the 1.0-compatible lenient mode.
	ServerSchemaVersion = "1.0"

	// ServerSchemaVersion11 is the strict-mode version. Setting
	// `schemaVersion: 1.1` in server.yaml opts into:
	//   - `plugins:` MUST declare any out-of-tree plugin-owned type the
	//     stack uses. No GitHub auto-inject fallback.
	//   - Unknown schemaVersion strings fail at parse time.
	// 1.0 (or empty) keeps the pre-A3 lenient behaviour.
	ServerSchemaVersion11 = "1.1"

	// UpgradingToPluginsDocsURL is the migration guide operators are
	// pointed at from every plugin-related parse error. Keep in one place
	// so a URL change in the docs site is a single edit here.
	UpgradingToPluginsDocsURL = "https://docs.simple-container.com/guides/upgrading-to-plugins/"
)

// SupportedServerSchemaVersions lists every value `schemaVersion:` may
// legally hold. An empty string is treated as implicit 1.0 and does not
// need to appear here; the validator special-cases it.
var SupportedServerSchemaVersions = map[string]bool{
	ServerSchemaVersion:   true, // 1.0
	ServerSchemaVersion11: true, // 1.1
}

// IsStrictSchemaVersion reports whether the descriptor opts into strict
// mode (schemaVersion 1.1 or newer). Call sites gate plugin-aware
// behaviour on this — in strict mode we refuse to auto-inject or guess;
// in lenient mode (1.0 / empty) we preserve the pre-A3 fallback UX.
func (sd *ServerDescriptor) IsStrictSchemaVersion() bool {
	if sd == nil {
		return false
	}
	return sd.SchemaVersion == ServerSchemaVersion11
}

// ValidateSchemaVersion rejects a server.yaml carrying an unknown
// schemaVersion string. Empty is accepted (implicit 1.0). Call this
// early, before any plugin-aware parse step runs.
func (sd *ServerDescriptor) ValidateSchemaVersion() error {
	if sd == nil || sd.SchemaVersion == "" {
		return nil
	}
	if SupportedServerSchemaVersions[sd.SchemaVersion] {
		return nil
	}
	known := make([]string, 0, len(SupportedServerSchemaVersions))
	for v := range SupportedServerSchemaVersions {
		known = append(known, v)
	}
	// Sort for stable error output. The map is small (<10 entries) so
	// the allocation is negligible.
	for i := 1; i < len(known); i++ {
		for j := i; j > 0 && known[j] < known[j-1]; j-- {
			known[j], known[j-1] = known[j-1], known[j]
		}
	}
	return fmt.Errorf("unsupported schemaVersion %q — this build accepts: %v. See %s for the upgrade guide",
		sd.SchemaVersion, known, UpgradingToPluginsDocsURL)
}

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

	// ComponentRaw carries the operator-declared `.Raw` map of a Component
	// resource when SC core's sidecar wrapper dispatches this input into a
	// `deployTo:` template (slice 1a.1 of sc-components-platform; design doc
	// §#19 path (b)). The dispatched template's ProvisionFunc checks for a
	// non-nil ComponentRaw and overlays the component's fields (image, env,
	// ingress, signingKey, …) onto its typed input struct before building
	// the cloud resource. nil for non-component dispatches — every existing
	// ProvisionFunc on main sees this unset and keeps working. See
	// forge/docs/design/2026-11-10/sc-components-platform/README.md §#19 for
	// the shape + the memory
	// `interface_assertion_against_plugin_owned_value_fails_in_prod` for why
	// SC core cannot type-overlay plugin-owned fields itself.
	ComponentRaw map[string]any `json:"component_raw,omitempty" yaml:"component_raw,omitempty"`
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

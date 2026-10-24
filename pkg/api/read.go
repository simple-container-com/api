// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
)

const (
	ServerDescriptorFileName  = "server.yaml"
	SecretsDescriptorFileName = "secrets.yaml"
	ClientDescriptorFileName  = "client.yaml"
)

func ReadDescriptor[T any](filePath string, descriptor *T) (*T, error) {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", filePath)
	}

	err = yaml.Unmarshal(fileBytes, descriptor)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal %s", filePath)
	}
	return descriptor, nil
}

func MarshalDescriptor[T any](descriptor *T) ([]byte, error) {
	return yaml.Marshal(descriptor)
}

func UnmarshalDescriptor[T any](bytes []byte) (*T, error) {
	var descriptor T
	err := yaml.Unmarshal(bytes, &descriptor)
	if err != nil {
		return nil, err
	}
	return &descriptor, nil
}

func ReadServerDescriptor(path string) (*ServerDescriptor, error) {
	descriptor, err := ReadDescriptor(path, &ServerDescriptor{})
	if err != nil {
		return descriptor, errors.Wrapf(err, "failed to read server descriptor from %q", path)
	}
	res, err := ReadServerConfigs(descriptor)
	if err != nil {
		return descriptor, errors.Wrapf(err, "failed to read server configs for %s", path)
	}

	return res, nil
}

func ReadSecretsDescriptor(path string) (*SecretsDescriptor, error) {
	descriptor, err := ReadDescriptor(path, &SecretsDescriptor{})
	if err != nil {
		return nil, err
	}
	res, err := ReadSecretsConfigs(descriptor)
	if err != nil {
		return descriptor, errors.Wrapf(err, "failed to read secret configs for %s", path)
	}

	return res, nil
}

func ReadClientDescriptor(path string) (*ClientDescriptor, error) {
	descriptor, err := ReadDescriptor(path, &ClientDescriptor{})
	if err != nil {
		return descriptor, errors.Wrapf(err, "failed to unmarshal %s", path)
	}
	for env, cfg := range descriptor.Stacks {
		if res, err := ConvertClientConfig(cfg); err != nil {
			return nil, errors.Wrapf(err, "failed to convert client config for env %q in %q", env, path)
		} else {
			descriptor.Stacks[env] = *res
		}
	}
	return descriptor, nil
}

func ReadSecretsConfigs(descriptor *SecretsDescriptor) (*SecretsDescriptor, error) {
	res := *descriptor

	if withAuth, err := DetectAuthType(&res); err != nil {
		return nil, err
	} else {
		res = *withAuth
	}
	return &res, nil
}

func DetectAuthType(descriptor *SecretsDescriptor) (*SecretsDescriptor, error) {
	for name, auth := range descriptor.Auth {
		if auth.IsInherited() {
			if len(auth.Type) > 0 {
				return descriptor, errors.Errorf("auth %q is inherited, but type %q is defined", name, auth.Type)
			}
			continue
		}
		processedAuth, err := DetectAuthProvider(&auth)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to detect auth type for %q", name)
		}
		descriptor.Auth[name] = *processedAuth
	}
	return descriptor, nil
}

// ParseAuthDescriptor reads one auth entry, written as the YAML that sits under its
// name in a secrets.yaml `auth:` map, and converts its config to the provider's
// type. A standalone entry has no parent to inherit from, so inherit is refused.
func ParseAuthDescriptor(text string) (AuthDescriptor, error) {
	var auth AuthDescriptor
	if err := yaml.Unmarshal([]byte(text), &auth); err != nil {
		return AuthDescriptor{}, errors.Wrap(err, "auth entry is not valid YAML")
	}
	if auth.IsInherited() {
		return AuthDescriptor{}, errors.New("auth entry cannot inherit here; give its type and config")
	}
	if auth.Type == "" {
		return AuthDescriptor{}, errors.New("auth entry has no type")
	}
	parsed, err := DetectAuthProvider(&auth)
	if err != nil {
		return AuthDescriptor{}, err
	}
	return *parsed, nil
}

func DetectAuthProvider(auth *AuthDescriptor) (*AuthDescriptor, error) {
	if fn, found := providerConfigMapping[auth.Type]; !found {
		return nil, errors.Errorf("unknown auth type %q", auth.Type)
	} else {
		var err error
		auth.Config, err = fn(&auth.Config)
		if err != nil {
			return nil, err
		}
	}
	return auth, nil
}

func ReadServerConfigs(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if descriptor == nil {
		return nil, errors.Errorf("failed to read descriptor: reference is nil")
	}
	res := *descriptor

	if withProvisioner, err := DetectProvisionerType(&res); err != nil {
		return nil, err
	} else {
		res = *withProvisioner
	}

	if withSecrets, err := DetectSecretsType(&res); err != nil {
		return nil, err
	} else {
		res = *withSecrets
	}

	if withTemplates, err := DetectTemplatesType(&res); err != nil {
		return nil, err
	} else {
		res = *withTemplates
	}

	if withResources, err := DetectResourcesType(&res); err != nil {
		return nil, err
	} else {
		res = *withResources
	}

	if withCicd, err := DetectCiCdType(&res); err != nil {
		return nil, err
	} else {
		res = *withCicd
	}

	return &res, nil
}

func DetectCiCdType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if descriptor.CiCd.IsInherited() {
		return descriptor, nil
	}
	if descriptor.CiCd.Type == "" {
		return descriptor, nil // skip cicd
	}

	if fn, found := providerConfigMapping[descriptor.CiCd.Type]; !found {
		return nil, errors.Errorf("unknown cicd type %q", descriptor.CiCd.Type)
	} else {
		var err error
		descriptor.CiCd.Config, err = fn(&descriptor.CiCd.Config)
		if err != nil {
			return descriptor, err
		}
	}
	return descriptor, nil
}

func DetectResourcesType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if withRegistrar, err := DetectRegistrarType(&descriptor.Resources); err != nil {
		return nil, err
	} else {
		descriptor.Resources = *withRegistrar
	}
	if withResources, err := DetectPerStackResourcesType(&descriptor.Resources); err != nil {
		return nil, err
	} else {
		descriptor.Resources = *withResources
	}

	return descriptor, nil
}

func DetectPerStackResourcesType(p *PerStackResourcesDescriptor) (*PerStackResourcesDescriptor, error) {
	for stackName, resources := range p.Resources {
		if resources.IsInherited() {
			if len(resources.Resources) > 0 {
				return p, errors.Errorf("resources are inherited, but resources are defined for stack %q", stackName)
			}
			continue
		}
		for resourceName, resource := range resources.Resources {
			if resource.IsInherited() {
				if len(resource.Type) > 0 {
					return p, errors.Errorf("resource %q is inherited, but type is defined for stack %q", resourceName, stackName)
				}
				continue
			}
			fn, found := providerConfigMapping[resource.Type]
			if !found {
				// Plugin fall-through: types with a package:module:Type shape
				// (Pulumi gRPC type-token convention) are routed to SC plugins
				// at provision time. The config-tier skips strict validation
				// for them — the plugin's Schema RPC authoritatively declares
				// which types it owns. See
				// forge/docs/design/2026-10-24/sc-plugin-architecture/.
				if strings.Count(resource.Type, ":") >= 2 {
					resources.Resources[resourceName] = resource
					p.Resources[stackName] = resources
					continue
				}
				return nil, errors.Errorf("unknown type %q for resource %q", resource.Type, resourceName)
			}
			var err error
			resource.Config, err = fn(&resource.Config)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to read resource %q for stack %q", resourceName, stackName)
			}
			if withDepProviders, ok := resource.Config.Config.(WithDependencyProviders); ok {
				depProviders := withDepProviders.DependencyProviders()
				for name, authCfg := range depProviders {
					procAuthCfg, err := DetectAuthProvider(&authCfg)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to detect auth type for dependency provider %q in resource %q for stack %q", name, resourceName, stackName)
					}
					depProviders[name] = *procAuthCfg
				}
			}
			resources.Resources[resourceName] = resource
		}
	}
	return p, nil
}

func DetectRegistrarType(p *PerStackResourcesDescriptor) (*PerStackResourcesDescriptor, error) {
	// inherited registrars are resolved later, an empty type means "not configured"
	skip := func(registrar RegistrarDescriptor) bool {
		return registrar.IsInherited() || registrar.Type == ""
	}
	// the bool reports whether the type is known at all, which decides whether an
	// unusable descriptor is returned alongside the error (as it always has been)
	detect := func(registrar RegistrarDescriptor) (RegistrarDescriptor, bool, error) {
		fn, found := providerConfigMapping[registrar.Type]
		if !found {
			// Plugin fall-through — same mechanism as the resource and
			// provisioner-field fall-throughs. A registrar type with `:`
			// prefix is handled by a pre-engine SC plugin's
			// ProvisionRegistrar RPC.
			if strings.Contains(registrar.Type, ":") {
				registrar.Config = pluginFieldConfig(registrar.Type, &registrar.Config)
				return registrar, true, nil
			}
			return registrar, false, errors.Errorf("unknown registrar type %q", registrar.Type)
		}
		config, err := fn(&registrar.Config)
		if err != nil {
			return registrar, true, err
		}
		registrar.Config = config
		return registrar, true, nil
	}

	if !skip(p.Registrar) {
		registrar, known, err := detect(p.Registrar)
		if !known {
			return nil, err
		} else if err != nil {
			return p, err
		}
		p.Registrar = registrar
	}
	for name, registrar := range p.Registrars {
		if skip(registrar) {
			continue
		}
		detected, known, err := detect(registrar)
		if !known {
			return nil, errors.Wrapf(err, "registrar %q", name)
		} else if err != nil {
			return p, errors.Wrapf(err, "registrar %q", name)
		}
		p.Registrars[name] = detected
	}
	return p, nil
}

func DetectTemplatesType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	for name, tpl := range descriptor.Templates {
		if tpl.IsInherited() {
			continue
		}
		stackDescriptor, err := DetectTemplateType(tpl)
		if err != nil {
			return descriptor, err
		}
		descriptor.Templates[name] = *stackDescriptor
	}
	return descriptor, nil
}

func DetectTemplateType(tpl StackDescriptor) (*StackDescriptor, error) {
	if fn, found := providerConfigMapping[tpl.Type]; !found {
		return nil, errors.Errorf("unknown template type %q", tpl.Type)
	} else {
		stackDesc := tpl
		var err error
		stackDesc.Config, err = fn(&stackDesc.Config)
		return &stackDesc, err
	}
}

func DetectSecretsType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if descriptor.Secrets.IsInherited() {
		return descriptor, nil
	}
	if descriptor.Secrets.Type == "" { // skip secrets
		return descriptor, nil
	}

	if fn, found := providerConfigMapping[descriptor.Secrets.Type]; !found {
		return nil, errors.Errorf("unknown secrets type %q", descriptor.Secrets.Type)
	} else {
		var err error
		descriptor.Secrets.Config, err = fn(&descriptor.Secrets.Config)
		if err != nil {
			return descriptor, err
		}
	}
	return descriptor, nil
}

func DetectProvisionerType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if descriptor.Provisioner.IsInherited() {
		return descriptor, nil
	}
	if fn, found := providerConfigMapping[descriptor.Provisioner.Type]; !found {
		return nil, errors.Errorf("unknown provisioner type %q", descriptor.Provisioner.Type)
	} else {
		var err error
		descriptor.Provisioner.Config, err = fn(&descriptor.Provisioner.Config)
		if err != nil {
			return descriptor, err
		}
	}
	if fn, found := provisionerConfigMapping[descriptor.Provisioner.Type]; !found {
		return nil, errors.Errorf("unknown provisioner type %q", descriptor.Provisioner.Type)
	} else {
		var err error
		descriptor.Provisioner.provisioner, err = fn(
			descriptor.Provisioner.Config,
			WithFieldConfigReader(ReadProvisionerFieldConfig),
		)
		if err != nil {
			return descriptor, err
		}
	}
	return descriptor, nil
}

func ReadProvisionerFieldConfig(cType string, config *Config) (Config, error) {
	if fn, found := provisionerFieldConfigMapping[cType]; !found {
		// Plugin fall-through — same mechanism as DetectPerStackResourcesType
		// uses for resource types. A provisioner field type with a `:` prefix
		// (sc-plugin:name shape, or similar namespaced token) is routed to a
		// pre-engine SC plugin; the plugin's InitStateStore / InitSecretsProvider
		// RPC takes the raw config. See Addendum C in
		// forge/docs/design/2026-10-24/sc-plugin-architecture/
		// 04-no-destroy-invariant.md.
		if strings.Contains(cType, ":") {
			return pluginFieldConfig(cType, config), nil
		}
		return *config, errors.Errorf("unknown provisioner field config type %q", cType)
	} else {
		return fn(config)
	}
}

// pluginFieldConfig wraps a plugin-provided provisioner-field config in a
// passthrough struct that implements api.StateStorageConfig (and
// api.SecretsProviderConfig via the same interface subset Pulumi actually
// uses — see pkg/clouds/pulumi/login.go). The plugin's actual config lives
// in the inner `raw` field as deserialised YAML; InitStateStoreFunc proxies
// to the plugin which reads it. SC-core-side only needs url + credentials
// to configure the Pulumi workspace.
func pluginFieldConfig(cType string, config *Config) Config {
	raw, _ := config.Config.(map[string]any)
	return Config{Config: &PluginFieldConfig{
		TypeName:    cType,
		Raw:         raw,
		URL:         stringFromMap(raw, "url"),
		Credentials: stringFromMap(raw, "credentials"),
		ProjectID:   stringFromMap(raw, "projectId"),
		Provision:   boolFromMap(raw, "provision"),
	}}
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func boolFromMap(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}

// PluginFieldConfig is the SC-core-side carrier for a provisioner-field
// config (state-storage or secrets-provider) that a plugin owns. Implements
// api.StateStorageConfig and api.AuthConfig minimally; the plugin itself
// has the authoritative view of its config via its own InitStateStore RPC.
type PluginFieldConfig struct {
	TypeName    string         `json:"type" yaml:"type"`
	Raw         map[string]any `json:"-" yaml:"-"`
	URL         string         `json:"url" yaml:"url"`
	Credentials string         `json:"credentials" yaml:"credentials"`
	ProjectID   string         `json:"projectId" yaml:"projectId"`
	Provision   bool           `json:"provision" yaml:"provision"`
}

func (p *PluginFieldConfig) ProviderType() string     { return p.TypeName }
func (p *PluginFieldConfig) CredentialsValue() string { return p.Credentials }
func (p *PluginFieldConfig) ProjectIdValue() string   { return p.ProjectID }
func (p *PluginFieldConfig) StorageUrl() string       { return p.URL }
func (p *PluginFieldConfig) KeyUrl() string           { return p.URL } // secrets-provider interface
func (p *PluginFieldConfig) IsProvisionEnabled() bool { return p.Provision }
func (p *PluginFieldConfig) UsesAmbientCredentials() bool {
	return p.Credentials == "" // if no explicit creds, assume ambient (env/role)
}

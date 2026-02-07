package api

import (
	"os"

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
				return nil, errors.Errorf("unknown type %q for resource %q", resource.Type, resourceName)
			}
			var err error
			resource.Config, err = fn(&resource.Config)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to read resource %q for stack %q", resourceName, stackName)
			}
			if withDepProviders, ok := (resource.Config.Config).(WithDependencyProviders); ok {
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
	registrar := p.Registrar
	if registrar.IsInherited() {
		return p, nil
	}
	if registrar.Type == "" { // skip registrar when not configured
		return p, nil
	}
	if fn, found := providerConfigMapping[registrar.Type]; !found {
		return nil, errors.Errorf("unknown registrar type %q", registrar.Type)
	} else {
		var err error
		registrar.Config, err = fn(&registrar.Config)
		if err != nil {
			return p, err
		}
		p.Registrar = registrar
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

	// Process secretsConfig if present
	if descriptor.Secrets.SecretsConfig != nil {
		if err := DetectSecretsConfigType(descriptor.Secrets.SecretsConfig); err != nil {
			return nil, errors.Wrapf(err, "failed to detect secretsConfig type")
		}
	}

	return descriptor, nil
}

// DetectSecretsConfigType validates the environment-specific secrets configuration
func DetectSecretsConfigType(config *EnvironmentSecretsConfigDescriptor) error {
	if config == nil {
		return nil
	}

	// Validate mode
	validModes := map[string]bool{
		"include":  true,
		"exclude":  true,
		"override": true,
	}
	if !validModes[config.Mode] {
		return errors.Errorf("invalid secretsConfig mode %q (must be 'include', 'exclude', or 'override')", config.Mode)
	}

	// Validate exclude mode requires inheritAll
	if config.Mode == "exclude" && !config.InheritAll {
		return errors.Errorf("exclude mode requires inheritAll: true to be set")
	}

	// Validate secret references
	for refName, value := range config.Secrets {
		if IsSecretReference(value) {
			if err := ValidateSecretReference(value); err != nil {
				return errors.Wrapf(err, "invalid secret reference %q for %q", value, refName)
			}
		}
	}

	return nil
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
		return *config, errors.Errorf("unknown provisioner field config type %q", cType)
	} else {
		return fn(config)
	}
}

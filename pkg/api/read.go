package api

import (
	"os"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
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

	res, err := ReadServerConfigs(descriptor)
	if err != nil {
		return descriptor, errors.Wrapf(err, "failed to read server configs for %s", path)
	}

	return res, nil
}

func ReadSecretsDescriptor(path string) (*SecretsDescriptor, error) {
	descriptor, err := ReadDescriptor(path, &SecretsDescriptor{})

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
		if fn, found := providerConfigMapping[auth.Type]; !found {
			return nil, errors.Errorf("unknown auth type %q for auth %q", auth.Type, name)
		} else {
			var err error
			auth.Config, err = fn(&auth.Config)
			if err != nil {
				return descriptor, err
			}
			descriptor.Auth[name] = auth
		}
	}
	return descriptor, nil
}

func ReadServerConfigs(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
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
		if fn, found := providerConfigMapping[tpl.Type]; !found {
			return nil, errors.Errorf("unknown template type %q for %q", tpl.Type, name)
		} else {
			stackDesc := descriptor.Templates[name]
			var err error
			stackDesc.Config, err = fn(&stackDesc.Config)
			if err != nil {
				return descriptor, err
			}
			descriptor.Templates[name] = stackDesc
		}
	}
	return descriptor, nil
}

func DetectSecretsType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if descriptor.Secrets.IsInherited() {
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
		descriptor.Provisioner.provisioner, err = fn()
		if err != nil {
			return descriptor, err
		}
	}
	return descriptor, nil
}

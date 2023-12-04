package api

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"os"
)

type configReaderFunc func(any) (any, error)

var cloudMapping = map[string]configReaderFunc{
	ProvisionerTypePulumi:        PulumiReadProvisionerConfig,
	SecretsTypeGCPSecretsManager: GcloudReadSecretsConfig,
	TemplateTypeGcpCloudrun:      GcloudReadTemplateConfig,
	RegistrarTypeCloudflare:      CloudflareReadRegistrarConfig,
}

func ReadServerDescriptor(path string) (*ServerDescriptor, error) {
	var descriptor ServerDescriptor
	fileBytes, err := os.ReadFile(path)

	if err != nil {
		return &descriptor, errors.Wrapf(err, "failed to read %s", path)
	}

	err = yaml.Unmarshal(fileBytes, &descriptor)

	if err != nil {
		return &descriptor, errors.Wrapf(err, "failed to unmarshal %s", path)
	}

	res, err := ReadServerConfigs(&descriptor)
	if err != nil {
		return &descriptor, errors.Wrapf(err, "failed to read server configs for %s", path)
	}

	return res, nil
}

func ConvertDescriptor[T any](from any, to *T) (*T, error) {
	if bytes, err := yaml.Marshal(from); err == nil {
		if err = yaml.Unmarshal(bytes, to); err != nil {
			return nil, err
		} else {
			return to, nil
		}
	} else {
		return nil, err
	}
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

	return &res, nil
}

func DetectResourcesType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if withRegistrar, err := DetectRegistrarType(&descriptor.Resources); err != nil {
		return nil, err
	} else {
		descriptor.Resources = *withRegistrar
	}
	return descriptor, nil
}

func DetectRegistrarType(p *PerStackResourcesDescriptor) (*PerStackResourcesDescriptor, error) {
	registrar := p.Registrar
	if fn, found := cloudMapping[registrar.Type]; !found {
		return nil, errors.Errorf("unknown registrar type %q", registrar.Type)
	} else {
		var err error
		registrar.Config, err = fn(registrar.Config)
		if err != nil {
			return p, err
		}
		p.Registrar = registrar
	}
	return p, nil
}

func DetectTemplatesType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	for name, tpl := range descriptor.Templates {
		if fn, found := cloudMapping[tpl.Type]; !found {
			return nil, errors.Errorf("unknown template type %q for %q", tpl.Type, name)
		} else {
			stackDesc := descriptor.Templates[name]
			var err error
			stackDesc.Config, err = fn(stackDesc.Config)
			if err != nil {
				return descriptor, err
			}
			descriptor.Templates[name] = stackDesc
		}
	}
	return descriptor, nil
}

func DetectSecretsType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if fn, found := cloudMapping[descriptor.Secrets.Type]; !found {
		return nil, errors.Errorf("unknown secrets type %q", descriptor.Secrets.Type)
	} else {
		var err error
		descriptor.Secrets.Config, err = fn(descriptor.Secrets.Config)
		if err != nil {
			return descriptor, err
		}
	}
	return descriptor, nil
}

func DetectProvisionerType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if fn, found := cloudMapping[descriptor.Provisioner.Type]; !found {
		return nil, errors.Errorf("unknown provisioner type %q", descriptor.Provisioner.Type)
	} else {
		var err error
		descriptor.Provisioner.Config, err = fn(descriptor.Provisioner.Config)
		if err != nil {
			return descriptor, err
		}
	}
	return descriptor, nil
}

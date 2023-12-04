package api

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"os"
)

type configReaderFunc func(any) any

var cloudMapping = map[string]configReaderFunc{
	ProvisionerTypePulumi:        PulumiReadProvisionerConfig,
	SecretsTypeGCPSecretsManager: GcloudReadSecretsConfig,
	TemplateTypeGcpCloudrun:      GcloudReadTemplateConfig,
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

	return &res, nil
}

func DetectTemplatesType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	for name, tpl := range descriptor.Templates {
		if fn, found := cloudMapping[tpl.Type]; !found {
			return nil, errors.Errorf("unknown template type %q for %q", tpl.Type, name)
		} else {
			stackDesc := descriptor.Templates[name]
			stackDesc.Config = fn(stackDesc.Config)
			descriptor.Templates[name] = stackDesc
		}
	}
	return descriptor, nil
}

func DetectSecretsType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if fn, found := cloudMapping[descriptor.Secrets.Type]; !found {
		return nil, errors.Errorf("unknown secrets type %q", descriptor.Secrets.Type)
	} else {
		descriptor.Secrets.Config = fn(descriptor.Secrets.Config)
	}
	return descriptor, nil
}

func DetectProvisionerType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
	if fn, found := cloudMapping[descriptor.Provisioner.Type]; !found {
		return nil, errors.Errorf("unknown provisioner type %q", descriptor.Provisioner.Type)
	} else {
		descriptor.Provisioner.Config = fn(descriptor.Provisioner.Config)
	}
	return descriptor, nil
}

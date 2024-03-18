package api

import (
	"context"

	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	ClientSchemaVersion = "1.0"

	ClientTypeCompose = "compose"
	ClientTypeStatic  = "static"
)

// ClientDescriptor describes the client schema
type ClientDescriptor struct {
	SchemaVersion string                           `json:"schemaVersion" yaml:"schemaVersion"`
	Stacks        map[string]StackClientDescriptor `json:"stacks" yaml:"stacks"`
}

type StackClientDescriptor struct {
	Type        string `json:"type" yaml:"type"`
	ParentStack string `json:"parent" yaml:"parent"`
	Environment string `json:"environment" yaml:"environment"`
	Domain      string `json:"domain" yaml:"domain"`
	Config      Config `json:",inline" yaml:",inline"`
}

type StackConfigCompose struct {
	DockerComposeFile string   `json:"docker-compose-file" yaml:"docker-compose-file"`
	Uses              []string `json:"uses" yaml:"uses"`
	Runs              []string `json:"runs" yaml:"runs"`
}

type StackConfigStatic struct {
	BundleDir string `json:"bundle-dir" yaml:"bundle-dir"`
}

type DeployParams struct {
	RootDir     string         `json:"rootDir" yaml:"rootDir"`
	Profile     string         `json:"profile" yaml:"profile"`
	StackName   string         `json:"stack" yaml:"stack"`
	ParentStack string         `json:"parent" yaml:"parent"`
	Environment string         `json:"environment" yaml:"environment"`
	Vars        VariableValues `json:"vars" yaml:"vars"`
}

func PrepareCloudComposeForDeploy(ctx context.Context, rootDir, stackName string, tpl StackDescriptor, clientConfig *StackConfigCompose) (*StackDescriptor, error) {
	stackDesc, err := DetectTemplateType(tpl)
	if err != nil {
		return nil, err
	}

	composeCfg, err := compose.ReadDockerCompose(ctx, rootDir, clientConfig.DockerComposeFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read docker-compose config from %q/%q", rootDir, clientConfig.DockerComposeFile)
	}

	if tplFun, found := cloudComposeConverterMapping[stackDesc.Type]; !found {
		return nil, errors.Errorf("unknown template type %q for %q", stackDesc.Type, stackName)
	} else if input, err := tplFun(stackDesc.Config.Config, composeCfg, clientConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert cloud compose for type %q in stack %q", stackDesc.Type, stackName)
	} else {
		return &StackDescriptor{
			Type: stackDesc.Type,
			Config: Config{
				Config: input,
			},
		}, nil
	}
}

func PrepareStaticForDeploy(ctx context.Context, rootDir, stackName string, tpl StackDescriptor, clientConfig *StackConfigStatic) (*StackDescriptor, error) {
	return nil, errors.Errorf("not implemented")
}

func PrepareClientConfigForDeploy(ctx context.Context, rootDir, stackName string, tpl StackDescriptor, clientDesc StackClientDescriptor) (*StackDescriptor, error) {
	if clientDesc.Type == ClientTypeCompose {
		configCompose, ok := clientDesc.Config.Config.(*StackConfigCompose)
		if !ok {
			return nil, errors.Errorf("client config is not of type *StackConfigCompose")
		}
		return PrepareCloudComposeForDeploy(ctx, rootDir, stackName, tpl, configCompose)
	}
	if clientDesc.Type == ClientTypeStatic {
		configStatic, ok := clientDesc.Config.Config.(*StackConfigStatic)
		if !ok {
			return nil, errors.Errorf("client config is not of type *StackConfigStatic")
		}
		return PrepareStaticForDeploy(ctx, rootDir, stackName, tpl, configStatic)
	}
	return nil, errors.Errorf("unsupported client type %q", tpl.Type)
}

func ConvertClientConfig(clientDesc StackClientDescriptor) (*StackClientDescriptor, error) {
	switch clientDesc.Type {
	case ClientTypeStatic:
		if converted, err := ConvertConfig(&clientDesc.Config, &StackConfigStatic{}); err != nil {
			return &clientDesc, err
		} else {
			clientDesc.Config = converted
		}
	case ClientTypeCompose:
		if converted, err := ConvertConfig(&clientDesc.Config, &StackConfigCompose{}); err != nil {
			return &clientDesc, err
		} else {
			clientDesc.Config = converted
		}
	}
	return &clientDesc, nil
}

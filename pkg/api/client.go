package api

import (
	"context"

	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

const (
	ClientSchemaVersion = "1.0"

	ClientTypeCloudCompose = "cloud-compose"
	ClientTypeStatic       = "static"
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
	Config      Config `json:",inline" yaml:",inline"`
}

type StackConfigCompose struct {
	DockerComposeFile string   `json:"dockerComposeFile" yaml:"dockerComposeFile"`
	Domain            string   `json:"domain" yaml:"domain"`
	Uses              []string `json:"uses" yaml:"uses"`
	Runs              []string `json:"runs" yaml:"runs"`
}

type StackConfigStatic struct {
	BundleDir          string `json:"bundleDir" yaml:"bundleDir"`
	Domain             string `json:"domain" yaml:"domain"`
	IndexDocument      string `json:"indexDocument" yaml:"indexDocument"`
	ErrorDocument      string `json:"errorDocument" yaml:"errorDocument"`
	ProvisionWwwDomain bool   `json:"provisionWwwDomain" yaml:"provisionWwwDomain"`
}

type StackParams struct {
	RootDir     string `json:"rootDir" yaml:"rootDir"`
	Profile     string `json:"profile" yaml:"profile"`
	StackName   string `json:"stack" yaml:"stack"`
	ParentStack string `json:"parent" yaml:"parent"`
	Environment string `json:"environment" yaml:"environment"`
}

type DeployParams struct {
	StackParams `json:",inline" yaml:",inline"`
	Vars        VariableValues `json:"vars" yaml:"vars"`
}

type PreviewResult struct {
	Operations map[string]int `json:"operations" yaml:"operations"`
}

type DestroyParams struct {
	StackParams `json:",inline" yaml:",inline"`
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
	stackDesc, err := DetectTemplateType(tpl)
	if err != nil {
		return nil, err
	}

	if tplFun, found := cloudStaticSiteConverterMapping[stackDesc.Type]; !found {
		return nil, errors.Errorf("unknown template type %q for %q", stackDesc.Type, stackName)
	} else if input, err := tplFun(stackDesc.Config.Config, rootDir, stackName, clientConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert cloud static site for type %q in stack %q", stackDesc.Type, stackName)
	} else {
		return &StackDescriptor{
			Type: stackDesc.Type,
			Config: Config{
				Config: input,
			},
		}, nil
	}
}

func PrepareClientConfigForDeploy(ctx context.Context, rootDir, stackName string, tpl StackDescriptor, clientDesc StackClientDescriptor) (*StackDescriptor, error) {
	if fnc, found := clientConfigsPrepareMap[clientDesc.Type]; !found {
		return nil, errors.Errorf("unsupported client type %q", tpl.Type)
	} else if sDesc, err := fnc(ctx, rootDir, stackName, tpl, clientDesc); err != nil {
		return nil, errors.Wrapf(err, "failed to prepare config for deploy")
	} else {
		return sDesc, nil
	}
}

func ConvertClientConfig(clientDesc StackClientDescriptor) (*StackClientDescriptor, error) {
	if fnc, found := clientConfigsConvertMap[clientDesc.Type]; !found {
		return nil, errors.Errorf("unsupported client config type %q", clientDesc.Type)
	} else if converted, err := fnc(&clientDesc.Config); err != nil {
		return nil, errors.Wrapf(err, "failed to convert client config")
	} else {
		clientDesc.Config = converted
		return &clientDesc, nil
	}
}

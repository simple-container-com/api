package kubernetes

import (
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	pcfg, ok := input.Descriptor.Config.Config.(k8s.KubernetesConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	creds := pcfg.CredentialsValue()

	provider, err := kubernetes.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &kubernetes.ProviderArgs{
		Kubeconfig: sdk.String(creds),
	})

	return &api.ResourceOutput{
		Ref: provider,
	}, err
}

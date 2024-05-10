package aws

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/secretsmanager"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

type CreatedSecret struct {
	Secret *secretsmanager.Secret
	EnvVar string
}

func toSecretName(params api.StackParams, resType, resName, varName, suffix string) string {
	return fmt.Sprintf("%s--%s--%s--%s--%s%s", params.StackName, params.Environment, resType, resName, varName, suffix)
}

func createSecret(ctx *sdk.Context, secretName, envVar, value string, opts ...sdk.ResourceOption) (*CreatedSecret, error) {
	secret, err := secretsmanager.NewSecret(ctx, secretName, &secretsmanager.SecretArgs{
		Name: sdk.String(secretName),
	}, opts...)
	if err != nil {
		return nil, err
	}
	_, err = secretsmanager.NewSecretVersion(ctx, secretName, &secretsmanager.SecretVersionArgs{
		SecretId:     secret.Arn,
		SecretString: sdk.String(value),
	}, opts...)
	if err != nil {
		return nil, err
	}
	return &CreatedSecret{
		Secret: secret,
		EnvVar: envVar,
	}, nil
}

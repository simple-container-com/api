package gcp

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/projects"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/util"
)

type ServiceAccount struct {
	ServiceAccount     *serviceaccount.Account
	ServiceAccountKey  *serviceaccount.Key
	CredentialsSecrets sdk.StringMap
}

type ServiceAccountArgs struct {
	Project     string
	Description string
	Roles       []string
}

func NewServiceAccount(ctx *sdk.Context, name string, args ServiceAccountArgs, opts ...sdk.ResourceOption) (*CloudSQLAccount, error) {
	accountName := strings.ReplaceAll(util.TrimStringMiddle(name, 28, "-"), "--", "-")

	serviceAccount, err := serviceaccount.NewAccount(ctx, accountName, &serviceaccount.AccountArgs{
		AccountId:   sdk.String(accountName),
		Project:     sdk.String(args.Project),
		Description: sdk.String(args.Description),
		DisplayName: sdk.String(fmt.Sprintf("%s-service-account", name)),
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, sdk.Parent(serviceAccount))
	serviceAccountKey, err := serviceaccount.NewKey(ctx, fmt.Sprintf("%s-key", accountName), &serviceaccount.KeyArgs{
		ServiceAccountId: serviceAccount.AccountId,
	}, opts...)
	if err != nil {
		return nil, err
	}
	for _, role := range args.Roles {
		_, err = projects.NewIAMMember(ctx, fmt.Sprintf("%s-%s-iam", accountName, role), &projects.IAMMemberArgs{
			Project: sdk.String(args.Project),
			Member:  serviceAccount.Member,
			Role:    sdk.String(role),
		}, opts...)
		if err != nil {
			return nil, err
		}
	}

	credentialsSecrets := sdk.StringMap{
		"credentials.json": serviceAccountKey.PrivateKey,
	}

	return &CloudSQLAccount{
		ServiceAccount:     serviceAccount,
		ServiceAccountKey:  serviceAccountKey,
		CredentialsSecrets: credentialsSecrets,
	}, nil
}

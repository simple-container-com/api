package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

type Registrar interface {
	MainDomain() string
	ProvisionRecords(ctx *sdk.Context, params ProvisionParams) (*api.ResourceOutput, error)
	NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error)
	NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule OverrideHeaderRule) (*api.ResourceOutput, error)
}

type OverrideHeaderRule struct {
	Name     string
	FromHost string
	ToHost   string

	OverridePages *OverridePagesRule
}

type OverridePagesRule struct {
	IndexPage    string
	NotFoundPage string
}

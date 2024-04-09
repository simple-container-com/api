package pulumi

import (
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

var NotConfiguredRegistrarError = errors.New("regisrar is not configured")

type notConfigured struct {
}

func (n notConfigured) MainDomain() string {
	return ""
}

func (n notConfigured) ProvisionRecords(ctx *sdk.Context, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	return nil, NotConfiguredRegistrarError
}

func (n notConfigured) NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error) {
	return nil, NotConfiguredRegistrarError
}

func (n notConfigured) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	return nil, NotConfiguredRegistrarError
}

func NotConfiguredRegistrar(ctx *sdk.Context, config api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error) {
	return &notConfigured{}, nil
}

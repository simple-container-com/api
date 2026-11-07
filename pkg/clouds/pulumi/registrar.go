// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// NotConfiguredRegistrarError is an alias; the sentinel moved to pApi so that
// subpackages this package imports (pulumi/vm) can match on it without a cycle. Kept
// here under its original name because it is exported and matched on by name.
var NotConfiguredRegistrarError = pApi.NotConfiguredRegistrarError

type notConfigured struct{}

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

func (n notConfigured) ProvisionDomainForEndpoint(ctx *sdk.Context, stack api.Stack, endpoint pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	return nil, NotConfiguredRegistrarError
}

func NotConfiguredRegistrar(ctx *sdk.Context, config api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error) {
	return &notConfigured{}, nil
}

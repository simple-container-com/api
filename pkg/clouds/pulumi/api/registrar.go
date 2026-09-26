// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

type Registrar interface {
	MainDomain() string
	ProvisionRecords(ctx *sdk.Context, params ProvisionParams) (*api.ResourceOutput, error)
	NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error)
	// NewOverrideHeaderRule overrides host header from one to another (only supported on certain providers)
	NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule OverrideHeaderRule) (*api.ResourceOutput, error)
	// ProvisionDomainForEndpoint makes a cloud endpoint answer on a custom domain,
	// including whatever edge the registrar's provider needs to get there.
	//
	// Callers must not assemble this themselves out of NewRecord and
	// NewOverrideHeaderRule: that pairing is a Cloudflare-shaped assumption. It works
	// because a proxied record terminates TLS and a Worker rewrites the Host header, so
	// the record can point straight at the endpoint and the edge is transparent. A
	// provider whose edge is a real resource has to create that resource first and point
	// the record at *it*, which is the opposite order.
	ProvisionDomainForEndpoint(ctx *sdk.Context, stack api.Stack, endpoint DomainEndpoint) (*api.ResourceOutput, error)
}

// DomainEndpoint describes a cloud endpoint that should answer on a custom domain.
type DomainEndpoint struct {
	// Name is a hint used to name the resources created, e.g. the lambda or container
	// name. It must be stable across deploys.
	Name   string
	Domain string
	// TargetHost is the hostname of the cloud endpoint, without scheme or path. It is
	// usually an Output, since the endpoint is typically created in the same deploy.
	TargetHost sdk.StringInput
}

type RegistrarWithWorkerScripts interface {
	NewWorkerScript(ctx *sdk.Context, workerName string, hostName string, script string) (*api.ResourceOutput, error)
}

type OverrideHeaderRule struct {
	Name       string
	FromHost   string
	ToHost     sdk.StringInput
	PathPrefix string

	BasicAuth     *BasicAuth
	OverridePages *OverridePagesRule
}

type BasicAuth struct {
	Username string
	Password string
	Realm    string
}

type OverridePagesRule struct {
	IndexPage    string
	NotFoundPage string
}

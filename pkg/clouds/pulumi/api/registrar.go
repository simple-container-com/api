// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

// NotConfiguredRegistrarError is what every method of the no-op registrar returns.
//
// A parent stack that declares no registrar still gets a Registrar — the empty type is
// registered to NotConfiguredRegistrar (pulumi/init.go), which hands back a non-nil
// value whose methods all fail with this. So a `params.Registrar == nil` check does NOT
// tell a caller "nobody will create DNS for me"; this error is the only thing that
// does, and a caller for whom hand-made DNS is a supported arrangement has to match on
// it rather than on nil.
//
// It lives beside the Registrar interface, not beside that no-op implementation, because
// the subpackages that have to match on it (pulumi/vm) are imported BY the pulumi
// package and cannot import it back.
var NotConfiguredRegistrarError = errors.New("registrar is not configured")

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
	// WebSocket asks the registrar's edge to terminate WebSocket connections for
	// this domain in addition to HTTP, when its provider has such an edge.
	//
	// A registrar whose edge is transparent (Cloudflare: a proxied record plus a
	// header-rewriting Worker) ignores this — a socket there already passes
	// through to the origin. It matters for a provider whose edge is a real
	// resource that has to be told: on Yandex Cloud an API Gateway only relays
	// frames for paths that declare the `x-yc-apigateway-websocket-*` operations,
	// and a socket is the only way a service behind a Serverless Container can
	// stream at all (see yandex.CloudExtras.WebSocket).
	WebSocket bool
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

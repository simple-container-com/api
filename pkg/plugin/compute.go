// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"github.com/pkg/errors"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// The plugin side of the ComputeProcess v2 RPC.
//
// SC core calls a plugin's ComputeProcess for every dependency resource that
// downstream compute (compose service, lambda, cloud-run) consumes. The
// plugin's job is to translate the resource's provisioning outputs into:
//
//   - **env vars** the consumer runs with (`MONGO_URI`, `DATABASE_URL`,
//     `S3_BUCKET`), optionally secret-flagged;
//   - **template extensions** — maps the consumer's config interpolation
//     reads to resolve placeholders (`${resource:<name>.hostname}`,
//     `${dependency:<name>.<field>}`).
//
// What crosses the bus here is a strict subset of what the in-tree
// `pApi.ComputeContextCollector` supports. The extras — `AddDependency`
// (Pulumi `sdk.Resource` handles), `AddOutput` (`sdk.Output`), and the
// pre/post-processors (Go closures) — cannot be serialised. A plugin that
// needs them applies them against its own `sdk.Context` before Build; the
// collector mutations crossing the wire are env vars + tpl extensions
// only.
//
//	resp, err := plugin.NewComputeResult().
//	    SecretEnvVar("MONGO_URI", mongoUri, resType, resName, stackName).
//	    EnvVar("MONGO_DB", dbName, resType, resName, stackName).
//	    ResourceTplExtension(resName, map[string]string{
//	        "connectionString": connStr,
//	        "database":         dbName,
//	    }).
//	    Build()

// ComputeResultBuilder accumulates the env + tpl-extension mutations a
// ComputeProcess handler wants SC core to apply back into the live
// `pApi.ComputeContextCollector`. Build validates and returns the proto
// response, which the sidecar unpacks and dispatches.
type ComputeResultBuilder struct {
	resp *scpluginv1.ComputeProcessResponse
	err  error
}

// NewComputeResult starts a response. Pass the URN of the resource the
// handler registered (if any) via Urn below; otherwise the response's
// `urn` field stays empty, same as a v1 compute processor.
func NewComputeResult() *ComputeResultBuilder {
	return &ComputeResultBuilder{resp: &scpluginv1.ComputeProcessResponse{}}
}

// fail records the first error and keeps the chain going, so a handler
// does not have to interleave error checks with the description of a
// result.
func (b *ComputeResultBuilder) fail(err error) *ComputeResultBuilder {
	if b.err == nil {
		b.err = err
	}
	return b
}

// Urn names the resource the compute processor is reporting against,
// if any. The sidecar echoes this back in the ResourceOutput ref so
// downstream config interpolation can key against the right URN.
func (b *ComputeResultBuilder) Urn(urn string) *ComputeResultBuilder {
	b.resp.Urn = urn
	return b
}

// EnvVar queues an env var to inject into downstream compute.
//
// resType/resName/stackName are the metadata pApi.ComputeEnvVariable
// records: the consuming compose service / lambda / cloud-run reads the
// var only when it is scoped to the stack it is deploying into. Getting
// one of these wrong silently sends the var to the wrong consumer.
func (b *ComputeResultBuilder) EnvVar(name, value, resType, resName, stackName string) *ComputeResultBuilder {
	if name == "" {
		return b.fail(errors.New("env var with no name"))
	}
	b.resp.EnvVars = append(b.resp.EnvVars, &scpluginv1.ComputeEnvVariable{
		Name:         name,
		Value:        value,
		Secret:       false,
		ResourceName: resName,
		ResourceType: resType,
		StackName:    stackName,
	})
	return b
}

// SecretEnvVar is EnvVar for a value that must not reach a log or state
// file in plain text. Core wraps it in pulumi.ToSecret before injection.
func (b *ComputeResultBuilder) SecretEnvVar(name, value, resType, resName, stackName string) *ComputeResultBuilder {
	if name == "" {
		return b.fail(errors.New("secret env var with no name"))
	}
	b.resp.EnvVars = append(b.resp.EnvVars, &scpluginv1.ComputeEnvVariable{
		Name:         name,
		Value:        value,
		Secret:       true,
		ResourceName: resName,
		ResourceType: resType,
		StackName:    stackName,
	})
	return b
}

// ResourceTplExtension queues values the consuming stack can interpolate
// via `${resource:<resName>.<key>}`. Mirrors
// pApi.ComputeContextCollector.AddResourceTplExtension.
func (b *ComputeResultBuilder) ResourceTplExtension(resName string, values map[string]string) *ComputeResultBuilder {
	if resName == "" {
		return b.fail(errors.New("resource tpl extension with no resource name"))
	}
	b.resp.TplExtensions = append(b.resp.TplExtensions, &scpluginv1.TemplateExtension{
		Kind:    scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_RESOURCE,
		ResName: resName,
		Values:  values,
	})
	return b
}

// DependencyTplExtension queues values the consuming stack can
// interpolate via `${dependency:<depName>.<resName>.<key>}`. Mirrors
// pApi.ComputeContextCollector.AddDependencyTplExtension.
func (b *ComputeResultBuilder) DependencyTplExtension(depName, resName string, values map[string]string) *ComputeResultBuilder {
	if depName == "" || resName == "" {
		return b.fail(errors.New("dependency tpl extension needs both a dependency name and a resource name"))
	}
	b.resp.TplExtensions = append(b.resp.TplExtensions, &scpluginv1.TemplateExtension{
		Kind:    scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_DEPENDENCY,
		DepName: depName,
		ResName: resName,
		Values:  values,
	})
	return b
}

// Build returns the response, or the first error the chain hit.
func (b *ComputeResultBuilder) Build() (*scpluginv1.ComputeProcessResponse, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.resp, nil
}

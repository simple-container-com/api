// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"google.golang.org/grpc"

	. "github.com/onsi/gomega"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// stubComputeProcessClient captures the ComputeProcess request so the test
// can assert what crossed the plugin bus. The response is canned so the
// test exercises the SIDECAR's response-handling path: unpack env vars +
// tpl extensions and dispatch them back into the live
// pApi.ComputeContextCollector.
type stubComputeProcessClient struct {
	scpluginv1.SCCloudPluginClient
	gotReq *scpluginv1.ComputeProcessRequest
	resp   *scpluginv1.ComputeProcessResponse
}

func (s *stubComputeProcessClient) ComputeProcess(
	_ context.Context, req *scpluginv1.ComputeProcessRequest, _ ...grpc.CallOption,
) (*scpluginv1.ComputeProcessResponse, error) {
	s.gotReq = req
	return s.resp, nil
}

// capturingCollector is a pApi.ComputeContextCollector that records every
// Add* call. The sidecar is supposed to apply the plugin's response here;
// if it drops env vars or tpl extensions, this test fails on the record.
type capturingCollector struct {
	mu            sync.Mutex
	envVars       []pApi.ComputeEnvVariable
	resourceTpl   map[string]map[string]string
	dependencyTpl map[string]map[string]map[string]string
}

func newCapturingCollector() *capturingCollector {
	return &capturingCollector{
		resourceTpl:   map[string]map[string]string{},
		dependencyTpl: map[string]map[string]map[string]string{},
	}
}

func (c *capturingCollector) SecretEnvVariables() []pApi.ComputeEnvVariable { return nil }
func (c *capturingCollector) EnvVariables() []pApi.ComputeEnvVariable       { return nil }

func (c *capturingCollector) AddEnvVariableIfNotExist(name, value, resType, resName, stackName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envVars = append(c.envVars, pApi.ComputeEnvVariable{
		Name: name, Value: value, ResourceType: resType, ResourceName: resName, StackName: stackName, Secret: false,
	})
}

func (c *capturingCollector) AddSecretEnvVariableIfNotExist(name, value, resType, resName, stackName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envVars = append(c.envVars, pApi.ComputeEnvVariable{
		Name: name, Value: value, ResourceType: resType, ResourceName: resName, StackName: stackName, Secret: true,
	})
}

func (c *capturingCollector) AddDependency(sdk.Resource)                       {}
func (c *capturingCollector) Dependencies() []sdk.Resource                     { return nil }
func (c *capturingCollector) AddOutput(_ *sdk.Context, _ sdk.Output)           {}
func (c *capturingCollector) Outputs() []sdk.Output                            { return nil }
func (c *capturingCollector) ResolvePlaceholders(any) error                    { return nil }
func (c *capturingCollector) AddPreProcessor(any, pApi.PreProcessor)           {}
func (c *capturingCollector) AddPostProcessor(any, pApi.PostProcessor)         {}
func (c *capturingCollector) GetPreProcessors(any) ([]pApi.PreProcessor, bool) { return nil, false }

func (c *capturingCollector) GetPostProcessors(any) ([]pApi.PostProcessor, bool) { return nil, false }
func (c *capturingCollector) RunPreProcessors(any, any) error                    { return nil }
func (c *capturingCollector) RunPostProcessors(any, any) error                   { return nil }
func (c *capturingCollector) ResourceOutputs() pApi.ResourcesOutputs             { return nil }

func (c *capturingCollector) AddResourceTplExtension(resName string, values map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resourceTpl[resName] = values
}

func (c *capturingCollector) AddDependencyTplExtension(depName, resName string, values map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dependencyTpl[depName] == nil {
		c.dependencyTpl[depName] = map[string]map[string]string{}
	}
	c.dependencyTpl[depName][resName] = values
}

// TestComputeProcessorSidecar_PacksParentInfoAndAppliesResponse — the
// end-to-end contract for ComputeProcess v2:
//
//   - The sidecar packs the request's new parent_info_json from
//     params.ParentStack (JSON of pApi.ParentInfo), so the plugin can
//     open sdk.NewStackReference against the right parent.
//   - The sidecar applies the response's env_vars + tpl_extensions back
//     into the live collector, with per-var secrecy preserved and tpl
//     extensions routed to the right AddResource/AddDependency method.
//
// This is what mongodb's ClusterComputeProcessor needs to round-trip,
// and what every subsequent extraction (aws/gcp/k8s) will reuse.
func TestComputeProcessorSidecar_PacksParentInfoAndAppliesResponse(t *testing.T) {
	RegisterTestingT(t)
	withActiveSession(t)

	stub := &stubComputeProcessClient{
		resp: &scpluginv1.ComputeProcessResponse{
			Urn: "urn:pulumi:test::proj::mongodbatlas:index/cluster:Cluster::primary",
			EnvVars: []*scpluginv1.ComputeEnvVariable{
				{Name: "MONGO_URI", Value: "mongodb+srv://...", Secret: true, ResourceName: "primary", ResourceType: "mongodb-atlas", StackName: "backend-prod"},
				{Name: "MONGO_DB", Value: "mydb", Secret: false, ResourceName: "primary", ResourceType: "mongodb-atlas", StackName: "backend-prod"},
			},
			TplExtensions: []*scpluginv1.TemplateExtension{
				{Kind: scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_RESOURCE, ResName: "primary", Values: map[string]string{"connectionString": "mongodb+srv://..."}},
				{Kind: scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_DEPENDENCY, DepName: "backend", ResName: "primary", Values: map[string]string{"env": "prod"}},
			},
		},
	}
	sidecar := (&preEnginePluginBundle{}).makeComputeProcessorSidecar(
		scPlugin.NewClientWithStub(stub), "sc-plugin-mongodb", "mongodb-atlas",
	)

	collector := newCapturingCollector()
	parentInfo := &pApi.ParentInfo{
		StackName:     "parent",
		ParentEnv:     "staging",
		StackEnv:      "prod",
		ResourceEnv:   "prod",
		FullReference: "org/parent/prod",
		UsesResource:  true,
		DependsOnResource: &api.StackConfigDependencyResource{
			Name:     "primary",
			Owner:    "parent-stack",
			Resource: "mongodb-atlas",
		},
	}

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		out, err := sidecar(
			ctx,
			api.Stack{Name: "backend"},
			api.ResourceInput{
				Descriptor:  &api.ResourceDescriptor{Type: "mongodb-atlas", Name: "primary"},
				StackParams: &api.StackParams{Environment: "prod"},
			},
			collector,
			pApi.ProvisionParams{ParentStack: parentInfo},
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).ToNot(BeNil())
		return nil
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).ToNot(HaveOccurred())

	// The request carries parent_info_json, round-trips through JSON back
	// to the Go struct the plugin would unmarshal.
	Expect(stub.gotReq).ToNot(BeNil())
	Expect(stub.gotReq.GetParentInfoJson()).ToNot(BeEmpty(), "parent info must cross the bus — this is the mongodb-blocking regression")
	var decoded pApi.ParentInfo
	Expect(json.Unmarshal(stub.gotReq.GetParentInfoJson(), &decoded)).To(Succeed())
	Expect(decoded.FullReference).To(Equal("org/parent/prod"))
	Expect(decoded.DependsOnResource).ToNot(BeNil())
	Expect(decoded.DependsOnResource.Name).To(Equal("primary"))

	// The response's env vars landed in the live collector, with the
	// secret flag preserved per entry.
	Expect(collector.envVars).To(HaveLen(2))
	Expect(collector.envVars[0].Name).To(Equal("MONGO_URI"))
	Expect(collector.envVars[0].Secret).To(BeTrue())
	Expect(collector.envVars[0].StackName).To(Equal("backend-prod"))
	Expect(collector.envVars[1].Name).To(Equal("MONGO_DB"))
	Expect(collector.envVars[1].Secret).To(BeFalse())

	// The response's tpl extensions landed under their respective keys.
	Expect(collector.resourceTpl).To(HaveKey("primary"))
	Expect(collector.resourceTpl["primary"]).To(HaveKeyWithValue("connectionString", "mongodb+srv://..."))
	Expect(collector.dependencyTpl).To(HaveKey("backend"))
	Expect(collector.dependencyTpl["backend"]).To(HaveKey("primary"))
	Expect(collector.dependencyTpl["backend"]["primary"]).To(HaveKeyWithValue("env", "prod"))
}

// TestComputeProcessorSidecar_HandlesNilParentStack — the sidecar must not
// panic on a resource with no parent (byo-shaped compute processors);
// parent_info_json just stays empty on the wire. Backwards-compat with
// v1 plugins is a function of this: old plugin reads empty bytes, same as
// today's behaviour.
func TestComputeProcessorSidecar_HandlesNilParentStack(t *testing.T) {
	RegisterTestingT(t)
	withActiveSession(t)

	stub := &stubComputeProcessClient{resp: &scpluginv1.ComputeProcessResponse{}}
	sidecar := (&preEnginePluginBundle{}).makeComputeProcessorSidecar(
		scPlugin.NewClientWithStub(stub), "sc-plugin-echo", "echo-bucket",
	)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := sidecar(
			ctx,
			api.Stack{Name: "backend"},
			api.ResourceInput{
				Descriptor:  &api.ResourceDescriptor{Type: "echo-bucket", Name: "bucket"},
				StackParams: &api.StackParams{Environment: "prod"},
			},
			newCapturingCollector(),
			pApi.ProvisionParams{ParentStack: nil},
		)
		return err
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).ToNot(HaveOccurred())
	Expect(stub.gotReq.GetParentInfoJson()).To(BeEmpty())
}

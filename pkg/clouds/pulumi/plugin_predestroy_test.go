// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// stubPreDestroyClient captures the PreDestroy request so the test can
// assert what crossed the plugin bus. The response is empty — PreDestroy
// is fire-and-forget on the wire, so there's nothing on the plugin side
// to assert except that the request packed the right payload.
type stubPreDestroyClient struct {
	scpluginv1.SCCloudPluginClient
	gotReq *scpluginv1.PreDestroyRequest
}

func (s *stubPreDestroyClient) PreDestroy(
	_ context.Context, req *scpluginv1.PreDestroyRequest, _ ...grpc.CallOption,
) (*scpluginv1.PreDestroyResponse, error) {
	s.gotReq = req
	return &scpluginv1.PreDestroyResponse{}, nil
}

// TestPreDestroySidecar_PacksStackOutputsClientDescriptorAndParams —
// the mongodb-shaped scenario: a cluster whose pre-destroy hook reads
// the deploying stack's `cloudExtras.mongoDbDestroyDatabase` flag off
// the per-env client descriptor, then enumerates outputs ending
// `-service-user` to drop each per-service DB.
//
// For the plugin extraction to carry that logic, the sidecar must pack
// all three onto the wire. This test asserts the request shape; a plugin
// then unmarshals back and runs the hook.
func TestPreDestroySidecar_PacksStackOutputsClientDescriptorAndParams(t *testing.T) {
	RegisterTestingT(t)

	stub := &stubPreDestroyClient{}
	sidecar := (&preEnginePluginBundle{}).makePreDestroySidecar(
		scPlugin.NewClientWithStub(stub), "sc-plugin-mongodb",
	)

	// A per-env client descriptor with `cloudExtras.mongoDbDestroyDatabase`
	// set. The hook on the plugin side reads this to decide whether to drop.
	envDesc := api.StackClientDescriptor{
		Type: "cloud-compose",
		Config: api.Config{
			Config: map[string]any{
				"cloudExtras": map[string]any{
					"mongoDbDestroyDatabase": true,
				},
			},
		},
	}
	stack := api.Stack{
		Name: "backend",
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"prod": envDesc,
			},
		},
	}
	params := api.DestroyParams{
		StackParams: api.StackParams{
			StackName:   "backend",
			Environment: "prod",
		},
		DestroySecretsStack: false,
	}

	// We can't easily construct a real auto.Stack in a unit test —
	// stackSource.Outputs() is where mongodb reads the service-user
	// outputs from. Pass the zero value; Outputs() will error, the
	// sidecar logs a warning, and stack_outputs lands empty. The other
	// two fields still get packed, which is what this test is about.
	sidecar(context.Background(), stack, params, auto.Stack{}, logger.New())

	Expect(stub.gotReq).ToNot(BeNil())
	Expect(stub.gotReq.GetHeader().GetStackName()).To(Equal("backend"))
	Expect(stub.gotReq.GetHeader().GetEnvironment()).To(Equal("prod"))

	// The per-env client descriptor round-trips through JSON.
	Expect(stub.gotReq.GetClientDescriptorJson()).ToNot(BeEmpty(), "client descriptor must cross — mongodb reads cloudExtras off it")
	var decodedDesc api.StackClientDescriptor
	Expect(json.Unmarshal(stub.gotReq.GetClientDescriptorJson(), &decodedDesc)).To(Succeed())
	Expect(decodedDesc.Type).To(Equal("cloud-compose"))

	// DestroyParams round-trips.
	Expect(stub.gotReq.GetDestroyParamsJson()).ToNot(BeEmpty())
	var decodedParams api.DestroyParams
	Expect(json.Unmarshal(stub.gotReq.GetDestroyParamsJson(), &decodedParams)).To(Succeed())
	Expect(decodedParams.Environment).To(Equal("prod"))
	Expect(decodedParams.StackName).To(Equal("backend"))
}

// TestPreDestroySidecar_HandlesMissingEnvDescriptor — the sidecar must
// not panic when the deploying stack has no per-env entry for params.Environment
// (operator typo, or deploy against an env that never had a config).
// client_descriptor_json just stays empty on the wire; the plugin treats
// absent as "nothing to do".
func TestPreDestroySidecar_HandlesMissingEnvDescriptor(t *testing.T) {
	RegisterTestingT(t)

	stub := &stubPreDestroyClient{}
	sidecar := (&preEnginePluginBundle{}).makePreDestroySidecar(
		scPlugin.NewClientWithStub(stub), "sc-plugin-mongodb",
	)

	stack := api.Stack{
		Name: "backend",
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"staging": {Type: "cloud-compose"},
			},
		},
	}
	params := api.DestroyParams{
		StackParams: api.StackParams{Environment: "prod"},
	}

	sidecar(context.Background(), stack, params, auto.Stack{}, logger.New())

	Expect(stub.gotReq).ToNot(BeNil())
	Expect(stub.gotReq.GetClientDescriptorJson()).To(BeEmpty(), "missing env must land as empty bytes, not a panic")
	// DestroyParams still packs even when the env lookup missed.
	Expect(stub.gotReq.GetDestroyParamsJson()).ToNot(BeEmpty())
}

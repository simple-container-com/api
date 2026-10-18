// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"context"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	gcpsdk "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type proxyMocks struct{}

func (proxyMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name + "_id", args.Inputs, nil
}

func (proxyMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// renderedProxyArgs runs the client-side sidecar path (createCloudsqlProxy) for a
// parent postgres config and returns the args of the proxy container it renders.
func renderedProxyArgs(t *testing.T, pgCfg *gcloud.PostgresGcpCloudsqlConfig) []string {
	t.Helper()
	var (
		mu   sync.Mutex
		args []string
	)
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		gcpProv, err := gcpsdk.NewProvider(ctx, "gcp", &gcpsdk.ProviderArgs{Project: pulumi.String("test-project")})
		if err != nil {
			return err
		}
		kubeProv, err := sdkK8s.NewProvider(ctx, "k8s", &sdkK8s.ProviderArgs{})
		if err != nil {
			return err
		}
		pgCfg.ProjectId = "test-project"
		pgCfg.Region = lo.ToPtr("europe-north1")
		input := createTestResourceInput("production", "telegram-bot", nil)
		input.Descriptor.Config.Config = pgCfg
		params := createTestAppendParams(input, nil)
		params.config = pgCfg
		params.gcpProvider = gcpProv
		params.kubeProvider = kubeProv
		params.collector = pApi.NewComputeContextCollector(context.Background(), logger.New(), "telegram-bot", "production")

		proxy, err := createCloudsqlProxy(ctx, params, pulumi.String("telegram-bot"))
		if err != nil {
			return err
		}
		// The output is typed Container but carries the ContainerArgs that
		// cloudsqlProxyContainer built, so take it untyped.
		proxy.ProxyContainer.ApplyT(func(v interface{}) error {
			c, ok := v.(corev1.ContainerArgs)
			if !ok {
				t.Errorf("unexpected proxy container value %T", v)
				return nil
			}
			mu.Lock()
			defer mu.Unlock()
			for _, a := range c.Args.(pulumi.StringArray) {
				args = append(args, string(a.(pulumi.String)))
			}
			return nil
		})
		return nil
	}, pulumi.WithMocks("project", "stack", proxyMocks{}))
	Expect(err).ToNot(HaveOccurred())
	mu.Lock()
	defer mu.Unlock()
	Expect(args).ToNot(BeEmpty(), "the proxy container was never rendered")
	return args
}

func TestCreateCloudsqlProxy_PrivateIpFollowsParentConfig(t *testing.T) {
	RegisterTestingT(t)
	network := lo.ToPtr("projects/p/global/networks/vpc")

	Expect(renderedProxyArgs(t, &gcloud.PostgresGcpCloudsqlConfig{})).
		ToNot(ContainElement("--private-ip"), "default stays on the public IP")
	Expect(renderedProxyArgs(t, &gcloud.PostgresGcpCloudsqlConfig{PrivateNetwork: network})).
		ToNot(ContainElement("--private-ip"), "privateNetwork alone must not move the proxy")
	Expect(renderedProxyArgs(t, &gcloud.PostgresGcpCloudsqlConfig{PrivateNetwork: network, ProxyPrivateIp: lo.ToPtr(true)})).
		To(ContainElement("--private-ip"), "proxyPrivateIp must reach the client sidecar")
	Expect(renderedProxyArgs(t, &gcloud.PostgresGcpCloudsqlConfig{PrivateNetwork: network, PublicIpEnabled: lo.ToPtr(false)})).
		To(ContainElement("--private-ip"), "with the public IP off the sidecar must dial private")
}

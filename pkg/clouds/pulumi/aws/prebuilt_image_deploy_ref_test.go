// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	clouds "github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type prebuiltImageMocks struct{}

func (*prebuiltImageMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name, args.Inputs, nil
}

func (*prebuiltImageMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// A compose service that carries a plain `image:` and no build section is never
// built here, so nothing produces a digest for it. The AST guard proves the
// task definition reads DeployImageRef; only running the code proves that field
// carries a value. A zero sdk.StringOutput resolves as unknown rather than
// panicking, so the failure is a task definition with no image at all -- and an
// unknown output never runs its apply, which is what makes this test fail by
// timing out rather than by comparing strings.
//
// This is the one arm of buildAndPushECSFargateImages that creates no
// resources, which is what lets it run under mocks with no cloud at all.
func TestPrebuiltImageContainerCarriesAKnownDeployRef(t *testing.T) {
	RegisterTestingT(t)

	const prebuilt = "public.ecr.aws/docker/library/busybox:1.37"

	crInput := &clouds.EcsFargateInput{
		Containers: []clouds.EcsFargateContainer{{
			Name:  "sidecar",
			Image: api.ContainerImage{Name: prebuilt},
		}},
	}
	ref := &EcsFargateOutput{}

	var deployRef, imageName string
	resolved := make(chan struct{})
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		if err := buildAndPushECSFargateImages(ctx, api.Stack{Name: "stack"}, pApi.ProvisionParams{},
			api.StackParams{Environment: "test", Version: "2026.09.17-0123abc"}, crInput, ref); err != nil {
			return err
		}
		sdk.All(ref.Images[0].DeployImageRef, ref.Images[0].ImageName).
			ApplyT(func(values []any) error {
				deployRef, _ = values[0].(string)
				imageName, _ = values[1].(string)
				close(resolved)
				return nil
			})
		return nil
	}, sdk.WithMocks("project", "stack", &prebuiltImageMocks{}))
	Expect(err).NotTo(HaveOccurred())

	select {
	case <-resolved:
	case <-time.After(10 * time.Second):
		t.Fatal("DeployImageRef never resolved: an unset sdk.StringOutput awaits as unknown, " +
			"which is exactly how a prebuilt image reaches the task definition with no reference")
	}

	Expect(deployRef).To(Equal(prebuilt))
	Expect(imageName).To(Equal(prebuilt))
}

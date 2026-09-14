// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	awsCfg "github.com/simple-container-com/api/pkg/clouds/aws"
	mongoCfg "github.com/simple-container-com/api/pkg/clouds/mongodb"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiAws "github.com/simple-container-com/api/pkg/clouds/pulumi/aws"
	pulumiMongo "github.com/simple-container-com/api/pkg/clouds/pulumi/mongodb"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
)

// Simple Container does not wrap the AWS or Atlas provider credentials,
// because those SDKs wrap their own: pulumi-aws secrets accessKey, secretKey
// and token, pulumi-mongodbatlas secrets privateKey. That is an assumption
// about a dependency, and dependencies get bumped.
//
// This pins it. If an upstream release stops marking one of these, the failure
// is this test rather than a credential in a customer's CI log, and the repair
// is to wrap the field at the call site the way the gcp and kubernetes
// providers already are.
func TestUpstreamProvidersStillSecretTheirOwnCredentials(t *testing.T) {
	RegisterTestingT(t)

	const (
		awsSecretKey    = "wJalrXUtnFEMIexampleKEY"
		atlasPrivateKey = "b8f9c0de-atlas-private-key"
	)

	mocks := testutil.NewRecordingMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		if _, err := pulumiAws.Provider(ctx, api.Stack{Name: "acme"}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name: "aws-auth",
				Config: api.Config{Config: &awsCfg.AccountConfig{
					Account:         "123456789012",
					AccessKey:       "AKIAIOSFODNN7EXAMPLE",
					SecretAccessKey: awsSecretKey,
					Region:          "us-east-1",
				}},
			},
			StackParams: &api.StackParams{Environment: "staging"},
		}, pApi.ProvisionParams{Log: logger.New()}); err != nil {
			return err
		}
		_, err := pulumiMongo.Provider(ctx, api.Stack{Name: "acme"}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Name: "atlas-auth",
				Config: api.Config{Config: &mongoCfg.AtlasConfig{
					PublicKey:  "atlas-public-key",
					PrivateKey: atlasPrivateKey,
					Region:     "US_EAST_1",
				}},
			},
			StackParams: &api.StackParams{Environment: "staging"},
		}, pApi.ProvisionParams{Log: logger.New()})
		return err
	}, sdk.WithMocks("acme", "staging", mocks))
	Expect(err).ToNot(HaveOccurred())

	awsInputs, ok := mocks.Inputs("pulumi:providers:aws", "aws-auth--staging")
	Expect(ok).To(BeTrue())
	Expect(awsInputs["secretKey"].IsSecret()).To(BeTrue(),
		"pulumi-aws no longer secrets secretKey: wrap it at the call site")
	Expect(awsInputs["accessKey"].IsSecret()).To(BeTrue(),
		"pulumi-aws no longer secrets accessKey: wrap it at the call site")

	atlasInputs, ok := mocks.Inputs("pulumi:providers:mongodbatlas", "atlas-auth--staging")
	Expect(ok).To(BeTrue())
	Expect(atlasInputs["privateKey"].IsSecret()).To(BeTrue(),
		"pulumi-mongodbatlas no longer secrets privateKey: wrap it at the call site")
}

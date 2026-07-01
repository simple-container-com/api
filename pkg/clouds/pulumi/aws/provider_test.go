// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	sdkAws "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/aws"
)

// TestInitStateStore_AmbientCredsPreserved is the regression guard for OIDC /
// instance-profile deploys: when the auth config carries NO static keys,
// InitStateStore must not touch the AWS_* env the runner already exported, so
// the AWS default credential chain (e.g. the GitHub OIDC web-identity creds)
// still works. The previous code always ran os.Setenv("AWS_SECRET_ACCESS_KEY",
// "") which blanked those ambient credentials.
func TestInitStateStore_AmbientCredsPreserved(t *testing.T) {
	RegisterTestingT(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "ASIAAMBIENT")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	t.Setenv("AWS_SESSION_TOKEN", "ambient-session")

	// Auth config with region only — no accessKey/secretAccessKey (OIDC).
	cfg := &aws.StateStorageConfig{AccountConfig: aws.AccountConfig{Region: "eu-central-1"}}
	Expect(InitStateStore(context.Background(), cfg, logger.New())).To(Succeed())

	Expect(os.Getenv("AWS_SECRET_ACCESS_KEY")).To(Equal("ambient-secret"))
	Expect(os.Getenv("AWS_ACCESS_KEY_ID")).To(Equal("ASIAAMBIENT"))
	Expect(os.Getenv("AWS_SESSION_TOKEN")).To(Equal("ambient-session"))
	Expect(os.Getenv("AWS_DEFAULT_REGION")).To(Equal("eu-central-1"))
}

// TestInitStateStore_StaticCredsExported confirms back-compat: a config WITH
// static keys still exports them unchanged.
func TestInitStateStore_StaticCredsExported(t *testing.T) {
	RegisterTestingT(t)
	t.Setenv("AWS_ACCESS_KEY", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	cfg := &aws.StateStorageConfig{AccountConfig: aws.AccountConfig{
		AccessKey:       "AKIASTATIC",
		SecretAccessKey: "static-secret",
		Region:          "us-east-1",
	}}
	Expect(InitStateStore(context.Background(), cfg, logger.New())).To(Succeed())

	Expect(os.Getenv("AWS_ACCESS_KEY")).To(Equal("AKIASTATIC"))
	Expect(os.Getenv("AWS_SECRET_ACCESS_KEY")).To(Equal("static-secret"))
	Expect(os.Getenv("AWS_DEFAULT_REGION")).To(Equal("us-east-1"))
}

// TestApplyAWSProviderCreds_Ambient is the regression guard for the testtmp
// failure: in ambient mode (empty static keys) the explicit provider must be
// left credential-less — so the AWS default chain resolves the runner's env
// creds at call time — with STS pre-validation skipped, so re-deploying a stack
// that previously baked static keys doesn't fail with "Invalid credentials
// configured". Crucially it must NOT bake the (rotating) env creds into inputs.
func TestApplyAWSProviderCreds_Ambient(t *testing.T) {
	RegisterTestingT(t)
	// Even with ambient creds in the env, they must not be copied onto the args.
	t.Setenv("AWS_ACCESS_KEY_ID", "ASIAAMBIENT")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	t.Setenv("AWS_SESSION_TOKEN", "ambient-session")

	args := &sdkAws.ProviderArgs{Region: sdk.String("eu-central-1")}
	applyAWSProviderCreds(args, "", "")

	Expect(args.AccessKey).To(BeNil(), "must not bake ambient creds into provider inputs")
	Expect(args.SecretKey).To(BeNil())
	Expect(args.Token).To(BeNil(), "must not persist the rotating session token in state")
	skip, ok := args.SkipCredentialsValidation.(sdk.Bool)
	Expect(ok).To(BeTrue(), "ambient mode must skip the eager STS pre-validation")
	Expect(bool(skip)).To(BeTrue())
}

// TestApplyAWSProviderCreds_Static confirms static keys are pinned and the
// pre-validation stays ON (it catches bad static keys early).
func TestApplyAWSProviderCreds_Static(t *testing.T) {
	RegisterTestingT(t)
	args := &sdkAws.ProviderArgs{Region: sdk.String("us-east-1")}
	applyAWSProviderCreds(args, "AKIASTATIC", "static-secret")

	ak, _ := args.AccessKey.(sdk.String)
	Expect(string(ak)).To(Equal("AKIASTATIC"))
	sk, _ := args.SecretKey.(sdk.String)
	Expect(string(sk)).To(Equal("static-secret"))
	Expect(args.Token).To(BeNil())
	Expect(args.SkipCredentialsValidation).To(BeNil(), "static keys keep validation on")
}

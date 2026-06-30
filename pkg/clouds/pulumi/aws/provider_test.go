// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"

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

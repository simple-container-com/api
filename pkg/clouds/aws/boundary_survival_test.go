// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const testBoundaryARN = "arn:aws:iam::123456789012:policy/my-workload-boundary"

// authBlob is a resolved ${auth:...} credentials blob WITHOUT permissionsBoundary.
// Using it (rather than an empty Credentials.Credentials) is what makes these
// tests load-bearing: with empty creds, CredentialsValue marshals the whole
// struct so the boundary rides along even if the preserve logic is deleted.
const authBlob = `{"account":"123456789012","region":"us-east-1"}`

// KeepBoundary centralizes the "template wins, else keep what ConvertAuth
// loaded" precedence. Pin both directions.
func TestAccountConfig_KeepBoundary(t *testing.T) {
	RegisterTestingT(t)

	// A non-empty template value overrides whatever is already set.
	ac := AccountConfig{PermissionsBoundary: "auth-level-arn"}
	ac.KeepBoundary("template-level-arn")
	Expect(ac.PermissionsBoundary).To(Equal("template-level-arn"))

	// An empty template value leaves the existing (auth-level) value intact.
	ac2 := AccountConfig{PermissionsBoundary: "auth-level-arn"}
	ac2.KeepBoundary("")
	Expect(ac2.PermissionsBoundary).To(Equal("auth-level-arn"))
}

// The in-place ConvertAuth path used by the ECS/Lambda pulumi constructors:
// a template-level boundary must survive the credential rehydrate.
func TestConvertAuth_InPlacePreservesTemplateBoundary(t *testing.T) {
	RegisterTestingT(t)
	ac := AccountConfig{
		PermissionsBoundary: testBoundaryARN,
		Credentials:         api.Credentials{Credentials: authBlob},
	}
	tpl := ac.PermissionsBoundary
	Expect(api.ConvertAuth(&ac, &ac)).To(Succeed())
	ac.KeepBoundary(tpl)
	Expect(ac.PermissionsBoundary).To(Equal(testBoundaryARN)) // survived
	Expect(ac.Account).To(Equal("123456789012"))              // creds still loaded
}

// The fresh-struct converter path (ToAwsLambdaConfig): boundary must reach the
// resulting *LambdaInput for both template-level and auth-level declaration,
// and stay empty when unset.
func TestToAwsLambdaConfig_PermissionsBoundary(t *testing.T) {
	t.Run("template-level survives ConvertAuth", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := &TemplateConfig{AccountConfig: AccountConfig{
			PermissionsBoundary: testBoundaryARN,
			Credentials:         api.Credentials{Credentials: authBlob},
		}}
		out, err := ToAwsLambdaConfig(tpl, &api.StackConfigSingleImage{})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.(*LambdaInput).PermissionsBoundary).To(Equal(testBoundaryARN))
	})

	t.Run("auth-level survives ConvertAuth", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := &TemplateConfig{AccountConfig: AccountConfig{
			Credentials: api.Credentials{Credentials: `{"account":"123456789012","region":"us-east-1","permissionsBoundary":"` + testBoundaryARN + `"}`},
		}}
		out, err := ToAwsLambdaConfig(tpl, &api.StackConfigSingleImage{})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.(*LambdaInput).PermissionsBoundary).To(Equal(testBoundaryARN))
	})

	t.Run("unset stays empty (no-op contract)", func(t *testing.T) {
		RegisterTestingT(t)
		out, err := ToAwsLambdaConfig(validTemplateConfig(), &api.StackConfigSingleImage{})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.(*LambdaInput).PermissionsBoundary).To(BeEmpty())
	})
}

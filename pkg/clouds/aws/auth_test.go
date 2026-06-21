// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// ---- AccountConfig getters ----------------------------------------------

func TestAccountConfig_ProviderType(t *testing.T) {
	RegisterTestingT(t)
	ac := &AccountConfig{Account: "123456789012"}
	Expect(ac.ProviderType()).To(Equal(ProviderType))
	Expect(ac.ProviderType()).To(Equal("aws"))
}

func TestAccountConfig_ProjectIdValue(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name    string
		account string
		want    string
	}{
		{name: "populated account", account: "123456789012", want: "123456789012"},
		{name: "empty account", account: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			ac := &AccountConfig{Account: tc.account}
			Expect(ac.ProjectIdValue()).To(Equal(tc.want))
		})
	}
}

func TestAccountConfig_CredentialsValue(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty Credentials.Credentials falls back to JSON of the struct", func(t *testing.T) {
		RegisterTestingT(t)
		ac := &AccountConfig{
			Account:         "123456789012",
			AccessKey:       "AKIAEXAMPLE",
			SecretAccessKey: "secret",
			Region:          "us-east-1",
		}
		got := ac.CredentialsValue()
		// Fallback path returns api.AuthToString(ac) which is a JSON object.
		Expect(got).To(ContainSubstring(`"account":"123456789012"`))
		Expect(got).To(ContainSubstring(`"accessKey":"AKIAEXAMPLE"`))
		Expect(got).To(ContainSubstring(`"region":"us-east-1"`))

		// It must be valid JSON that round-trips back into an AccountConfig
		// (this is exactly what ConvertAuth relies on).
		var rt AccountConfig
		Expect(json.Unmarshal([]byte(got), &rt)).To(Succeed())
		Expect(rt.Account).To(Equal("123456789012"))
		Expect(rt.AccessKey).To(Equal("AKIAEXAMPLE"))
		Expect(rt.Region).To(Equal("us-east-1"))
	})

	t.Run("non-empty Credentials.Credentials is returned verbatim", func(t *testing.T) {
		RegisterTestingT(t)
		ac := &AccountConfig{
			Account:     "123456789012",
			Credentials: api.Credentials{Credentials: "raw-creds-blob"},
		}
		Expect(ac.CredentialsValue()).To(Equal("raw-creds-blob"))
	})
}

// ---- StateStorageConfig getters -----------------------------------------

func TestStateStorageConfig_StorageUrl(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name   string
		bucket string
		want   string
	}{
		{name: "named bucket", bucket: "my-state-bucket", want: "s3://my-state-bucket"},
		{name: "empty bucket", bucket: "", want: "s3://"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			ss := &StateStorageConfig{BucketName: tc.bucket}
			Expect(ss.StorageUrl()).To(Equal(tc.want))
		})
	}
}

func TestStateStorageConfig_IsProvisionEnabled(t *testing.T) {
	RegisterTestingT(t)
	Expect((&StateStorageConfig{Provision: true}).IsProvisionEnabled()).To(BeTrue())
	Expect((&StateStorageConfig{Provision: false}).IsProvisionEnabled()).To(BeFalse())
}

// ---- SecretsProviderConfig getters --------------------------------------

func TestSecretsProviderConfig_IsProvisionEnabled(t *testing.T) {
	RegisterTestingT(t)
	Expect((&SecretsProviderConfig{Provision: true}).IsProvisionEnabled()).To(BeTrue())
	Expect((&SecretsProviderConfig{Provision: false}).IsProvisionEnabled()).To(BeFalse())
}

func TestSecretsProviderConfig_KeyUrl(t *testing.T) {
	RegisterTestingT(t)
	key := "awskms://1234abcd-12ab-34cd-56ef-1234567890ab?region=us-east-1"
	Expect((&SecretsProviderConfig{KeyName: key}).KeyUrl()).To(Equal(key))
	Expect((&SecretsProviderConfig{}).KeyUrl()).To(Equal(""))
}

// ---- Read* config readers -----------------------------------------------

func TestReadAuthServiceAccountConfig(t *testing.T) {
	RegisterTestingT(t)
	cfg := &api.Config{Config: map[string]any{
		"account":         "123456789012",
		"accessKey":       "AKIAEXAMPLE",
		"secretAccessKey": "secret",
		"region":          "eu-central-1",
	}}
	out, err := ReadAuthServiceAccountConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	ac, ok := out.Config.(*AccountConfig)
	Expect(ok).To(BeTrue())
	Expect(ac.Account).To(Equal("123456789012"))
	Expect(ac.AccessKey).To(Equal("AKIAEXAMPLE"))
	Expect(ac.SecretAccessKey).To(Equal("secret"))
	Expect(ac.Region).To(Equal("eu-central-1"))
}

func TestReadSecretsConfig(t *testing.T) {
	RegisterTestingT(t)
	cfg := &api.Config{Config: map[string]any{
		"account": "123456789012",
		"region":  "us-west-2",
	}}
	out, err := ReadSecretsConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	sc, ok := out.Config.(*SecretsConfig)
	Expect(ok).To(BeTrue())
	Expect(sc.Account).To(Equal("123456789012"))
	Expect(sc.Region).To(Equal("us-west-2"))
}

func TestReadStateStorageConfig(t *testing.T) {
	RegisterTestingT(t)
	cfg := &api.Config{Config: map[string]any{
		"account":    "123456789012",
		"region":     "us-west-2",
		"bucketName": "sc-state",
		"provision":  true,
	}}
	out, err := ReadStateStorageConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	ss, ok := out.Config.(*StateStorageConfig)
	Expect(ok).To(BeTrue())
	Expect(ss.BucketName).To(Equal("sc-state"))
	Expect(ss.IsProvisionEnabled()).To(BeTrue())
	Expect(ss.StorageUrl()).To(Equal("s3://sc-state"))

	// Interface conformance: StateStorageConfig must satisfy api.StateStorageConfig.
	var _ api.StateStorageConfig = ss
}

func TestReadSecretsProviderConfig(t *testing.T) {
	RegisterTestingT(t)
	key := "awskms://1234abcd-12ab-34cd-56ef-1234567890ab?region=us-east-1"
	cfg := &api.Config{Config: map[string]any{
		"account":   "123456789012",
		"region":    "us-east-1",
		"provision": false,
		"keyName":   key,
	}}
	out, err := ReadSecretsProviderConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	sp, ok := out.Config.(*SecretsProviderConfig)
	Expect(ok).To(BeTrue())
	Expect(sp.KeyUrl()).To(Equal(key))
	Expect(sp.IsProvisionEnabled()).To(BeFalse())

	var _ api.SecretsProviderConfig = sp
}

func TestReadTemplateConfig(t *testing.T) {
	RegisterTestingT(t)
	cfg := &api.Config{Config: map[string]any{
		"account": "123456789012",
		"region":  "us-east-1",
	}}
	out, err := ReadTemplateConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	tc, ok := out.Config.(*TemplateConfig)
	Expect(ok).To(BeTrue())
	Expect(tc.Account).To(Equal("123456789012"))
	Expect(tc.Region).To(Equal("us-east-1"))
}

func TestS3BucketReadConfig(t *testing.T) {
	RegisterTestingT(t)
	cfg := &api.Config{Config: map[string]any{
		"account":        "123456789012",
		"name":           "my-bucket",
		"allowOnlyHttps": true,
	}}
	out, err := S3BucketReadConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	b, ok := out.Config.(*S3Bucket)
	Expect(ok).To(BeTrue())
	Expect(b.Name).To(Equal("my-bucket"))
	Expect(b.AllowOnlyHttps).To(BeTrue())
	Expect(b.Account).To(Equal("123456789012"))
}

func TestEcrRepositoryReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("with explicit lifecycle policy", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"account": "123456789012",
			"name":    "my-repo",
			"lifecyclePolicy": map[string]any{
				"rules": []any{
					map[string]any{
						"rulePriority": 1,
						"description":  "keep 5",
						"selection": map[string]any{
							"tagStatus":   "any",
							"countType":   "imageCountMoreThan",
							"countNumber": 5,
						},
						"action": map[string]any{"type": "expire"},
					},
				},
			},
		}}
		out, err := EcrRepositoryReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		repo, ok := out.Config.(*EcrRepository)
		Expect(ok).To(BeTrue())
		Expect(repo.Name).To(Equal("my-repo"))
		Expect(repo.LifecyclePolicy).ToNot(BeNil())
		Expect(repo.LifecyclePolicy.Rules).To(HaveLen(1))
		Expect(repo.LifecyclePolicy.Rules[0].RulePriority).To(Equal(1))
		Expect(repo.LifecyclePolicy.Rules[0].Selection.CountNumber).To(Equal(5))
		Expect(repo.LifecyclePolicy.Rules[0].Action.Type).To(Equal("expire"))
	})

	t.Run("without lifecycle policy leaves it nil", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"account": "123456789012",
			"name":    "my-repo",
		}}
		out, err := EcrRepositoryReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		repo := out.Config.(*EcrRepository)
		Expect(repo.LifecyclePolicy).To(BeNil())
	})
}

// DefaultEcrLifecyclePolicy is a package-level default; assert its documented shape.
func TestDefaultEcrLifecyclePolicy(t *testing.T) {
	RegisterTestingT(t)
	Expect(DefaultEcrLifecyclePolicy.Rules).To(HaveLen(1))
	rule := DefaultEcrLifecyclePolicy.Rules[0]
	Expect(rule.RulePriority).To(Equal(1))
	Expect(rule.Description).To(Equal("Keep only 3 last images"))
	Expect(rule.Selection.TagStatus).To(Equal("any"))
	Expect(rule.Selection.CountType).To(Equal("imageCountMoreThan"))
	Expect(rule.Selection.CountNumber).To(Equal(3))
	Expect(rule.Action.Type).To(Equal("expire"))
}

// ---- RequiresTrailValidation --------------------------------------------

func TestCloudTrailSecurityAlertsConfig_RequiresTrailValidation(t *testing.T) {
	RegisterTestingT(t)
	tr := func(b bool) *bool { return &b }

	tests := []struct {
		name      string
		trailName string
		require   *bool
		want      bool
	}{
		{name: "empty trail name -> never validates", trailName: "", require: nil, want: false},
		{name: "empty trail name even when require=true -> false", trailName: "", require: tr(true), want: false},
		{name: "trail set, require unset -> default true", trailName: "my-trail", require: nil, want: true},
		{name: "trail set, require=true -> true", trailName: "my-trail", require: tr(true), want: true},
		{name: "trail set, require=false -> false (warn-only)", trailName: "my-trail", require: tr(false), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			c := CloudTrailSecurityAlertsConfig{
				TrailName:                tc.trailName,
				RequireLogFileValidation: tc.require,
			}
			Expect(c.RequiresTrailValidation()).To(Equal(tc.want))
		})
	}
}

// ---- init() registration ------------------------------------------------

// init() runs at package load and registers all AWS provider/field/converter
// readers into the api global maps. Assert the AWS resource types resolve.
func TestInitRegistration(t *testing.T) {
	RegisterTestingT(t)

	providers := api.GetRegisteredProviderConfigs()
	for _, key := range []string{
		SecretsTypeAWSSecretsManager,
		TemplateTypeEcsFargate,
		TemplateTypeAwsLambda,
		TemplateTypeStaticWebsite,
		AuthTypeAWSToken,
		ResourceTypeS3Bucket,
		ResourceTypeEcrRepository,
		ResourceTypeRdsPostgres,
		ResourceTypeRdsMysql,
		ResourceTypeCloudTrailSecurityAlerts,
	} {
		Expect(providers).To(HaveKey(key), "provider config %q must be registered by init()", key)
	}

	fields := api.GetRegisteredProvisionerFieldConfigs()
	Expect(fields).To(HaveKey(StateStorageTypeS3Bucket))
	Expect(fields).To(HaveKey(SecretsProviderTypeAwsKms))
}

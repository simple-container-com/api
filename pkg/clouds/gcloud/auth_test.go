// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// ---- Credentials getters -------------------------------------------------

func TestCredentials_ProviderType(t *testing.T) {
	RegisterTestingT(t)
	c := &Credentials{}
	Expect(c.ProviderType()).To(Equal(ProviderType))
	Expect(c.ProviderType()).To(Equal("gcp"))
}

func TestCredentials_ProjectIdValue(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name      string
		projectId string
		want      string
	}{
		{name: "populated project id", projectId: "my-gcp-project", want: "my-gcp-project"},
		{name: "empty project id", projectId: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			c := &Credentials{
				ServiceAccountConfig: ServiceAccountConfig{ProjectId: tc.projectId},
			}
			Expect(c.ProjectIdValue()).To(Equal(tc.want))
		})
	}
}

func TestCredentials_CredentialsValue(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name  string
		creds string
		want  string
	}{
		{name: "serialized gcp account json", creds: `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com"}`, want: `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com"}`},
		{name: "empty credentials", creds: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			c := &Credentials{
				Credentials: api.Credentials{Credentials: tc.creds},
			}
			Expect(c.CredentialsValue()).To(Equal(tc.want))
		})
	}
}

func TestCredentials_CredentialsParsed(t *testing.T) {
	RegisterTestingT(t)

	t.Run("valid service account json parses type and client_email", func(t *testing.T) {
		RegisterTestingT(t)
		c := &Credentials{
			Credentials: api.Credentials{Credentials: `{"type":"service_account","client_email":"sa@proj.iam.gserviceaccount.com","extra":"ignored"}`},
		}
		parsed, err := c.CredentialsParsed()
		Expect(err).ToNot(HaveOccurred())
		Expect(parsed).ToNot(BeNil())
		Expect(parsed.Type).To(Equal("service_account"))
		Expect(parsed.ClientEmail).To(Equal("sa@proj.iam.gserviceaccount.com"))
	})

	t.Run("invalid json returns an error", func(t *testing.T) {
		RegisterTestingT(t)
		c := &Credentials{
			Credentials: api.Credentials{Credentials: "this-is-not-json"},
		}
		parsed, err := c.CredentialsParsed()
		Expect(err).To(HaveOccurred())
		Expect(parsed).To(BeNil())
	})

	t.Run("empty credentials string returns an error", func(t *testing.T) {
		RegisterTestingT(t)
		c := &Credentials{Credentials: api.Credentials{Credentials: ""}}
		_, err := c.CredentialsParsed()
		Expect(err).To(HaveOccurred())
	})

	// Interface conformance: Credentials must satisfy api.AuthConfig.
	var _ api.AuthConfig = &Credentials{}
}

// ---- StateStorageConfig getters -----------------------------------------

func TestStateStorageConfig_GetBucketName(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name       string
		bucketName string
		nameField  string
		want       string
	}{
		{name: "bucketName takes precedence", bucketName: "bucket-a", nameField: "name-b", want: "bucket-a"},
		{name: "falls back to name when bucketName empty", bucketName: "", nameField: "name-b", want: "name-b"},
		{name: "both empty yields empty", bucketName: "", nameField: "", want: ""},
		{name: "only bucketName set", bucketName: "bucket-a", nameField: "", want: "bucket-a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := &StateStorageConfig{BucketName: tc.bucketName, Name: tc.nameField}
			Expect(s.GetBucketName()).To(Equal(tc.want))
		})
	}
}

func TestStateStorageConfig_StorageUrl(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name       string
		bucketName string
		nameField  string
		want       string
	}{
		{name: "uses bucketName", bucketName: "my-state", nameField: "", want: "gs://my-state"},
		{name: "uses name fallback", bucketName: "", nameField: "fallback-state", want: "gs://fallback-state"},
		{name: "empty bucket", bucketName: "", nameField: "", want: "gs://"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := &StateStorageConfig{BucketName: tc.bucketName, Name: tc.nameField}
			Expect(s.StorageUrl()).To(Equal(tc.want))
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
	key := "gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/k"
	Expect((&SecretsProviderConfig{KeyName: key}).KeyUrl()).To(Equal(key))
	Expect((&SecretsProviderConfig{}).KeyUrl()).To(Equal(""))
}

// ---- Read* config readers -----------------------------------------------

func TestReadAuthServiceAccountConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps projectId and credentials", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":   "my-gcp-project",
			"credentials": `{"type":"service_account"}`,
		}}
		out, err := ReadAuthServiceAccountConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		c, ok := out.Config.(*Credentials)
		Expect(ok).To(BeTrue())
		Expect(c.ProjectId).To(Equal("my-gcp-project"))
		Expect(c.CredentialsValue()).To(Equal(`{"type":"service_account"}`))

		var _ api.AuthConfig = c
	})

	t.Run("error path: wrong-typed field surfaces conversion error", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId": []string{"not", "a", "string"},
		}}
		_, err := ReadAuthServiceAccountConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

func TestReadStateStorageConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":  "my-gcp-project",
			"bucketName": "sc-state",
			"location":   "EU",
			"provision":  true,
		}}
		out, err := ReadStateStorageConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		ss, ok := out.Config.(*StateStorageConfig)
		Expect(ok).To(BeTrue())
		Expect(ss.GetBucketName()).To(Equal("sc-state"))
		Expect(ss.IsProvisionEnabled()).To(BeTrue())
		Expect(ss.StorageUrl()).To(Equal("gs://sc-state"))
		Expect(ss.Location).ToNot(BeNil())
		Expect(*ss.Location).To(Equal("EU"))

		var _ api.StateStorageConfig = ss
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"provision": "not-a-bool-but-a-map",
			"bucketName": map[string]any{
				"nested": "value",
			},
		}}
		_, err := ReadStateStorageConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

func TestReadSecretsProviderConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		key := "gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/k"
		cfg := &api.Config{Config: map[string]any{
			"projectId":         "my-gcp-project",
			"keyName":           key,
			"keyLocation":       "global",
			"keyRotationPeriod": "7776000s",
			"provision":         false,
		}}
		out, err := ReadSecretsProviderConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		sp, ok := out.Config.(*SecretsProviderConfig)
		Expect(ok).To(BeTrue())
		Expect(sp.KeyUrl()).To(Equal(key))
		Expect(sp.KeyLocation).To(Equal("global"))
		Expect(sp.KeyRotationPeriod).To(Equal("7776000s"))
		Expect(sp.IsProvisionEnabled()).To(BeFalse())

		var _ api.SecretsProviderConfig = sp
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"keyName": []int{1, 2, 3},
		}}
		_, err := ReadSecretsProviderConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

// CredentialsValue round-trips through json (used by api.ConvertAuth).
func TestCredentials_RoundTripJSON(t *testing.T) {
	RegisterTestingT(t)
	raw := `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com"}`
	c := &Credentials{Credentials: api.Credentials{Credentials: raw}}
	var parsed CredentialsParsed
	Expect(json.Unmarshal([]byte(c.CredentialsValue()), &parsed)).To(Succeed())
	Expect(parsed.Type).To(Equal("service_account"))
}

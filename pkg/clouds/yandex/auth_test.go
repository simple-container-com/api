// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"net/url"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestAccountConfigAuthInterface(t *testing.T) {
	RegisterTestingT(t)

	cfg := &AccountConfig{
		CloudID:         "b1gexamplecloud",
		FolderID:        "b1gexamplefolder",
		AccessKey:       "YCAJEexample",
		SecretAccessKey: "YCexamplesecret",
	}

	Expect(cfg.ProviderType()).To(Equal(ProviderType))
	// ${auth:yc.projectId} must interpolate to the folder: it is the unit every
	// YC resource is created in, the closest analogue to an AWS account.
	Expect(cfg.ProjectIdValue()).To(Equal("b1gexamplefolder"))

	// With no opaque credentials blob, CredentialsValue serialises the config so
	// api.ConvertAuth can rehydrate it. Every field must survive that round trip.
	roundTripped := &AccountConfig{}
	Expect(api.ConvertAuth(cfg, roundTripped)).To(Succeed())
	Expect(roundTripped.CloudID).To(Equal("b1gexamplecloud"))
	Expect(roundTripped.FolderID).To(Equal("b1gexamplefolder"))
	Expect(roundTripped.AccessKey).To(Equal("YCAJEexample"))
	Expect(roundTripped.SecretAccessKey).To(Equal("YCexamplesecret"))
}

func TestAccountConfigDefaults(t *testing.T) {
	RegisterTestingT(t)

	empty := &AccountConfig{}
	Expect(empty.EffectiveRegion()).To(Equal(DefaultRegion))
	Expect(empty.EffectiveZone()).To(Equal(DefaultZone))

	set := &AccountConfig{Region: "kz1", Zone: "kz1-a"}
	Expect(set.EffectiveRegion()).To(Equal("kz1"))
	Expect(set.EffectiveZone()).To(Equal("kz1-a"))
}

// TestStateStorageUrl pins the exact query parameters Pulumi's DIY backend needs
// to reach Yandex Object Storage. All three are read by gocloud — `endpoint` and
// `region` by aws.V2ConfigFromURLParams, `s3ForcePathStyle` by
// s3blob.URLOpener.OpenBucketURL — and gocloud REJECTS unknown parameters, so a
// typo here is a hard failure at login time, not a fallback.
func TestStateStorageUrl(t *testing.T) {
	RegisterTestingT(t)

	cfg := &StateStorageConfig{BucketName: "sc-state-yc"}

	u, err := url.Parse(cfg.StorageUrl())
	Expect(err).ToNot(HaveOccurred())
	Expect(u.Scheme).To(Equal("s3"))
	Expect(u.Host).To(Equal("sc-state-yc"))

	q := u.Query()
	Expect(q.Get("endpoint")).To(Equal(DefaultStorageEndpoint))
	Expect(q.Get("region")).To(Equal(DefaultRegion))
	Expect(q.Get("s3ForcePathStyle")).To(Equal("true"))
	// Exactly these three: gocloud errors on any parameter it does not know.
	Expect(q).To(HaveLen(3))

	override := &StateStorageConfig{
		BucketName:    "sc-state-yc",
		Endpoint:      "https://storage.example.net",
		AccountConfig: AccountConfig{Region: "kz1"},
	}
	oq, err := url.Parse(override.StorageUrl())
	Expect(err).ToNot(HaveOccurred())
	Expect(oq.Query().Get("endpoint")).To(Equal("https://storage.example.net"))
	Expect(oq.Query().Get("region")).To(Equal("kz1"))
}

func TestStateStorageAndSecretsProviderInterfaces(t *testing.T) {
	RegisterTestingT(t)

	// Compile-time proof these satisfy the interfaces pulumi/login.go asserts on;
	// a missing method there surfaces as a runtime "config is not of type" error.
	var ss api.StateStorageConfig = &StateStorageConfig{BucketName: "b", Provision: true}
	Expect(ss.IsProvisionEnabled()).To(BeTrue())
	Expect(ss.StorageUrl()).To(HavePrefix("s3://b?"))

	var sp api.SecretsProviderConfig = &SecretsProviderConfig{
		KeyName:   "awskms://abc?endpoint=https://kms.yandex/&region=ru-central1",
		Provision: false,
	}
	Expect(sp.IsProvisionEnabled()).To(BeFalse())
	Expect(sp.KeyUrl()).To(ContainSubstring("kms.yandex"))
	Expect(sp.ProviderType()).To(Equal(ProviderType))
}

func TestReadConfigs(t *testing.T) {
	RegisterTestingT(t)

	// Shape of a secrets.yaml `auth:` entry once the YAML is loaded.
	authCfg := &api.Config{Config: map[string]any{
		"cloudId":         "b1gcloud",
		"folderId":        "b1gfolder",
		"accessKey":       "YCAJE",
		"secretAccessKey": "shh",
	}}
	out, err := ReadAuthServiceAccountConfig(authCfg)
	Expect(err).ToNot(HaveOccurred())
	acc, ok := out.Config.(*AccountConfig)
	Expect(ok).To(BeTrue())
	Expect(acc.FolderID).To(Equal("b1gfolder"))

	stateCfg := &api.Config{Config: map[string]any{
		"bucketName": "sc-state-yc",
		"provision":  true,
		"folderId":   "b1gfolder",
	}}
	stateOut, err := ReadStateStorageConfig(stateCfg)
	Expect(err).ToNot(HaveOccurred())
	state, ok := stateOut.Config.(*StateStorageConfig)
	Expect(ok).To(BeTrue())
	Expect(state.BucketName).To(Equal("sc-state-yc"))
	Expect(state.IsProvisionEnabled()).To(BeTrue())
}

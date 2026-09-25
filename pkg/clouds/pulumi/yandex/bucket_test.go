// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

// bucketMocks records what the provisioner actually asked Pulumi to create, so
// the assertions land on the inputs that reach each resource rather than on the
// helpers that compute them.
type bucketMocks struct {
	mu      sync.Mutex
	created []pulumi.MockResourceArgs
}

func newBucketMocks() *bucketMocks { return &bucketMocks{} }

func (m *bucketMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, args)
	return args.Name + "-id", args.Inputs, nil
}

func (m *bucketMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (m *bucketMocks) inputsOf(typeToken string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.created {
		if a.TypeToken == typeToken {
			return a.Inputs
		}
	}
	return nil
}

func (m *bucketMocks) countOf(typeToken string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, a := range m.created {
		if a.TypeToken == typeToken {
			n++
		}
	}
	return n
}

// bucketProvisionParams builds a real provider under the mocks: ProvisionParams
// with a nil Provider panics inside the resource constructors.
func bucketProvisionParams(ctx *pulumi.Context) (pApi.ProvisionParams, error) {
	params := pApi.ProvisionParams{Log: logger.New()}
	prov, err := Provider(ctx, api.Stack{}, api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type: yandex.ProviderType,
			Name: "test-yandex",
			Config: api.Config{Config: &yandex.AccountConfig{
				CloudID:  "test-cloud",
				FolderID: "test-folder",
			}},
		},
		StackParams: &api.StackParams{Environment: "test"},
	}, params)
	if err != nil {
		return params, err
	}
	provider, ok := prov.Ref.(pulumi.ProviderResource)
	if !ok {
		return params, errors.Errorf("Provider().Ref is %T, not a pulumi.ProviderResource", prov.Ref)
	}
	params.Provider = provider
	return params, nil
}

func bucketResourceInput(cfg *yandex.ObjectStorageBucket) api.ResourceInput {
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type:   yandex.ResourceTypeObjectStorageBucket,
			Name:   "blobs",
			Config: api.Config{Config: cfg},
		},
		StackParams: &api.StackParams{Environment: "test"},
	}
}

func baseBucketConfig() *yandex.ObjectStorageBucket {
	return &yandex.ObjectStorageBucket{
		AccountConfig: yandex.AccountConfig{
			CloudID:  "test-cloud",
			FolderID: "test-folder",
		},
	}
}

// Provider().Ref must satisfy sdk.ProviderResource — provision.go hard-casts it
// and fails the whole stack if it does not.
func TestProvider_ReturnsProviderResource(t *testing.T) {
	RegisterTestingT(t)

	mocks := newBucketMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := bucketProvisionParams(ctx)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).To(BeNil())
	inputs := mocks.inputsOf("pulumi:providers:yandex")
	Expect(inputs).NotTo(BeNil(), "the yandex provider must actually be instantiated")
	Expect(inputs["folderId"].StringValue()).To(Equal("test-folder"))
	Expect(inputs["regionId"].StringValue()).To(Equal(yandex.DefaultRegion))
	Expect(inputs["zone"].StringValue()).To(Equal(yandex.DefaultZone))
}

func TestProvider_RejectsMissingFolder(t *testing.T) {
	RegisterTestingT(t)

	mocks := newBucketMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := Provider(ctx, api.Stack{}, api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Type:   yandex.ProviderType,
				Name:   "test-yandex",
				Config: api.Config{Config: &yandex.AccountConfig{CloudID: "test-cloud"}},
			},
			StackParams: &api.StackParams{Environment: "test"},
		}, pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("folderId"))
	Expect(mocks.countOf("pulumi:providers:yandex")).To(Equal(0))
}

// The bucket is useless without an identity that can reach it: a service
// account, an additive folder role binding, and a static key pair. Asserting all
// four resources pins the shape against a refactor that quietly drops one.
func TestObjectStorageBucket_ProvisionsIdentityAlongsideBucket(t *testing.T) {
	RegisterTestingT(t)

	mocks := newBucketMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := bucketProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = ObjectStorageBucket(ctx, api.Stack{Name: "test-stack"}, bucketResourceInput(baseBucketConfig()), params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).To(BeNil())
	Expect(mocks.countOf("yandex:index/storageBucket:StorageBucket")).To(Equal(1))
	Expect(mocks.countOf("yandex:index/iamServiceAccount:IamServiceAccount")).To(Equal(1))
	Expect(mocks.countOf("yandex:index/iamServiceAccountStaticAccessKey:IamServiceAccountStaticAccessKey")).To(Equal(1))

	// Additive, never authoritative: ...FolderIamPolicy would replace every other
	// binding in the folder.
	Expect(mocks.countOf("yandex:index/resourcemanagerFolderIamMember:ResourcemanagerFolderIamMember")).To(Equal(1))
	Expect(mocks.countOf("yandex:index/resourcemanagerFolderIamPolicy:ResourcemanagerFolderIamPolicy")).To(Equal(0))

	role := mocks.inputsOf("yandex:index/resourcemanagerFolderIamMember:ResourcemanagerFolderIamMember")
	Expect(role["role"].StringValue()).To(Equal(yandex.DefaultBucketRole))
}

func TestObjectStorageBucket_HonoursExplicitNameAndRole(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseBucketConfig()
	cfg.Name = "custom-bucket"
	cfg.Role = "storage.viewer"
	cfg.MaxSize = 1024

	mocks := newBucketMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := bucketProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = ObjectStorageBucket(ctx, api.Stack{Name: "test-stack"}, bucketResourceInput(cfg), params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).To(BeNil())
	bucket := mocks.inputsOf("yandex:index/storageBucket:StorageBucket")
	Expect(bucket["bucket"].StringValue()).To(ContainSubstring("custom-bucket"))
	Expect(bucket["maxSize"].NumberValue()).To(Equal(float64(1024)))

	role := mocks.inputsOf("yandex:index/resourcemanagerFolderIamMember:ResourcemanagerFolderIamMember")
	Expect(role["role"].StringValue()).To(Equal("storage.viewer"))
}

func TestObjectStorageBucket_RejectsMissingFolderBeforeCreatingAnything(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseBucketConfig()
	cfg.FolderID = ""

	mocks := newBucketMocks()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := bucketProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = ObjectStorageBucket(ctx, api.Stack{Name: "test-stack"}, bucketResourceInput(cfg), params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("folderId"))
	Expect(mocks.countOf("yandex:index/iamServiceAccount:IamServiceAccount")).To(Equal(0))
	Expect(mocks.countOf("yandex:index/storageBucket:StorageBucket")).To(Equal(0))
}

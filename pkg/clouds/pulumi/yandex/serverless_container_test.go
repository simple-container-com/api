// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

const (
	tokenContainer      = "yandex:index/serverlessContainer:ServerlessContainer"
	tokenContainerIam   = "yandex:index/serverlessContainerIamBinding:ServerlessContainerIamBinding"
	tokenRegistry       = "yandex:index/containerRegistry:ContainerRegistry"
	tokenServiceAccount = "yandex:index/iamServiceAccount:IamServiceAccount"
	tokenFolderMember   = "yandex:index/resourcemanagerFolderIamMember:ResourcemanagerFolderIamMember"
	tokenLockboxSecret  = "yandex:index/lockboxSecret:LockboxSecret"
	tokenLockboxVersion = "yandex:index/lockboxSecretVersion:LockboxSecretVersion"
	tokenTrigger        = "yandex:index/functionTrigger:FunctionTrigger"
	tokenDockerImage    = "docker:index/image:Image"
)

// testServiceAccountKey stands in for an authorized-key document. It is a
// throwaway literal, not a credential: containerRegistryPassword only needs to
// see something that parses as a JSON object.
const testServiceAccountKey = `{"id":"test","service_account_id":"test","private_key":"not-a-key"}`

func containerAccountConfig() yandex.AccountConfig {
	return yandex.AccountConfig{
		CloudID:           "test-cloud",
		FolderID:          "test-folder",
		ServiceAccountKey: testServiceAccountKey,
		Credentials:       api.Credentials{Credentials: "{}"},
	}
}

func baseContainerInput() *yandex.ServerlessContainerInput {
	return &yandex.ServerlessContainerInput{
		AccountConfig: containerAccountConfig(),
		StackConfig: api.StackConfigSingleImage{
			Image: &api.ContainerImage{
				Name:       "app",
				Dockerfile: "Dockerfile",
				Context:    ".",
			},
		},
	}
}

func containerResourceInput(cfg *yandex.ServerlessContainerInput) api.ResourceInput {
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type:   yandex.TemplateTypeYandexServerlessContainer,
			Name:   "app",
			Config: api.Config{Config: cfg},
		},
		StackParams: &api.StackParams{Environment: "test", Version: "1.2.3"},
	}
}

// containerProvisionParams extends bucketProvisionParams with the compute
// context the container path reads env/secret variables and dependencies from —
// a nil ComputeContext panics before the first resource is created.
func containerProvisionParams(ctx *pulumi.Context) (pApi.ProvisionParams, error) {
	params, err := bucketProvisionParams(ctx)
	if err != nil {
		return params, err
	}
	params.ComputeContext = pApi.NewComputeContextCollector(ctx.Context(), logger.New(), "test-stack", "test")
	return params, nil
}

func provisionContainer(cfg *yandex.ServerlessContainerInput, mocks *bucketMocks) error {
	return pulumi.RunErr(func(ctx *pulumi.Context) error {
		params, err := containerProvisionParams(ctx)
		if err != nil {
			return err
		}
		_, err = ServerlessContainer(ctx, api.Stack{Name: "test-stack"}, containerResourceInput(cfg), params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
}

// A container that cannot pull its image, cannot read its secrets, or cannot be
// invoked is not a deploy. Asserting the whole set pins the shape against a
// refactor that quietly drops one of them.
func TestServerlessContainer_ProvisionsCoreResources(t *testing.T) {
	RegisterTestingT(t)

	mocks := newBucketMocks()
	Expect(provisionContainer(baseContainerInput(), mocks)).To(BeNil())

	Expect(mocks.countOf(tokenRegistry)).To(Equal(1))
	Expect(mocks.countOf(tokenDockerImage)).To(Equal(1))
	Expect(mocks.countOf(tokenServiceAccount)).To(Equal(1))
	Expect(mocks.countOf(tokenContainer)).To(Equal(1))
	Expect(mocks.countOf(tokenContainerIam)).To(Equal(1))

	// One additive binding per default role. FolderIamPolicy is authoritative and
	// would wipe every other binding in the folder — it must never appear.
	Expect(mocks.countOf(tokenFolderMember)).To(Equal(len(DefaultContainerRoles)))
	Expect(mocks.countOf("yandex:index/resourcemanagerFolderIamPolicy:ResourcemanagerFolderIamPolicy")).To(Equal(0))

	container := mocks.inputsOf(tokenContainer)
	Expect(container["name"].StringValue()).To(Equal("test-stack--test"))
	Expect(container["folderId"].StringValue()).To(Equal("test-folder"))
	Expect(container["memory"].NumberValue()).To(Equal(float64(DefaultMemoryMB)))
	Expect(container["executionTimeout"].StringValue()).To(Equal("10s"))

	env := container["image"].ObjectValue()["environment"].ObjectValue()
	Expect(env[resource.PropertyKey(api.ComputeEnv.StackName)].StringValue()).To(Equal("test-stack"))
	Expect(env[resource.PropertyKey(api.ComputeEnv.StackEnv)].StringValue()).To(Equal("test"))
	Expect(env[resource.PropertyKey(api.ComputeEnv.StackVersion)].StringValue()).To(Equal("1.2.3"))
	// go-aws-lambda-sdk serves plain HTTP instead of starting the Lambda runtime
	// loop only when it sees this; YC itself sets nothing that names the cloud.
	Expect(env[resource.PropertyKey(api.ComputeEnv.CloudProvider)].StringValue()).To(Equal(yandex.ProviderType))

	// Public invoke, the analogue of the AWS function URL's Principal "*": without
	// it the endpoint answers 403 even to the Cloudflare worker fronting the domain.
	iam := mocks.inputsOf(tokenContainerIam)
	Expect(iam["role"].StringValue()).To(Equal(ContainerInvokerRole))
	Expect(iam["members"].ArrayValue()[0].StringValue()).To(Equal(AllUsersMember))
}

// Secrets must reach the container as Lockbox *references*. A value inlined into
// image.environment would be readable by anyone with describe rights on the
// container and would land in plain text in Pulumi state.
func TestServerlessContainer_SecretsBecomeLockboxReferences(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.StackConfig.Secrets = map[string]string{"MONGO_URI": "mongodb://secret", "API_KEY": "s3cr3t"}
	cfg.StackConfig.Env = map[string]string{"LOG_LEVEL": "debug"}

	mocks := newBucketMocks()
	Expect(provisionContainer(cfg, mocks)).To(BeNil())

	// One secret holding every entry, not one secret per variable: Lockbox quotas
	// and billing are per-secret.
	Expect(mocks.countOf(tokenLockboxSecret)).To(Equal(1))
	Expect(mocks.countOf(tokenLockboxVersion)).To(Equal(1))

	entries := mocks.inputsOf(tokenLockboxVersion)["entries"].ArrayValue()
	Expect(entries).To(HaveLen(2))
	// Sorted, so the entry list does not diff on every deploy from map ordering.
	Expect(entries[0].ObjectValue()["key"].StringValue()).To(Equal("API_KEY"))
	Expect(entries[1].ObjectValue()["key"].StringValue()).To(Equal("MONGO_URI"))

	container := mocks.inputsOf(tokenContainer)
	secretRefs := container["secrets"].ArrayValue()
	Expect(secretRefs).To(HaveLen(2))
	Expect(secretRefs[0].ObjectValue()["environmentVariable"].StringValue()).To(Equal("API_KEY"))
	Expect(secretRefs[0].ObjectValue()["key"].StringValue()).To(Equal("API_KEY"))

	env := container["image"].ObjectValue()["environment"].ObjectValue()
	Expect(env["LOG_LEVEL"].StringValue()).To(Equal("debug"))
	Expect(env).NotTo(HaveKey(resource.PropertyKey("MONGO_URI")))
	Expect(env).NotTo(HaveKey(resource.PropertyKey("API_KEY")))
}

// A version with zero entries is rejected by the Lockbox API, so a stack with no
// secrets must get no Lockbox resources at all rather than an empty one.
func TestServerlessContainer_NoSecretsCreatesNoLockbox(t *testing.T) {
	RegisterTestingT(t)

	mocks := newBucketMocks()
	Expect(provisionContainer(baseContainerInput(), mocks)).To(BeNil())

	Expect(mocks.countOf(tokenLockboxSecret)).To(Equal(0))
	Expect(mocks.countOf(tokenLockboxVersion)).To(Equal(0))
	Expect(mocks.inputsOf(tokenContainer)).NotTo(HaveKey(resource.PropertyKey("secrets")))
}

func TestServerlessContainer_SchedulesBecomeTimerTriggers(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.StackConfig.CloudExtras = lo.ToPtr(any(map[string]any{
		"schedules": []any{
			map[string]any{
				"name":          "cleanup",
				"expression":    "0 0 * * ? *",
				"request":       `{"path":"/cleanup"}`,
				"retryAttempts": 3,
				"retryInterval": "30s",
			},
		},
	}))

	mocks := newBucketMocks()
	Expect(provisionContainer(cfg, mocks)).To(BeNil())

	Expect(mocks.countOf(tokenTrigger)).To(Equal(1))
	trigger := mocks.inputsOf(tokenTrigger)
	Expect(trigger["name"].StringValue()).To(Equal("test-stack--test-cleanup"))
	Expect(trigger["timer"].ObjectValue()["cronExpression"].StringValue()).To(Equal("0 0 * * ? *"))
	Expect(trigger["timer"].ObjectValue()["payload"].StringValue()).To(Equal(`{"path":"/cleanup"}`))

	container := trigger["container"].ObjectValue()
	Expect(container["retryAttempts"].StringValue()).To(Equal("3"))
	// Seconds as a bare integer. The field is a string, but the provider parses it
	// with strconv.ParseInt, so the duration spelling the client.yaml uses ("30s")
	// must not survive into the resource input.
	Expect(container["retryInterval"].StringValue()).To(Equal("30"))

	// No dlq block unless one was asked for — an empty QueueId is not a valid
	// resource and would fail at apply time rather than being ignored.
	Expect(trigger["dlq"].IsNull()).To(BeTrue())
}

func TestServerlessContainer_ScheduleDLQBecomesTriggerDLQ(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.StackConfig.CloudExtras = lo.ToPtr(any(map[string]any{
		"schedules": []any{
			map[string]any{
				"name":       "cleanup",
				"expression": "0 0 * * ? *",
				"request":    `{"path":"/cleanup"}`,
				"dlq":        "yc-dlq-queue-id",
			},
		},
	}))

	mocks := newBucketMocks()
	Expect(provisionContainer(cfg, mocks)).To(BeNil())

	dlq := mocks.inputsOf(tokenTrigger)["dlq"].ObjectValue()
	Expect(dlq["queueId"].StringValue()).To(Equal("yc-dlq-queue-id"))
	// The trigger writes to the queue as the container's own service account;
	// without it YC accepts the trigger and silently drops every failed message.
	Expect(dlq["serviceAccountId"].IsNull()).To(BeFalse())
}

// Folder role bindings are additive, so two stacks each "owning" the same
// (folder, role, member) triple would let destroying one silently strip the grant
// from the other. Roles alongside a caller-supplied account are rejected, never
// silently ignored.
func TestServerlessContainer_RejectsRolesWithExplicitServiceAccount(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.ServiceAccountID = "aje000000000000000000"
	cfg.StackConfig.CloudExtras = lo.ToPtr(any(map[string]any{
		"roles": []any{"storage.editor"},
	}))

	mocks := newBucketMocks()
	err := provisionContainer(cfg, mocks)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("serviceAccountId"))
	Expect(mocks.countOf(tokenServiceAccount)).To(Equal(0))
	Expect(mocks.countOf(tokenFolderMember)).To(Equal(0))
	Expect(mocks.countOf(tokenContainer)).To(Equal(0))
}

func TestServerlessContainer_UsesExplicitServiceAccountWithoutManagingItsIam(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.ServiceAccountID = "aje000000000000000000"

	mocks := newBucketMocks()
	Expect(provisionContainer(cfg, mocks)).To(BeNil())

	Expect(mocks.countOf(tokenServiceAccount)).To(Equal(0))
	Expect(mocks.countOf(tokenFolderMember)).To(Equal(0))
	Expect(mocks.inputsOf(tokenContainer)["serviceAccountId"].StringValue()).To(Equal("aje000000000000000000"))
}

// Rounding would quietly change what the operator asked for, and the same
// client.yaml is shared with the AWS deploy where any value is legal.
func TestServerlessContainer_RejectsUnalignedMemory(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.StackConfig.MaxMemory = lo.ToPtr(1000)

	mocks := newBucketMocks()
	err := provisionContainer(cfg, mocks)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("128"))
	Expect(mocks.countOf(tokenContainer)).To(Equal(0))
	Expect(mocks.countOf(tokenDockerImage)).To(Equal(0))
}

// Without the authorized-key document there is nothing to `docker login cr.yandex`
// with: the static access key pair signs the S3-compatible services only.
func TestServerlessContainer_RejectsMissingServiceAccountKey(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.ServiceAccountKey = ""

	mocks := newBucketMocks()
	err := provisionContainer(cfg, mocks)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("serviceAccountKey"))
	Expect(mocks.countOf(tokenDockerImage)).To(Equal(0))
	Expect(mocks.countOf(tokenContainer)).To(Equal(0))
}

// The operator-supplied registry is used as-is, so SC neither creates nor diffs a
// registry it does not own.
func TestServerlessContainer_UsesPreExistingRegistry(t *testing.T) {
	RegisterTestingT(t)

	cfg := baseContainerInput()
	cfg.RegistryID = "crp000000000000000000"

	mocks := newBucketMocks()
	Expect(provisionContainer(cfg, mocks)).To(BeNil())

	Expect(mocks.countOf(tokenRegistry)).To(Equal(0))
	Expect(mocks.inputsOf(tokenDockerImage)["imageName"].StringValue()).
		To(Equal("cr.yandex/crp000000000000000000/test-stack:1.2.3"))
}

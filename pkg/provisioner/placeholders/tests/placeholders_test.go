// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package tests

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	gcpApi "github.com/simple-container-com/sc-plugin-gcp/api"

	"github.com/simple-container-com/api/pkg/api"
	git_mocks "github.com/simple-container-com/api/pkg/api/git/mocks"
	tests "github.com/simple-container-com/api/pkg/api/tests"
	testutils "github.com/simple-container-com/api/pkg/api/tests/testutil"
	"github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/clouds/pulumi"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

func Test_placeholders_ProcessStacks(t *testing.T) {
	RegisterTestingT(t)

	tcs := []struct {
		name     string
		stacks   api.StacksMap
		wantErr  string
		initOpts func(t *testing.T) []placeholders.InitOption
		check    func(t *testing.T, stacks api.StacksMap)
	}{
		{
			name: "common stack",
			stacks: api.StacksMap{
				"common": tests.CommonStack,
			},
			check: func(t *testing.T, stacks api.StacksMap) {
				// auth
				Expect(stacks["common"].Secrets.Auth["gcloud"]).NotTo(BeNil())
				authConfigGeneric := stacks["common"].Secrets.Auth["gcloud"].Config.Config
				Expect(authConfigGeneric).To(BeAssignableToTypeOf(&gcpApi.Credentials{}))
				authConfig := authConfigGeneric.(*gcpApi.Credentials)
				Expect(authConfig.ProjectId).To(Equal("test-gcp-project"))
				Expect(authConfig.Credentials.Credentials).To(Equal("<gcloud-service-account-email>"))

				// provisioner
				srvConfig := stacks["common"].Server.Provisioner.Config.Config
				Expect(srvConfig).To(BeAssignableToTypeOf(&pulumi.ProvisionerConfig{}))
				pConfig := srvConfig.(*pulumi.ProvisionerConfig)

				// state storage
				Expect(pConfig.StateStorage.Config.Config).To(BeAssignableToTypeOf(&gcpApi.StateStorageConfig{}))
				stateStorageCfg := pConfig.StateStorage.Config.Config.(*gcpApi.StateStorageConfig)
				Expect(pConfig.SecretsProvider.Config.Config).To(BeAssignableToTypeOf(&gcpApi.SecretsProviderConfig{}))
				secretsProviderCfg := pConfig.SecretsProvider.Config.Config.(*gcpApi.SecretsProviderConfig)
				Expect(stateStorageCfg.ProjectId).To(Equal("test-gcp-project"))
				Expect(stateStorageCfg.Credentials.Credentials.Credentials).To(Equal("<gcloud-service-account-email>"))
				Expect(secretsProviderCfg.ProjectId).To(Equal("test-gcp-project"))
				Expect(secretsProviderCfg.Credentials.Credentials.Credentials).To(Equal("<gcloud-service-account-email>"))

				// cicd
				Expect(stacks["common"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.GitHubActionsCiCdConfig{}))
				cicdConfig := stacks["common"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.GitHubActionsCiCdConfig)
				Expect(ghConfig.Organization).To(Equal("simple-container-org"))
			},
		},
		{
			name: "refapp stack",
			stacks: api.StacksMap{
				"common": tests.CommonStack,
				"refapp": tests.RefappStack,
			},
			initOpts: func(t *testing.T) []placeholders.InitOption {
				gitMock := git_mocks.NewGitRepoMock(t)
				gitMock.On("Workdir").Return("<root-dir>")
				gitMock.On("Hash").Return("abc123", nil).Maybe()
				gitMock.On("Branch").Return("main", nil).Maybe()
				return []placeholders.InitOption{
					placeholders.WithGitRepo(gitMock),
				}
			},
			check: func(t *testing.T, stacks api.StacksMap) {
				Expect(stacks["refapp"]).NotTo(BeNil())
				resPgCfg := stacks["refapp"].Server.Resources.Resources["staging"].Resources["postgres"].Config.Config
				Expect(resPgCfg).To(BeAssignableToTypeOf(&gcpApi.PostgresGcpCloudsqlConfig{}))
				pgConfig := resPgCfg.(*gcpApi.PostgresGcpCloudsqlConfig)
				Expect(pgConfig.CredentialsValue()).To(Equal("<gcloud-service-account-email>"))
				Expect(pgConfig.Project).To(Equal("refapp"))

				Expect(stacks["refapp"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.GitHubActionsCiCdConfig{}))
				cicdConfig := stacks["refapp"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.GitHubActionsCiCdConfig)
				Expect(ghConfig.Organization).To(Equal("simple-container-org"))

				// Post-A3 the mongodb-atlas config decodes opaquely into
				// *api.PluginFieldConfig; assert via the Raw map rather
				// than through sc-plugin-mongodb's typed `AtlasConfig`.
				resMongoCfg := stacks["refapp"].Server.Resources.Resources["staging"].Resources["mongodb"].Config.Config
				Expect(resMongoCfg).To(BeAssignableToTypeOf(&api.PluginFieldConfig{}))
				mongoField := resMongoCfg.(*api.PluginFieldConfig)
				Expect(mongoField.TypeName).To(Equal("mongodb-atlas"))
				Expect(mongoField.Raw["publicKey"]).To(Equal("<encrypted-secret>"))
				Expect(mongoField.Raw["privateKey"]).To(Equal("<encrypted-secret>"))
				Expect(mongoField.Raw["instanceSize"]).To(Equal("M10"))
				Expect(mongoField.Raw["orgId"]).To(Equal("5b89110a4e6581562623c59c"))
				Expect(mongoField.Raw["projectId"]).To(Equal("5b89110a4e6581562623c59c"))
				Expect(mongoField.Raw["projectName"]).To(Equal("refapp"))
				Expect(mongoField.Raw["region"]).To(Equal("US_SOUTH_1"))

				// client
				Expect(stacks["refapp"].Client.Stacks).To(HaveKey("staging"))
				Expect(stacks["refapp"].Client.Stacks["staging"].Config.Config).NotTo(BeNil())
				stagingCfg := stacks["refapp"].Client.Stacks["staging"].Config.Config
				Expect(stagingCfg).To(BeAssignableToTypeOf(&api.StackConfigCompose{}))
				stagingClientCfg := stagingCfg.(*api.StackConfigCompose)
				Expect(stagingClientCfg.DockerComposeFile).To(Equal("<root-dir>/docker-compose.yaml"))
				Expect(stagingClientCfg.Env["JWT_SECRET"]).To(Equal("<encrypted-secret>"))
				// must not process ${resource:} refs at this time
				Expect(stagingClientCfg.Env["MONGO_URI"]).To(Equal("${resource:mongodb.uri}"))
			},
		},
		{
			name: "refapp-aws stack",
			stacks: api.StacksMap{
				"common":     tests.CommonStack,
				"refapp-aws": tests.RefappAwsStack,
			},
			check: func(t *testing.T, stacks api.StacksMap) {
				Expect(stacks["refapp-aws"]).NotTo(BeNil())
				Expect(stacks["refapp-aws"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.GitHubActionsCiCdConfig{}))
				cicdConfig := stacks["refapp-aws"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.GitHubActionsCiCdConfig)
				Expect(ghConfig.Organization).To(Equal("simple-container-org"))
				// TODO: tests for aws resources
			},
		},
	}
	t.Parallel()
	for _, tt := range tcs {
		t.Run(tt.name, func(t *testing.T) {
			var initOpts []placeholders.InitOption
			if tt.initOpts != nil {
				initOpts = tt.initOpts(t)
			}
			ph := placeholders.New(initOpts...)

			err := ph.Resolve(tt.stacks)

			testutils.CheckError(err, tt.wantErr)

			if tt.check != nil {
				tt.check(t, tt.stacks)
			}
			fmt.Println("OK")
		})
	}
}

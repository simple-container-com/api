package placeholders

import (
	"fmt"
	"testing"

	git_mocks "github.com/simple-container-com/api/pkg/api/git/mocks"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	tests "github.com/simple-container-com/api/pkg/api/tests"
	testutils "github.com/simple-container-com/api/pkg/api/tests/testutil"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/github"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	"github.com/simple-container-com/api/pkg/clouds/pulumi"
)

func Test_placeholders_ProcessStacks(t *testing.T) {
	RegisterTestingT(t)

	tcs := []struct {
		name    string
		stacks  api.StacksMap
		wantErr string
		init    func(t *testing.T, ph *placeholders)
		check   func(t *testing.T, stacks api.StacksMap)
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
				Expect(authConfigGeneric).To(BeAssignableToTypeOf(&gcloud.Credentials{}))
				authConfig := authConfigGeneric.(*gcloud.Credentials)
				Expect(authConfig.ProjectId).To(Equal("test-gcp-project"))
				Expect(authConfig.Credentials.Credentials).To(Equal("<gcloud-service-account-email>"))

				// provisioner
				srvConfig := stacks["common"].Server.Provisioner.Config.Config
				Expect(srvConfig).To(BeAssignableToTypeOf(&pulumi.ProvisionerConfig{}))
				pConfig := srvConfig.(*pulumi.ProvisionerConfig)

				// state storage
				Expect(pConfig.StateStorage.Config.Config).To(BeAssignableToTypeOf(&gcloud.StateStorageConfig{}))
				stateStorageCfg := pConfig.StateStorage.Config.Config.(*gcloud.StateStorageConfig)
				Expect(pConfig.SecretsProvider.Config.Config).To(BeAssignableToTypeOf(&gcloud.SecretsProviderConfig{}))
				secretsProviderCfg := pConfig.SecretsProvider.Config.Config.(*gcloud.SecretsProviderConfig)
				Expect(stateStorageCfg.ProjectId).To(Equal("test-gcp-project"))
				Expect(stateStorageCfg.Credentials.Credentials.Credentials).To(Equal("<gcloud-service-account-email>"))
				Expect(secretsProviderCfg.ProjectId).To(Equal("test-gcp-project"))
				Expect(secretsProviderCfg.Credentials.Credentials.Credentials).To(Equal("<gcloud-service-account-email>"))

				// cicd
				Expect(stacks["common"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.ActionsCiCdConfig{}))
				cicdConfig := stacks["common"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.ActionsCiCdConfig)
				Expect(ghConfig.AuthToken).To(Equal("<encrypted-secret>"))
			},
		},
		{
			name: "refapp stack",
			stacks: api.StacksMap{
				"common": tests.CommonStack,
				"refapp": tests.RefappStack,
			},
			init: func(t *testing.T, ph *placeholders) {
				gitMock := git_mocks.NewGitRepoMock(t)
				gitMock.On("Workdir").Return("<root-dir>")
				ph.git = gitMock
			},
			check: func(t *testing.T, stacks api.StacksMap) {
				Expect(stacks["refapp"]).NotTo(BeNil())
				resPgCfg := stacks["refapp"].Server.Resources.Resources["staging"].Resources["postgres"].Config.Config
				Expect(resPgCfg).To(BeAssignableToTypeOf(&gcloud.PostgresGcpCloudsqlConfig{}))
				pgConfig := resPgCfg.(*gcloud.PostgresGcpCloudsqlConfig)
				Expect(pgConfig.CredentialsValue()).To(Equal("<gcloud-service-account-email>"))
				Expect(pgConfig.Project).To(Equal("refapp"))

				Expect(stacks["refapp"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.ActionsCiCdConfig{}))
				cicdConfig := stacks["refapp"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.ActionsCiCdConfig)
				Expect(ghConfig.AuthToken).To(Equal("<encrypted-secret>"))

				resMongoCfg := stacks["refapp"].Server.Resources.Resources["staging"].Resources["mongodb"].Config.Config
				Expect(resMongoCfg).To(BeAssignableToTypeOf(&mongodb.AtlasConfig{}))
				mongoConfig := resMongoCfg.(*mongodb.AtlasConfig)
				Expect(mongoConfig.PublicKey).To(Equal("<encrypted-secret>"))
				Expect(mongoConfig.PrivateKey).To(Equal("<encrypted-secret>"))
				Expect(mongoConfig.InstanceSize).To(Equal("M10"))
				Expect(mongoConfig.OrgId).To(Equal("5b89110a4e6581562623c59c"))
				Expect(mongoConfig.ProjectId).To(Equal("5b89110a4e6581562623c59c"))
				Expect(mongoConfig.ProjectName).To(Equal("refapp"))
				Expect(mongoConfig.Region).To(Equal("US_SOUTH_1"))

				// client
				Expect(stacks["refapp"].Client.Stacks).To(HaveKey("staging"))
				Expect(stacks["refapp"].Client.Stacks["staging"].Config.Config).NotTo(BeNil())
				stagingCfg := stacks["refapp"].Client.Stacks["staging"].Config.Config
				Expect(stagingCfg).To(BeAssignableToTypeOf(&api.StackConfigCompose{}))
				stagingClientCfg := stagingCfg.(*api.StackConfigCompose)
				Expect(stagingClientCfg.DockerComposeFile).To(Equal("<root-dir>/docker-compose.yaml"))
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
				Expect(stacks["refapp-aws"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.ActionsCiCdConfig{}))
				cicdConfig := stacks["refapp-aws"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.ActionsCiCdConfig)
				Expect(ghConfig.AuthToken).To(Equal("<encrypted-secret>"))
				// TODO: tests for aws resources
			},
		},
	}
	t.Parallel()
	for _, tt := range tcs {
		t.Run(tt.name, func(t *testing.T) {
			ph := &placeholders{
				log: logger.New(),
			}

			if tt.init != nil {
				tt.init(t, ph)
			}

			err := ph.Resolve(tt.stacks)

			testutils.CheckError(err, tt.wantErr)

			if tt.check != nil {
				tt.check(t, tt.stacks)
			}
			fmt.Println("OK")
		})
	}
}

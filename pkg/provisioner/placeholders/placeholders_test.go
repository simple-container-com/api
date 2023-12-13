package placeholders

import (
	"fmt"
	"testing"

	"api/pkg/api"

	"api/pkg/clouds/gcloud"
	"api/pkg/clouds/github"
	"api/pkg/clouds/mongodb"
	"api/pkg/clouds/pulumi"

	. "github.com/onsi/gomega"

	"api/pkg/provisioner/logger"
	testutils "api/pkg/provisioner/tests"
)

func Test_placeholders_ProcessStacks(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		stacks  api.StacksMap
		wantErr string
		check   func(t *testing.T, stacks api.StacksMap)
	}{
		{
			name: "common stack",
			stacks: api.StacksMap{
				"common": testutils.CommonStack,
			},
			check: func(t *testing.T, stacks api.StacksMap) {
				Expect(stacks["common"].Secrets.Auth["gcloud"]).NotTo(BeNil())
				srvConfig := stacks["common"].Server.Provisioner.Config.Config
				Expect(srvConfig).To(BeAssignableToTypeOf(&pulumi.ProvisionerConfig{}))
				pConfig := srvConfig.(*pulumi.ProvisionerConfig)
				Expect(pConfig.StateStorage.Credentials).To(Equal("<gcloud-service-account-email>"))
				Expect(pConfig.SecretsProvider.Credentials).To(Equal("<gcloud-service-account-email>"))

				Expect(stacks["common"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.GithubActionsCiCdConfig{}))
				cicdConfig := stacks["common"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.GithubActionsCiCdConfig)
				Expect(ghConfig.AuthToken).To(Equal("<encrypted-secret>"))
			},
		},
		{
			name: "refapp stack",
			stacks: api.StacksMap{
				"common": testutils.CommonStack,
				"refapp": testutils.RefappStack,
			},
			check: func(t *testing.T, stacks api.StacksMap) {
				Expect(stacks["refapp"]).NotTo(BeNil())
				resPgCfg := stacks["refapp"].Server.Resources.Resources["staging"].Resources["postgres"].Config.Config
				Expect(resPgCfg).To(BeAssignableToTypeOf(&gcloud.PostgresGcpCloudsqlConfig{}))
				pgConfig := resPgCfg.(*gcloud.PostgresGcpCloudsqlConfig)
				Expect(pgConfig.Credentials).To(Equal("<gcloud-service-account-email>"))
				Expect(pgConfig.Project).To(Equal("refapp"))

				Expect(stacks["refapp"].Server.CiCd.Config.Config).To(BeAssignableToTypeOf(&github.GithubActionsCiCdConfig{}))
				cicdConfig := stacks["refapp"].Server.CiCd.Config.Config
				ghConfig := cicdConfig.(*github.GithubActionsCiCdConfig)
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
			},
		},
	}
	t.Parallel()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ph := &placeholders{
				log: logger.New(),
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

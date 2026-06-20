package gcloud

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// ---- CloudRunInput getters ----------------------------------------------

func TestCloudRunInput_Uses(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name string
		uses []string
	}{
		{name: "non-empty uses", uses: []string{"postgres", "redis"}},
		{name: "empty uses", uses: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			in := &CloudRunInput{
				Deployment: k8s.DeploymentConfig{
					StackConfig: &api.StackConfigCompose{Uses: tc.uses},
				},
			}
			Expect(in.Uses()).To(Equal(tc.uses))
		})
	}
}

func TestCloudRunInput_OverriddenBaseZone(t *testing.T) {
	RegisterTestingT(t)
	in := &CloudRunInput{
		Deployment: k8s.DeploymentConfig{
			StackConfig: &api.StackConfigCompose{BaseDnsZone: "example.com"},
		},
	}
	Expect(in.OverriddenBaseZone()).To(Equal("example.com"))
}

func TestToCloudRunConfig_Errors(t *testing.T) {
	RegisterTestingT(t)

	t.Run("wrong template type returns error", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToCloudRunConfig("not-a-template", compose.Config{Project: &types.Project{}}, &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not of type *gcloud.TemplateConfig"))
	})

	t.Run("nil template pointer returns error", func(t *testing.T) {
		RegisterTestingT(t)
		var tpl *TemplateConfig
		_, err := ToCloudRunConfig(tpl, compose.Config{Project: &types.Project{}}, &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is nil"))
	})

	t.Run("ConvertComposeToContainers error surfaces (unknown service in runs)", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{Runs: []string{"does-not-exist"}}
		_, err := ToCloudRunConfig(&TemplateConfig{}, compose.Config{Project: &types.Project{}}, stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found in docker-compose"))
	})

	t.Run("FindIngressContainer error surfaces (multiple ingress containers)", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToCloudRunConfig(&TemplateConfig{}, twoIngressComposeConfig(), &api.StackConfigCompose{Runs: []string{"web", "api"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to detect ingress container"))
	})
}

// twoIngressComposeConfig builds a compose project with two services both
// labeled as ingress containers, which makes FindIngressContainer error out.
func twoIngressComposeConfig() compose.Config {
	ingressLabel := types.Labels{api.ComposeLabelIngressContainer: "true"}
	return compose.Config{Project: &types.Project{
		Services: types.Services{
			{Name: "web", Labels: ingressLabel},
			{Name: "api", Labels: ingressLabel},
		},
	}}
}

// ---- GkeAutopilotInput getters ------------------------------------------

func TestGkeAutopilotInput_Uses(t *testing.T) {
	RegisterTestingT(t)
	in := &GkeAutopilotInput{
		Deployment: k8s.DeploymentConfig{
			StackConfig: &api.StackConfigCompose{Uses: []string{"postgres"}},
		},
	}
	Expect(in.Uses()).To(ConsistOf("postgres"))
}

func TestGkeAutopilotInput_OverriddenBaseZone(t *testing.T) {
	RegisterTestingT(t)
	in := &GkeAutopilotInput{
		Deployment: k8s.DeploymentConfig{
			StackConfig: &api.StackConfigCompose{BaseDnsZone: "zone.example.com"},
		},
	}
	Expect(in.OverriddenBaseZone()).To(Equal("zone.example.com"))
}

func TestGkeAutopilotInput_DependsOnResources(t *testing.T) {
	RegisterTestingT(t)
	deps := []api.StackConfigDependencyResource{
		{Name: "db", Owner: "other-stack", Resource: "postgres"},
	}
	in := &GkeAutopilotInput{
		Deployment: k8s.DeploymentConfig{
			StackConfig: &api.StackConfigCompose{Dependencies: deps},
		},
	}
	got := in.DependsOnResources()
	Expect(got).To(HaveLen(1))
	Expect(got[0].Name).To(Equal("db"))
	Expect(got[0].Owner).To(Equal("other-stack"))
	Expect(got[0].Resource).To(Equal("postgres"))
}

func TestToGkeAutopilotConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("wrong template type returns error", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToGkeAutopilotConfig("not-a-template", compose.Config{Project: &types.Project{}}, &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not of type *gcloud.GkeAutopilotTemplate"))
	})

	t.Run("nil template pointer returns error", func(t *testing.T) {
		RegisterTestingT(t)
		var tpl *GkeAutopilotTemplate
		_, err := ToGkeAutopilotConfig(tpl, compose.Config{Project: &types.Project{}}, &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is nil"))
	})

	t.Run("happy path with no cloudExtras", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{Runs: []string{}}
		res, err := ToGkeAutopilotConfig(&GkeAutopilotTemplate{GkeClusterResource: "c"}, compose.Config{Project: &types.Project{}}, stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in, ok := res.(*GkeAutopilotInput)
		Expect(ok).To(BeTrue())
		Expect(in.GkeClusterResource).To(Equal("c"))
		Expect(in.Deployment.StackConfig).To(Equal(stackCfg))
	})

	t.Run("cloudExtras affinity nodePool adds workload-group selector and toleration", func(t *testing.T) {
		RegisterTestingT(t)
		cloudExtras := any(map[string]any{
			"affinity": map[string]any{
				"nodePool":     "high-mem",
				"computeClass": "Balanced",
			},
		})
		stackCfg := &api.StackConfigCompose{
			Runs:        []string{},
			CloudExtras: &cloudExtras,
		}
		res, err := ToGkeAutopilotConfig(&GkeAutopilotTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in := res.(*GkeAutopilotInput)
		Expect(in.Deployment.NodeSelector).To(HaveKeyWithValue("workload-group", "high-mem"))
		Expect(in.Deployment.NodeSelector).To(HaveKeyWithValue("cloud.google.com/compute-class", "Balanced"))
		Expect(in.Deployment.Tolerations).To(HaveLen(1))
		Expect(in.Deployment.Tolerations[0].Key).To(Equal("workload-group"))
		Expect(in.Deployment.Tolerations[0].Value).To(Equal("high-mem"))
		Expect(in.Deployment.Tolerations[0].Operator).To(Equal("Equal"))
		Expect(in.Deployment.Tolerations[0].Effect).To(Equal("NoSchedule"))
	})

	t.Run("ConvertComposeToContainers error surfaces (unknown service in runs)", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{Runs: []string{"does-not-exist"}}
		_, err := ToGkeAutopilotConfig(&GkeAutopilotTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found in docker-compose"))
	})

	t.Run("FindIngressContainer error surfaces (multiple ingress containers)", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToGkeAutopilotConfig(&GkeAutopilotTemplate{}, twoIngressComposeConfig(), &api.StackConfigCompose{Runs: []string{"web", "api"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to detect ingress container"))
	})

	t.Run("invalid cloudExtras yields conversion error", func(t *testing.T) {
		RegisterTestingT(t)
		// cloudExtras whose affinity is not a mapping cannot be decoded into k8s.CloudExtras.
		cloudExtras := any(map[string]any{
			"affinity": "this-should-be-a-mapping-not-a-string",
		})
		stackCfg := &api.StackConfigCompose{
			Runs:        []string{},
			CloudExtras: &cloudExtras,
		}
		_, err := ToGkeAutopilotConfig(&GkeAutopilotTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
		Expect(err).To(HaveOccurred())
	})
}

// ---- ExternalEgressIpConfig.Validate ------------------------------------

func TestExternalEgressIpConfig_Validate(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name      string
		cfg       ExternalEgressIpConfig
		expectErr bool
		errSubstr string
	}{
		{
			name: "disabled requires no validation",
			cfg:  ExternalEgressIpConfig{Enabled: false, Existing: "garbage"},
		},
		{
			name: "enabled with no existing is valid",
			cfg:  ExternalEgressIpConfig{Enabled: true},
		},
		{
			name: "enabled with valid full path",
			cfg:  ExternalEgressIpConfig{Enabled: true, Existing: "projects/p/regions/eu/addresses/egress"},
		},
		{
			name:      "enabled with non projects/ prefix",
			cfg:       ExternalEgressIpConfig{Enabled: true, Existing: "regions/eu/addresses/egress"},
			expectErr: true,
			errSubstr: "must be a full GCP resource path",
		},
		{
			name:      "enabled with wrong segment count",
			cfg:       ExternalEgressIpConfig{Enabled: true, Existing: "projects/p/regions/eu/addresses"},
			expectErr: true,
			errSubstr: "invalid 'existing' format",
		},
		{
			name:      "enabled with wrong segment labels",
			cfg:       ExternalEgressIpConfig{Enabled: true, Existing: "projects/p/zones/eu/networks/egress"},
			expectErr: true,
			errSubstr: "invalid 'existing' format",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.cfg.Validate()
			if tc.expectErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.errSubstr))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// ---- StaticSiteInput / ToStaticSiteConfig -------------------------------

func TestStaticSiteInput_OverriddenBaseZone(t *testing.T) {
	RegisterTestingT(t)
	in := &StaticSiteInput{BaseDnsZone: "static.example.com"}
	Expect(in.OverriddenBaseZone()).To(Equal("static.example.com"))
}

func TestToStaticSiteConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps stackDir/stackName/location/baseDnsZone", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigStatic{
			Location: "EU",
			Site:     api.StaticSiteConfig{BaseDnsZone: "static.example.com"},
		}
		res, err := ToStaticSiteConfig(&TemplateConfig{}, "/stacks/web", "web", stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in, ok := res.(*StaticSiteInput)
		Expect(ok).To(BeTrue())
		Expect(in.StackDir).To(Equal("/stacks/web"))
		Expect(in.StackName).To(Equal("web"))
		Expect(in.Location).To(Equal("EU"))
		Expect(in.BaseDnsZone).To(Equal("static.example.com"))
		Expect(in.OverriddenBaseZone()).To(Equal("static.example.com"))
		Expect(in.StackConfigStatic).To(Equal(stackCfg))
	})

	t.Run("wrong template type returns error", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToStaticSiteConfig("not-a-template", "/d", "n", &api.StackConfigStatic{Location: "EU"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is not of type"))
	})

	t.Run("nil template pointer returns error", func(t *testing.T) {
		RegisterTestingT(t)
		var tpl *TemplateConfig
		_, err := ToStaticSiteConfig(tpl, "/d", "n", &api.StackConfigStatic{Location: "EU"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is nil"))
	})

	t.Run("missing location returns error", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToStaticSiteConfig(&TemplateConfig{}, "/d", "n", &api.StackConfigStatic{Location: ""})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("location is required"))
	})
}

// ---- init() registration ------------------------------------------------

// init() runs at package load and registers all GCP provider/field readers
// into the api global maps. Assert the GCP resource types resolve.
func TestInitRegistration(t *testing.T) {
	RegisterTestingT(t)

	providers := api.GetRegisteredProviderConfigs()
	for _, key := range []string{
		SecretsTypeGCPSecretsManager,
		TemplateTypeGcpCloudrun,
		TemplateTypeStaticWebsite,
		AuthTypeGCPServiceAccount,
		TemplateTypeGkeAutopilot,
		ResourceTypeGkeAutopilot,
		ResourceTypePostgresGcpCloudsql,
		ResourceTypeRedis,
		ResourceTypeBucket,
		ResourceTypePubSub,
		ResourceTypeArtifactRegistry,
		ResourceTypeRemoteDockerImagePush,
	} {
		Expect(providers).To(HaveKey(key), "provider config %q must be registered by init()", key)
	}

	fields := api.GetRegisteredProvisionerFieldConfigs()
	Expect(fields).To(HaveKey(StateStorageTypeGcpBucket))
	Expect(fields).To(HaveKey(SecretsProviderTypeGcpKms))
}

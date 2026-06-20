package aws

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

// validTemplateConfig builds a TemplateConfig whose AccountConfig round-trips
// cleanly through api.ConvertAuth (which json-Unmarshals CredentialsValue()).
// With Credentials.Credentials empty, CredentialsValue() returns the JSON of
// the AccountConfig itself, so all account fields survive the conversion.
func validTemplateConfig() *TemplateConfig {
	return &TemplateConfig{
		AccountConfig: AccountConfig{
			Account:         "123456789012",
			AccessKey:       "AKIAEXAMPLE",
			SecretAccessKey: "secret",
			Region:          "us-east-1",
		},
	}
}

// ---- ToAwsLambdaConfig --------------------------------------------------

func TestToAwsLambdaConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps account + stack config", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigSingleImage{
			Domain:       "fn.example.com",
			BaseDnsZone:  "example.com",
			Uses:         []string{"my-bucket", "my-db"},
			Dependencies: []api.StackConfigDependencyResource{{Name: "db", Owner: "other", Resource: "postgres"}},
		}
		out, err := ToAwsLambdaConfig(validTemplateConfig(), stackCfg)
		Expect(err).ToNot(HaveOccurred())
		li, ok := out.(*LambdaInput)
		Expect(ok).To(BeTrue())
		Expect(li.Account).To(Equal("123456789012"))
		Expect(li.Region).To(Equal("us-east-1"))
		Expect(li.StackConfig.Domain).To(Equal("fn.example.com"))
		// Getter methods delegate to the embedded StackConfig.
		Expect(li.Uses()).To(ConsistOf("my-bucket", "my-db"))
		Expect(li.OverriddenBaseZone()).To(Equal("example.com"))
		Expect(li.DependsOnResources()).To(HaveLen(1))
		Expect(li.DependsOnResources()[0].Resource).To(Equal("postgres"))
	})

	t.Run("wrong template type errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToAwsLambdaConfig("not-a-template", &api.StackConfigSingleImage{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not of type aws.TemplateConfig"))
	})

	t.Run("nil template config errors", func(t *testing.T) {
		RegisterTestingT(t)
		var tpl *TemplateConfig
		_, err := ToAwsLambdaConfig(tpl, &api.StackConfigSingleImage{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is nil"))
	})

	t.Run("nil stack config errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToAwsLambdaConfig(validTemplateConfig(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("stack config cannot be nil"))
	})

	t.Run("non-JSON credentials make ConvertAuth fail", func(t *testing.T) {
		RegisterTestingT(t)
		// CredentialsValue() returns the raw string here; it is not valid JSON
		// so api.ConvertAuth's json.Unmarshal fails.
		tpl := &TemplateConfig{AccountConfig: AccountConfig{
			Credentials: api.Credentials{Credentials: "this-is-not-json"},
		}}
		_, err := ToAwsLambdaConfig(tpl, &api.StackConfigSingleImage{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert aws account config"))
	})
}

// ---- ToStaticSiteConfig -------------------------------------------------

func TestToStaticSiteConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps account + stack dir/name + base zone", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigStatic{
			BundleDir: "dist",
			Site: api.StaticSiteConfig{
				Domain:      "www.example.com",
				BaseDnsZone: "example.com",
			},
		}
		out, err := ToStaticSiteConfig(validTemplateConfig(), "/stacks/site", "site", stackCfg)
		Expect(err).ToNot(HaveOccurred())
		si, ok := out.(*StaticSiteInput)
		Expect(ok).To(BeTrue())
		Expect(si.StackDir).To(Equal("/stacks/site"))
		Expect(si.StackName).To(Equal("site"))
		Expect(si.Account).To(Equal("123456789012"))
		Expect(si.BundleDir).To(Equal("dist"))
		// OverriddenBaseZone reads StackConfigStatic.Site.BaseDnsZone.
		Expect(si.OverriddenBaseZone()).To(Equal("example.com"))
	})

	t.Run("wrong template type errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToStaticSiteConfig(42, "d", "n", &api.StackConfigStatic{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not of type aws.TemplateConfig"))
	})

	t.Run("nil template config errors", func(t *testing.T) {
		RegisterTestingT(t)
		var tpl *TemplateConfig
		_, err := ToStaticSiteConfig(tpl, "d", "n", &api.StackConfigStatic{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is nil"))
	})

	t.Run("nil stack config errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToStaticSiteConfig(validTemplateConfig(), "d", "n", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("stack config is nil"))
	})

	t.Run("non-JSON credentials make ConvertAuth fail", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := &TemplateConfig{AccountConfig: AccountConfig{
			Credentials: api.Credentials{Credentials: "not-json"},
		}}
		_, err := ToStaticSiteConfig(tpl, "d", "n", &api.StackConfigStatic{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert aws account config"))
	})
}

// ---- ToEcsFargateConfig -------------------------------------------------

// minimalProject builds a compose project with one ingress-labelled service.
func ingressService(name string) types.ServiceConfig {
	return types.ServiceConfig{
		Name:   name,
		Image:  "nginx:latest",
		Ports:  []types.ServicePortConfig{{Target: 8080}},
		Labels: map[string]string{api.ComposeLabelIngressContainer: "true"},
	}
}

func composeWith(services ...types.ServiceConfig) compose.Config {
	return compose.Config{Project: &types.Project{
		Name:         "proj",
		WorkingDir:   "/work",
		Services:     services,
		ComposeFiles: []string{"docker-compose.yaml"},
	}}
}

func TestToEcsFargateConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path: single run, default scale, ingress resolved", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs:        []string{"web"},
			Domain:      "app.example.com",
			BaseDnsZone: "example.com",
			Uses:        []string{"db"},
			Env:         map[string]string{"GLOBAL": "1"},
		}
		out, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in, ok := out.(*EcsFargateInput)
		Expect(ok).To(BeTrue())

		Expect(in.Config.Account).To(Equal("123456789012"))
		Expect(in.Domain).To(Equal("app.example.com"))
		Expect(in.BaseDnsZone).To(Equal("example.com"))
		Expect(in.ComposeDir).To(Equal("/work"))
		Expect(in.Containers).To(HaveLen(1))
		Expect(in.Containers[0].Name).To(Equal("web"))
		Expect(in.Containers[0].Port).To(Equal(8080))
		Expect(in.Containers[0].Image.Name).To(Equal("nginx:latest"))
		Expect(in.Containers[0].Image.Platform).To(Equal(api.ImagePlatformLinuxAmd64))
		// Global env merged in.
		Expect(in.Containers[0].Env).To(HaveKeyWithValue("GLOBAL", "1"))
		// Ingress container resolved by label.
		Expect(in.IngressContainer.Name).To(Equal("web"))

		// No stackCfg.Scale -> default Min 1 / Max 2, and rolling update set.
		Expect(in.Scale.Min).To(Equal(1))
		Expect(in.Scale.Max).To(Equal(2))
		Expect(in.Scale.Update.MinHealthyPercent).To(Equal(100))
		Expect(in.Scale.Update.MaxPercent).To(Equal(200))

		// getters
		Expect(in.Uses()).To(ConsistOf("db"))
		Expect(in.OverriddenBaseZone()).To(Equal("example.com"))
		Expect(in.DependsOnResources()).To(BeEmpty())
	})

	t.Run("dependencies propagate to DependsOnResources getter", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs:         []string{"web"},
			Dependencies: []api.StackConfigDependencyResource{{Name: "db", Owner: "other", Resource: "postgres"}},
		}
		out, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in := out.(*EcsFargateInput)
		Expect(in.DependsOnResources()).To(HaveLen(1))
		Expect(in.DependsOnResources()[0].Resource).To(Equal("postgres"))
	})

	t.Run("non-JSON credentials make ConvertAuth fail", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := &TemplateConfig{AccountConfig: AccountConfig{
			Credentials: api.Credentials{Credentials: "not-json"},
		}}
		_, err := ToEcsFargateConfig(tpl, composeWith(ingressService("web")), &api.StackConfigCompose{Runs: []string{"web"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert aws account config"))
	})

	t.Run("non-numeric memory size is rejected", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs: []string{"web"},
			Size: &api.StackConfigComposeSize{Cpu: "256", Memory: "lots"},
		}
		_, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ECS fargate memory size"))
	})

	// SOURCE QUIRK (ecs_fargate.go:159-160 vs 230): the `composeCfg.Project == nil`
	// guard at line 230 is dead code — Project is dereferenced much earlier at
	// lines 159 (`composeCfg.Project.WorkingDir`) and 160 (`...Project.Volumes`),
	// so a nil Project SEGV-panics before the guard is ever reached. We therefore
	// cannot cover line 230's error branch without crashing; it is asserted here
	// as a recovered panic to document the actual behavior.
	t.Run("nil compose project panics (dead nil-guard at line 230)", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(func() {
			_, _ = ToEcsFargateConfig(validTemplateConfig(), compose.Config{Project: nil}, &api.StackConfigCompose{})
		}).To(Panic())
	})

	// Errors raised inside the per-run loop return a zero-value EcsFargateInput
	// (NOT a pointer) alongside the error — assert that quirk holds.
	t.Run("run-loop port error returns zero-value struct", func(t *testing.T) {
		RegisterTestingT(t)
		// web is the ingress container but has no ports and no expose -> toRunPort errors.
		svc := types.ServiceConfig{
			Name:   "web",
			Image:  "nginx",
			Labels: map[string]string{api.ComposeLabelIngressContainer: "true"},
		}
		stackCfg := &api.StackConfigCompose{Runs: []string{"web"}}
		out, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(svc), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected 1 port"))
		// quirk: a value-typed EcsFargateInput{} is returned, not nil/pointer.
		_, isVal := out.(EcsFargateInput)
		Expect(isVal).To(BeTrue())
	})

	t.Run("run-loop cpu error from multi-run service deploy limit", func(t *testing.T) {
		RegisterTestingT(t)
		// >1 run forces the per-service deploy branch in toCpu; bad NanoCPUs -> error.
		web := ingressService("web")
		api2 := types.ServiceConfig{
			Name:  "api",
			Image: "api:latest",
			Ports: []types.ServicePortConfig{{Target: 9000}},
			Deploy: &types.DeployConfig{Resources: types.Resources{
				Limits: &types.Resource{NanoCPUs: "not-a-float"},
			}},
		}
		stackCfg := &api.StackConfigCompose{Runs: []string{"web", "api"}}
		_, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(web, api2), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse cpu limit"))
	})

	t.Run("explicit scale with cpu+memory policies and zero-min/max defaults", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs: []string{"web"},
			Scale: &api.StackConfigComposeScale{
				Min: 0, // -> defaults to 1
				Max: 0, // -> defaults to 1
				Policy: &api.StackConfigComposeScalePolicy{
					Cpu:    &api.StackConfigComposeScaleCpu{Max: 0},     // -> defaults to 70
					Memory: &api.StackConfigComposeScaleMemory{Max: 85}, // explicit
				},
			},
		}
		out, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in := out.(*EcsFargateInput)
		Expect(in.Scale.Min).To(Equal(1))
		Expect(in.Scale.Max).To(Equal(1))
		Expect(in.Scale.Policies).To(HaveLen(2))

		byType := map[EcsFargateScalePolicyType]EcsFargateScalePolicy{}
		for _, p := range in.Scale.Policies {
			byType[p.Type] = p
		}
		Expect(byType).To(HaveKey(ScaleCpu))
		Expect(byType).To(HaveKey(ScaleMemory))
		Expect(byType[ScaleCpu].TargetValue).To(Equal(70)) // defaulted
		Expect(byType[ScaleMemory].TargetValue).To(Equal(85))
		Expect(byType[ScaleCpu].ScaleInCooldown).To(Equal(60))
		Expect(byType[ScaleCpu].ScaleOutCooldown).To(Equal(60))
	})

	t.Run("size sets cpu/memory and validates ephemeral floor", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs: []string{"web"},
			Size: &api.StackConfigComposeSize{
				Cpu:       "512",
				Memory:    "1024",
				Ephemeral: "23622320128", // ~22GB -> >= 21
			},
		}
		out, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in := out.(*EcsFargateInput)
		Expect(in.Config.Cpu).To(Equal(512))
		Expect(in.Config.Memory).To(Equal(1024))
		Expect(in.Config.EphemeralStorageGB).To(Equal(22))
		// With a single run + Size, container cpu/memory come from Size (toCpu/toMemory).
		Expect(in.Containers[0].Cpu).To(Equal(512))
		Expect(in.Containers[0].Memory).To(Equal(1024))
	})

	// SOURCE BUG (ecs_fargate.go:177-178): the "must be above 21GB" guard does
	// `return nil, errors.Wrapf(err, ...)` where the inner err is already nil.
	// Per pkg/errors, Wrapf(nil, ...) returns nil, so the below-21GB case is
	// SILENTLY ACCEPTED — the function returns (nil, nil) instead of a useful
	// error. We assert the actual (buggy) behavior: no error AND a nil result.
	t.Run("ephemeral below 21GB returns (nil, nil) (source bug: Wrapf(nil))", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs: []string{"web"},
			Size: &api.StackConfigComposeSize{
				Cpu:       "256",
				Memory:    "512",
				Ephemeral: "1073741824", // 1GB, below the documented 21GB floor
			},
		}
		out, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).ToNot(HaveOccurred()) // should have errored, but Wrapf(nil) swallows it
		Expect(out).To(BeNil())           // and the result is the bare nil returned alongside it
	})

	t.Run("non-numeric ephemeral is rejected", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs: []string{"web"},
			Size: &api.StackConfigComposeSize{
				Cpu:       "256",
				Memory:    "512",
				Ephemeral: "notanumber",
			},
		}
		_, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ephemeral size"))
	})

	t.Run("non-numeric cpu size is rejected", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs: []string{"web"},
			Size: &api.StackConfigComposeSize{Cpu: "big", Memory: "512"},
		}
		_, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ECS fargate cpu size"))
	})

	t.Run("wrong template type errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToEcsFargateConfig("nope", composeWith(ingressService("web")), &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not of type aws.TemplateConfig"))
	})

	t.Run("nil template config errors", func(t *testing.T) {
		RegisterTestingT(t)
		var tpl *TemplateConfig
		_, err := ToEcsFargateConfig(tpl, composeWith(ingressService("web")), &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is nil"))
	})

	t.Run("zero ingress containers errors", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{Name: "web", Image: "nginx", Ports: []types.ServicePortConfig{{Target: 80}}}
		stackCfg := &api.StackConfigCompose{Runs: []string{"web"}}
		_, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(svc), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly 1 ingress container"))
	})

	t.Run("more than one ingress container errors", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{Runs: []string{"web", "api"}}
		_, err := ToEcsFargateConfig(validTemplateConfig(),
			composeWith(ingressService("web"), ingressService("api")), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly 1 ingress container"))
	})

	t.Run("service with neither image nor build errors", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{
			Name:   "web",
			Ports:  []types.ServicePortConfig{{Target: 80}},
			Labels: map[string]string{api.ComposeLabelIngressContainer: "true"},
		}
		stackCfg := &api.StackConfigCompose{Runs: []string{"web"}}
		_, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(svc), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("either `image` or `build` must be specified"))
	})

	t.Run("build context defaults dockerfile and maps args; mounted volume retained", func(t *testing.T) {
		RegisterTestingT(t)
		argVal := "v1"
		svc := types.ServiceConfig{
			Name: "web",
			Build: &types.BuildConfig{
				Context: "./app",
				Args:    types.MappingWithEquals{"ARG1": &argVal},
			},
			Ports:   []types.ServicePortConfig{{Target: 3000}},
			Volumes: []types.ServiceVolumeConfig{{Source: "data", Target: "/data", ReadOnly: true}},
			Labels:  map[string]string{api.ComposeLabelIngressContainer: "true"},
		}
		proj := composeWith(svc)
		proj.Project.Volumes = types.Volumes{
			"data":   types.VolumeConfig{Name: "data"},
			"unused": types.VolumeConfig{Name: "unused"},
		}
		stackCfg := &api.StackConfigCompose{Runs: []string{"web"}}
		out, err := ToEcsFargateConfig(validTemplateConfig(), proj, stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in := out.(*EcsFargateInput)
		Expect(in.Containers[0].Image.Context).To(Equal("./app"))
		Expect(in.Containers[0].Image.Dockerfile).To(Equal("Dockerfile")) // defaulted
		Expect(in.Containers[0].Image.Build.Args).To(HaveKeyWithValue("ARG1", "v1"))
		Expect(in.Containers[0].MountPoints).To(HaveLen(1))
		Expect(in.Containers[0].MountPoints[0].SourceVolume).To(Equal("data"))
		// Only the mounted volume survives the filter.
		Expect(in.Volumes).To(HaveLen(1))
		Expect(in.Volumes[0].Name).To(Equal("data"))
	})

	t.Run("malformed cloudExtras (type mismatch) errors on conversion", func(t *testing.T) {
		RegisterTestingT(t)
		// awsRoles must be a list; a scalar makes yaml.Unmarshal into
		// *CloudExtras fail inside api.ConvertDescriptor.
		var extras any = map[string]any{"awsRoles": "should-be-a-list"}
		stackCfg := &api.StackConfigCompose{
			Runs:        []string{"web"},
			CloudExtras: &extras,
		}
		_, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert cloudExtras"))
	})

	t.Run("cloudExtras converted into AWS CloudExtras", func(t *testing.T) {
		RegisterTestingT(t)
		var extras any = map[string]any{
			"awsRoles":          []any{"role-a", "role-b"},
			"lambdaRoutingType": "function-url",
		}
		stackCfg := &api.StackConfigCompose{
			Runs:        []string{"web"},
			CloudExtras: &extras,
		}
		out, err := ToEcsFargateConfig(validTemplateConfig(), composeWith(ingressService("web")), stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in := out.(*EcsFargateInput)
		Expect(in.CloudExtras).ToNot(BeNil())
		Expect(in.CloudExtras.AwsRoles).To(ConsistOf("role-a", "role-b"))
		Expect(string(in.CloudExtras.LambdaRoutingType)).To(Equal("function-url"))
	})
}

// ---- EcsFargateConfig getters -------------------------------------------

func TestEcsFargateConfig_Getters(t *testing.T) {
	RegisterTestingT(t)
	cfg := &EcsFargateConfig{
		AccountConfig: AccountConfig{Account: "123456789012", Region: "us-east-1"},
		Cpu:           256,
		Memory:        512,
	}
	Expect(cfg.ProjectIdValue()).To(Equal("123456789012"))

	// CredentialsValue marshals the whole EcsFargateConfig to JSON.
	cv := cfg.CredentialsValue()
	Expect(cv).To(ContainSubstring(`"account":"123456789012"`))
	Expect(cv).To(ContainSubstring(`"cpu":256`))
	Expect(cv).To(ContainSubstring(`"memory":512`))
}

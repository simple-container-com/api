// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

// TestComposeTemplateRegistered pins the template type string and BOTH converters, by
// driving the same entry points `sc deploy` uses (an unregistered converter fails there
// with "incompatible server template type", api/client.go:394,420).
//
// Both converters, because the first stack this delivery path has to carry
// (forge-atrium) is `type: single-image`; a template registered for `cloud-compose` only
// would fail that stack in a way that reads like a broken installation.
func TestComposeTemplateRegistered(t *testing.T) {
	RegisterTestingT(t)

	Expect(api.GetRegisteredProviderConfigs()).To(HaveKey(TemplateTypeVmCompose))

	ctx := context.Background()
	tpl := api.StackDescriptor{
		Type:   TemplateTypeVmCompose,
		Config: api.Config{Config: map[string]any{"vmResource": "vm-ru"}},
	}

	single, err := api.PrepareCloudSingleImageForDeploy(ctx, "", "atrium", tpl, &api.StackConfigSingleImage{
		Image: &api.ContainerImage{Name: "simplecontainer/atrium:latest"},
	}, "infra")
	Expect(err).ToNot(HaveOccurred())
	Expect(single.Type).To(Equal(TemplateTypeVmCompose))
	Expect(single.Config.Config).To(BeAssignableToTypeOf(&VmComposeInput{}))

	compose, err := api.PrepareCloudComposeForDeploy(ctx, "testdata/stacks/refapp", "refapp", tpl, &api.StackConfigCompose{
		DockerComposeFile: "docker-compose.yaml",
		Runs:              []string{"backend"},
	}, "infra")
	Expect(err).ToNot(HaveOccurred())
	Expect(compose.Config.Config).To(BeAssignableToTypeOf(&VmComposeInput{}))
}

// The same constraint as TestReadVmConfigAcceptsEmptyConfig: cmd/schema-gen drives every
// registered reader with an EMPTY config and silently drops the ones that error, so a
// validating reader would delete `vm-compose` from the generated schema with no failure
// anywhere to explain it.
func TestReadComposeTemplateConfigAcceptsEmptyConfig(t *testing.T) {
	RegisterTestingT(t)

	out, err := ReadComposeTemplateConfig(&api.Config{Config: make(map[string]any)})
	Expect(err).ToNot(HaveOccurred())

	cfg, ok := out.Config.(*ComposeTemplateConfig)
	Expect(ok).To(BeTrue())
	// And the same config does not survive the real gate.
	Expect(cfg.Validate()).To(HaveOccurred())
}

func TestReadComposeTemplateConfig(t *testing.T) {
	RegisterTestingT(t)

	out, err := ReadComposeTemplateConfig(&api.Config{Config: map[string]any{
		"vmResource":    "vm-ru",
		"imageDelivery": ImageDeliveryRegistry,
		"strategy":      StrategyRecreate,
		"keepImages":    0,
		// Inlined from docker.RegistryCredentials — the registry delivery path needs it.
		"dockerRegistryURL": "cr.cloud.ru/forge",
	}})
	Expect(err).ToNot(HaveOccurred())

	cfg, ok := out.Config.(*ComposeTemplateConfig)
	Expect(ok).To(BeTrue())
	Expect(cfg.VmResource).To(Equal("vm-ru"))
	Expect(cfg.EffectiveImageDelivery()).To(Equal(ImageDeliveryRegistry))
	Expect(cfg.EffectiveStrategy()).To(Equal(StrategyRecreate))
	// 0 is a valid explicit choice ("keep none") and must not fall back to the default.
	Expect(cfg.EffectiveKeepImages()).To(Equal(0))
	Expect(cfg.Validate()).ToNot(HaveOccurred())
}

func TestComposeTemplateDefaults(t *testing.T) {
	RegisterTestingT(t)

	cfg := &ComposeTemplateConfig{VmResource: "vm-ru"}
	// ssh-tar, because "no registry at all" is the reason this delivery path exists.
	Expect(cfg.EffectiveImageDelivery()).To(Equal(ImageDeliverySshTar))
	Expect(cfg.EffectiveStrategy()).To(Equal(StrategyBlueGreen))
	Expect(cfg.EffectiveKeepImages()).To(Equal(DefaultKeepImages))
	Expect(cfg.Validate()).ToNot(HaveOccurred())
}

func TestComposeTemplateValidate(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		name string
		cfg  ComposeTemplateConfig
		msg  string
	}{
		{
			name: "vmResource is the only required field",
			cfg:  ComposeTemplateConfig{},
			msg:  "requires `vmResource`",
		},
		{
			name: "unknown imageDelivery",
			cfg:  ComposeTemplateConfig{VmResource: "vm-ru", ImageDelivery: "rsync"},
			msg:  `unknown ` + "`imageDelivery`" + ` "rsync"`,
		},
		{
			name: "unknown strategy",
			cfg:  ComposeTemplateConfig{VmResource: "vm-ru", Strategy: "canary"},
			msg:  `unknown ` + "`strategy`" + ` "canary"`,
		},
		{
			name: "negative keepImages",
			cfg:  ComposeTemplateConfig{VmResource: "vm-ru", KeepImages: lo.ToPtr(-1)},
			msg:  "must not be negative",
		},
		{
			// Refused here rather than at `docker pull` time on the machine, where it
			// surfaces as an unauthenticated pull of a bare image name.
			name: "registry delivery without a registry",
			cfg:  ComposeTemplateConfig{VmResource: "vm-ru", ImageDelivery: ImageDeliveryRegistry},
			msg:  "requires `dockerRegistryURL`",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			Expect(cfg.Validate()).To(MatchError(ContainSubstring(tt.msg)))
		})
	}
}

// Which services are stateful decides which ones blue-green may duplicate — so the
// classification is read-only mounts out, named volumes in, tmpfs never.
func TestStatefulServicesAndVolumes(t *testing.T) {
	RegisterTestingT(t)

	input := &VmComposeInput{Mounts: []VmMount{
		{Service: "mongo", Type: MountTypeVolume, Name: "mongo-data", MountPath: "/data/db"},
		{Service: "worker", Type: MountTypeVolume, Name: "mongo-data", MountPath: "/data/db", ReadOnly: true},
		{Service: "backend", Type: MountTypeTmpfs, MountPath: "/scratch", Size: "67108864"},
		{Service: "redis", Type: MountTypeVolume, Name: "redis-data", MountPath: "/data"},
	}}

	Expect(input.StatefulServices()).To(Equal([]string{"mongo", "redis"}))
	// Sorted: the list goes into a warning message and into the render order.
	Expect(input.ExternalVolumes()).To(Equal([]string{"mongo-data", "redis-data"}))
	Expect(input.MountsOf("backend")).To(HaveLen(1))
	Expect(input.MountsOfVolume("mongo-data")).To(HaveLen(2))
	// A tmpfs is never a named volume, so it never becomes an external docker volume.
	Expect(input.MountsOfVolume("")).To(BeEmpty())
}

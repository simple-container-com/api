// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"context"
	"testing"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func testTemplate() *ComposeTemplateConfig {
	return &ComposeTemplateConfig{VmResource: "vm-ru"}
}

func readTestCompose(t *testing.T) compose.Config {
	t.Helper()
	cfg, err := compose.ReadDockerCompose(context.Background(), "testdata/stacks/refapp", "docker-compose.yaml")
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg.Project).ToNot(BeNil())
	return cfg
}

func composeOf(services ...types.ServiceConfig) compose.Config {
	return compose.Config{Project: &types.Project{Services: services}}
}

func toInput(got any) *VmComposeInput {
	input, ok := got.(*VmComposeInput)
	Expect(ok).To(BeTrue())
	return input
}

// The happy path, end to end from the compose file: who is stateful, which depends_on
// edges survive, which healthcheck is carried verbatim, and what the ingress is.
func TestToVmComposeConfig(t *testing.T) {
	RegisterTestingT(t)

	got, err := ToVmComposeConfig(testTemplate(), readTestCompose(t), &api.StackConfigCompose{
		Domain: "refapp.example.com",
		Runs:   []string{"backend", "mongo", "worker"},
		Env:    map[string]string{"NODE_ENV": "production"},
	})
	Expect(err).ToNot(HaveOccurred())
	input := toInput(got)

	Expect(input.Deployment.Containers).To(HaveLen(3))
	Expect(input.Deployment.IngressContainer).ToNot(BeNil())
	Expect(input.Deployment.IngressContainer.Name).To(Equal("backend"))
	Expect(lo.FromPtr(input.Deployment.IngressContainer.MainPort)).To(Equal(8080))

	// Mongo writes to a named volume, so it must run outside the blue-green slots.
	// Worker mounts the SAME volume read-only, which does not make it stateful.
	Expect(input.StatefulServices()).To(Equal([]string{"mongo"}))
	Expect(input.ExternalVolumes()).To(Equal([]string{"mongo-data"}))

	// A tmpfs is a mount with a size and no name, and is never external.
	tmp, found := lo.Find(input.Mounts, func(m VmMount) bool { return m.Type == MountTypeTmpfs })
	Expect(found).To(BeTrue())
	Expect(tmp.Service).To(Equal("backend"))
	Expect(tmp.MountPath).To(Equal("/scratch"))
	Expect(tmp.Size).To(Equal("67108864"))

	// Only edges inside `runs` survive; `mailer` is not run, so its edge is dropped
	// with a warning rather than left to wedge `docker compose up`.
	Expect(input.DependsOn["backend"]).To(Equal([]VmDependsOn{{Service: "mongo", Condition: "service_healthy"}}))
	Expect(input.Warnings).To(ContainElement(ContainSubstring(`depends_on "mailer"`)))

	// The healthcheck is carried as written. ConvertComposeToContainers turns it into a
	// Kubernetes HTTP probe and loses the test command, which on a VM IS the check.
	Expect(input.HealthChecks["backend"]).To(Equal(VmHealthCheck{
		Test:     []string{"CMD", "curl", "-f", "http://localhost:8080/health"},
		Interval: "15s",
		Timeout:  "5s",
		Retries:  lo.ToPtr(4),
	}))

	// A service with `image:` and no build keeps a pullable reference. The Kubernetes
	// path substitutes the SERVICE NAME here, which would deploy image `mongo` if the
	// service were called `mongodb`.
	mongo, found := lo.Find(input.Deployment.Containers, func(c k8s.CloudRunContainer) bool { return c.Name == "mongo" })
	Expect(found).To(BeTrue())
	Expect(mongo.Image.Name).To(Equal("mongo:7"))
	// A built service keeps an empty name on purpose: the deploy must pass what it built.
	backend, _ := lo.Find(input.Deployment.Containers, func(c k8s.CloudRunContainer) bool { return c.Name == "backend" })
	Expect(backend.Image.Name).To(BeEmpty())
	// The loader resolves the build context against the stack directory, so what the
	// deploy builds is an absolute path and not the `./backend` of the compose file.
	Expect(backend.Image.Context).To(HaveSuffix("/testdata/stacks/refapp/backend"))

	// Stateful services exist and the strategy is the default blue-green, so the
	// operator is told the flip will not redeploy them.
	Expect(input.Warnings).To(ContainElement(ContainSubstring("outside the blue-green slots")))
}

// The §2.2 table: a field is either supported or refused. A field accepted and silently
// ignored is a future incident where the operator is certain the setting applied.
func TestToVmComposeConfigRefusals(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		name     string
		stackCfg *api.StackConfigCompose
		msg      string
	}{
		{
			name: "domainProxied breaks ACME",
			stackCfg: &api.StackConfigCompose{
				Runs: []string{"backend"}, DomainProxied: lo.ToPtr(true),
			},
			msg: "`domainProxied` is not supported",
		},
		{
			// Even `false` is refused: accepting it would imply SC understood the field.
			name: "domainProxied false is still refused",
			stackCfg: &api.StackConfigCompose{
				Runs: []string{"backend"}, DomainProxied: lo.ToPtr(false),
			},
			msg: "`domainProxied` is not supported",
		},
		{
			name: "clusterIPAddress has no cluster",
			stackCfg: &api.StackConfigCompose{
				Runs: []string{"backend"}, ClusterIPAddress: lo.ToPtr("10.0.0.1"),
			},
			msg: "there is no cluster",
		},
		{
			name: "scale.max > 1 needs more than one node",
			stackCfg: &api.StackConfigCompose{
				Runs: []string{"backend"}, Scale: &api.StackConfigComposeScale{Min: 1, Max: 3},
			},
			msg: "move the stack to a k3s/Kubernetes parent",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ToVmComposeConfig(testTemplate(), readTestCompose(t), tt.stackCfg)
			Expect(err).To(MatchError(ContainSubstring(tt.msg)))
		})
	}
}

// Accepted, but the operator is told what SC did instead — the other half of §2.2.
func TestToVmComposeConfigWarnings(t *testing.T) {
	RegisterTestingT(t)

	got, err := ToVmComposeConfig(testTemplate(), readTestCompose(t), &api.StackConfigCompose{
		Runs:            []string{"backend"},
		Scale:           &api.StackConfigComposeScale{Min: 0, Max: 1},
		StaticEgressIP:  lo.ToPtr(true),
		ImagePullPolicy: lo.ToPtr("Always"),
		CloudExtras:     lo.ToPtr[any](map[string]any{"nodeSelector": map[string]any{"disk": "ssd"}}),
		Alerts:          &api.AlertsConfig{},
	})
	Expect(err).ToNot(HaveOccurred())

	warnings := toInput(got).Warnings
	Expect(warnings).To(ContainElement(ContainSubstring("`scale.min: 0` has no effect")))
	Expect(warnings).To(ContainElement(ContainSubstring("`staticEgressIP: true` is already true on a VM")))
	// imagePullPolicy is only meaningless because the default delivery streams the
	// image over SSH rather than pulling it.
	Expect(warnings).To(ContainElement(ContainSubstring("`imagePullPolicy: Always` is ignored")))
	Expect(warnings).To(ContainElement(ContainSubstring("`cloudExtras` is ignored")))
	Expect(warnings).To(ContainElement(ContainSubstring("`alerts` is not wired")))
}

func TestToVmComposeConfigMountRefusals(t *testing.T) {
	RegisterTestingT(t)

	for _, tt := range []struct {
		name   string
		volume types.ServiceVolumeConfig
		msg    string
	}{
		{
			name:   "bind mount points at the deploy machine",
			volume: types.ServiceVolumeConfig{Type: types.VolumeTypeBind, Source: "./conf", Target: "/etc/app"},
			msg:    "the source is a path on the machine that ran `sc deploy`",
		},
		{
			// An anonymous volume belongs to the slot's container, so the next deploy
			// tears it down and the data goes with it.
			name:   "anonymous volume cannot survive a slot teardown",
			volume: types.ServiceVolumeConfig{Type: types.VolumeTypeVolume, Target: "/data"},
			msg:    "mounts an anonymous volume",
		},
		{
			name:   "unsupported volume type",
			volume: types.ServiceVolumeConfig{Type: types.VolumeTypeNamedPipe, Source: "pipe", Target: "/pipe"},
			msg:    "unsupported volume type",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := composeOf(types.ServiceConfig{
				Name:    "backend",
				Image:   "backend:1",
				Ports:   []types.ServicePortConfig{{Target: 8080, Protocol: "tcp"}},
				Volumes: []types.ServiceVolumeConfig{tt.volume},
			})
			_, err := ToVmComposeConfig(testTemplate(), cfg, &api.StackConfigCompose{Runs: []string{"backend"}})
			Expect(err).To(MatchError(ContainSubstring(tt.msg)))
		})
	}
}

// The ingress service's healthcheck is the blue-green gate, so there always is one.
// A synthesized TCP check is weak and the operator is told so — flipping Caddy onto an
// upstream nobody checked is worse.
func TestToVmComposeConfigSynthesizesIngressHealthCheck(t *testing.T) {
	RegisterTestingT(t)

	cfg := composeOf(types.ServiceConfig{
		Name:  "backend",
		Image: "backend:1",
		Ports: []types.ServicePortConfig{{Target: 8080, Protocol: "tcp"}},
	})
	got, err := ToVmComposeConfig(testTemplate(), cfg, &api.StackConfigCompose{Runs: []string{"backend"}})
	Expect(err).ToNot(HaveOccurred())
	input := toInput(got)

	Expect(input.HealthChecks["backend"]).To(Equal(VmHealthCheck{
		Test:        []string{"CMD-SHELL", "nc -z localhost 8080 || exit 1"},
		Interval:    "10s",
		Timeout:     "3s",
		StartPeriod: "20s",
		Retries:     lo.ToPtr(3),
		Synthesized: true,
	}))
	Expect(input.Warnings).To(ContainElement(ContainSubstring("declares no `healthcheck`")))
}

func TestToVmComposeConfigIngressRules(t *testing.T) {
	RegisterTestingT(t)

	noPorts := composeOf(types.ServiceConfig{Name: "worker", Image: "worker:1"})

	// A domain with nothing to serve it is a deploy that comes up and answers nothing.
	_, err := ToVmComposeConfig(testTemplate(), noPorts, &api.StackConfigCompose{
		Runs: []string{"worker"}, Domain: "refapp.example.com",
	})
	Expect(err).To(MatchError(ContainSubstring(api.ComposeLabelIngressContainer)))

	// Without a domain the same stack is valid — a worker-only stack is a real shape —
	// but the operator is told nothing is reachable.
	got, err := ToVmComposeConfig(testTemplate(), noPorts, &api.StackConfigCompose{Runs: []string{"worker"}})
	Expect(err).ToNot(HaveOccurred())
	Expect(toInput(got).Warnings).To(ContainElement(ContainSubstring("no ingress container")))
}

// Blue-green needs two copies of the ingress service at once; two writers on one volume
// corrupt it. The two requirements are incompatible, so SC refuses and names the way out.
func TestToVmComposeConfigStatefulIngressUnderBlueGreen(t *testing.T) {
	RegisterTestingT(t)

	cfg := composeOf(types.ServiceConfig{
		Name:  "backend",
		Image: "backend:1",
		Ports: []types.ServicePortConfig{{Target: 8080, Protocol: "tcp"}},
		Volumes: []types.ServiceVolumeConfig{
			{Type: types.VolumeTypeVolume, Source: "uploads", Target: "/data"},
		},
	})
	stackCfg := &api.StackConfigCompose{Runs: []string{"backend"}}

	_, err := ToVmComposeConfig(testTemplate(), cfg, stackCfg)
	Expect(err).To(MatchError(ContainSubstring("`strategy: blue-green` cannot work")))
	Expect(err).To(MatchError(ContainSubstring("uploads")))

	// `recreate` is the operator's way out, and it is accepted.
	tpl := testTemplate()
	tpl.Strategy = StrategyRecreate
	got, err := ToVmComposeConfig(tpl, cfg, stackCfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(toInput(got).StatefulServices()).To(Equal([]string{"backend"}))
}

func TestToVmComposeConfigRejectsBadInput(t *testing.T) {
	RegisterTestingT(t)

	_, err := ToVmComposeConfig(&ComposeTemplateConfig{}, readTestCompose(t), &api.StackConfigCompose{})
	Expect(err).To(MatchError(ContainSubstring("requires `vmResource`")))

	_, err = ToVmComposeConfig(testTemplate(), compose.Config{}, &api.StackConfigCompose{})
	Expect(err).To(MatchError(ContainSubstring("`dockerComposeFile` is required")))

	_, err = ToVmComposeConfig(testTemplate(), readTestCompose(t), nil)
	Expect(err).To(MatchError(ContainSubstring("stack config cannot be nil")))

	_, err = ToVmComposeConfig(&VmConfig{}, readTestCompose(t), &api.StackConfigCompose{})
	Expect(err).To(MatchError(ContainSubstring("not of type *vm.ComposeTemplateConfig")))
}

// forge-atrium is `type: single-image`, so this projection is what carries the first
// live stand. It is total on purpose: everything downstream sees one shape.
func TestToVmComposeSingleImageConfig(t *testing.T) {
	RegisterTestingT(t)

	got, err := ToVmComposeSingleImageConfig(testTemplate(), &api.StackConfigSingleImage{
		Image:     &api.ContainerImage{Name: "simplecontainer/atrium:2026.10.16-abc1234"},
		Domain:    "atriumdev.ru",
		BasePath:  "/portal/",
		Env:       map[string]string{"NODE_ENV": "production"},
		Secrets:   map[string]string{"API_KEY": "x"},
		MaxMemory: lo.ToPtr(1024),
		Min:       1,
		Max:       1,
	})
	Expect(err).ToNot(HaveOccurred())
	input := toInput(got)

	Expect(input.SingleImage).To(BeTrue())
	Expect(input.Deployment.Containers).To(HaveLen(1))
	container := input.Deployment.Containers[0]
	Expect(container.Name).To(Equal(SingleImageService))
	Expect(container.Image.Name).To(Equal("simplecontainer/atrium:2026.10.16-abc1234"))
	// StackConfigSingleImage has no ports field — on Lambda and YC the runtime decides.
	// On a VM somebody has to pick, so SC picks and passes it in PORT.
	Expect(lo.FromPtr(container.MainPort)).To(Equal(DefaultSingleImagePort))
	Expect(container.Env).To(HaveKeyWithValue(EnvPort, "8080"))
	Expect(container.Image.Platform).To(Equal(api.ImagePlatformLinuxAmd64))

	// The ingress container is the same container, so the Caddy site and the health
	// gate have something to point at.
	Expect(input.Deployment.IngressContainer).ToNot(BeNil())
	Expect(input.Deployment.IngressContainer.Name).To(Equal(SingleImageService))

	// basePath is the path the application itself serves under, so the prefix has to
	// survive the proxy hop.
	Expect(input.Deployment.StackConfig.Prefix).To(Equal("/portal"))
	Expect(input.Deployment.StackConfig.ProxyKeepPrefix).To(BeTrue())
	Expect(input.Deployment.StackConfig.Domain).To(Equal("atriumdev.ru"))
	Expect(input.Deployment.StackConfig.Secrets).To(HaveKey("API_KEY"))
	// maxMemory is MiB on every target that has it, and `size.memory` is a bare MB
	// number — k8s.toMemoryLimit parses it with strconv.Atoi, so a Kubernetes quantity
	// here would fail conversion with a strconv error.
	Expect(input.Deployment.StackConfig.Size.Memory).To(Equal("1024"))
	Expect(container.Resources.Limits).To(HaveKeyWithValue("memory", "1Gi"))

	// A single-image stack has no compose file, so there is nothing stateful and no
	// depends_on — but the ingress health gate still exists.
	Expect(input.StatefulServices()).To(BeEmpty())
	Expect(input.HealthChecks[SingleImageService].Synthesized).To(BeTrue())
}

func TestToVmComposeSingleImageConfigRefusalsAndWarnings(t *testing.T) {
	RegisterTestingT(t)

	_, err := ToVmComposeSingleImageConfig(testTemplate(), &api.StackConfigSingleImage{})
	Expect(err).To(MatchError(ContainSubstring("`image` is required")))

	// `min`/`max` project onto `scale`, so the one-replica-per-machine rule applies to
	// a single-image stack through the same gate.
	_, err = ToVmComposeSingleImageConfig(testTemplate(), &api.StackConfigSingleImage{
		Image: &api.ContainerImage{Name: "atrium:1"},
		Min:   1, Max: 3,
	})
	Expect(err).To(MatchError(ContainSubstring("move the stack to a k3s/Kubernetes parent")))

	got, err := ToVmComposeSingleImageConfig(testTemplate(), &api.StackConfigSingleImage{
		Image:               &api.ContainerImage{Name: "atrium:1"},
		Timeout:             lo.ToPtr(30),
		MaxEphemeralStorage: lo.ToPtr(2048),
	})
	Expect(err).ToNot(HaveOccurred())
	warnings := toInput(got).Warnings
	// Both fields describe a function-runtime limit that has no counterpart on a VM.
	Expect(warnings).To(ContainElement(ContainSubstring("`timeout: 30` is not supported on a VM")))
	Expect(warnings).To(ContainElement(ContainSubstring("`maxEphemeralStorage: 2048` is not supported on a VM")))
}

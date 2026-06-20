package aws

import (
	"testing"
	"time"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func durPtr(d time.Duration) *types.Duration {
	v := types.Duration(d)
	return &v
}

func uint64Ptr(v uint64) *uint64 { return &v }

// ---- bytesToGB ----------------------------------------------------------

func TestBytesToGB(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name  string
		bytes int
		want  int
	}{
		{name: "exactly 1GB", bytes: 1024 * 1024 * 1024, want: 1},
		{name: "22GB", bytes: 22 * 1024 * 1024 * 1024, want: 22},
		{name: "below 1GB rounds down to 0", bytes: 500 * 1024 * 1024, want: 0},
		{name: "zero", bytes: 0, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(bytesToGB(tc.bytes)).To(Equal(tc.want))
		})
	}
}

// ---- toRunPort ----------------------------------------------------------

func TestToRunPort(t *testing.T) {
	RegisterTestingT(t)

	t.Run("single port", func(t *testing.T) {
		RegisterTestingT(t)
		p, err := toRunPort([]types.ServicePortConfig{{Target: 8080}}, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(p).To(Equal(8080))
	})

	t.Run("single expose entry when no ports", func(t *testing.T) {
		RegisterTestingT(t)
		p, err := toRunPort(nil, types.StringOrNumberList{"9090"})
		Expect(err).ToNot(HaveOccurred())
		Expect(p).To(Equal(9090))
	})

	t.Run("non-numeric expose errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := toRunPort(nil, types.StringOrNumberList{"http"})
		Expect(err).To(HaveOccurred())
	})

	t.Run("zero ports and zero expose errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := toRunPort(nil, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected 1 port"))
	})

	t.Run("multiple ports errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := toRunPort([]types.ServicePortConfig{{Target: 80}, {Target: 443}}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected 1 port"))
	})

	t.Run("single port takes precedence over expose", func(t *testing.T) {
		RegisterTestingT(t)
		p, err := toRunPort([]types.ServicePortConfig{{Target: 80}}, types.StringOrNumberList{"9090"})
		Expect(err).ToNot(HaveOccurred())
		Expect(p).To(Equal(80))
	})
}

// ---- toCpu --------------------------------------------------------------

func TestToCpu(t *testing.T) {
	RegisterTestingT(t)

	t.Run("single run with Size uses Size.Cpu", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web"}, Size: &api.StackConfigComposeSize{Cpu: "1024"}}
		v, err := toCpu(cfg, types.ServiceConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(1024))
	})

	t.Run("single run with non-numeric Size.Cpu errors", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web"}, Size: &api.StackConfigComposeSize{Cpu: "huge"}}
		_, err := toCpu(cfg, types.ServiceConfig{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse cpu value"))
	})

	t.Run("deploy NanoCPUs converted to 1024-scale", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web", "api"}} // >1 run -> skip Size branch
		svc := types.ServiceConfig{Deploy: &types.DeployConfig{Resources: types.Resources{
			Limits: &types.Resource{NanoCPUs: "0.5"},
		}}}
		v, err := toCpu(cfg, svc)
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(512)) // 1024 * 0.5
	})

	t.Run("invalid deploy NanoCPUs errors", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web", "api"}}
		svc := types.ServiceConfig{Name: "web", Deploy: &types.DeployConfig{Resources: types.Resources{
			Limits: &types.Resource{NanoCPUs: "lots"},
		}}}
		_, err := toCpu(cfg, svc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse cpu limit"))
	})

	t.Run("no Size, no deploy -> default 256", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web", "api"}}
		v, err := toCpu(cfg, types.ServiceConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(256))
	})
}

// ---- toMemory -----------------------------------------------------------

func TestToMemory(t *testing.T) {
	RegisterTestingT(t)

	t.Run("single run with Size uses Size.Memory", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web"}, Size: &api.StackConfigComposeSize{Memory: "2048"}}
		v, err := toMemory(cfg, types.ServiceConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(2048))
	})

	t.Run("single run with non-numeric Size.Memory errors", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web"}, Size: &api.StackConfigComposeSize{Memory: "lots"}}
		_, err := toMemory(cfg, types.ServiceConfig{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse memory value"))
	})

	t.Run("deploy MemoryBytes converted to MB", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web", "api"}}
		svc := types.ServiceConfig{Deploy: &types.DeployConfig{Resources: types.Resources{
			Limits: &types.Resource{MemoryBytes: types.UnitBytes(512 * 1024 * 1024)},
		}}}
		v, err := toMemory(cfg, svc)
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(512))
	})

	t.Run("no Size, no deploy -> default 512", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"web", "api"}}
		v, err := toMemory(cfg, types.ServiceConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(512))
	})
}

// ---- toMountPoints ------------------------------------------------------

func TestToMountPoints(t *testing.T) {
	RegisterTestingT(t)

	t.Run("maps volumes 1:1", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{Volumes: []types.ServiceVolumeConfig{
			{Source: "data", Target: "/data", ReadOnly: true},
			{Source: "cache", Target: "/cache", ReadOnly: false},
		}}
		mps := toMountPoints(svc)
		Expect(mps).To(HaveLen(2))
		Expect(mps[0].SourceVolume).To(Equal("data"))
		Expect(mps[0].ContainerPath).To(Equal("/data"))
		Expect(mps[0].ReadOnly).To(BeTrue())
		Expect(mps[1].SourceVolume).To(Equal("cache"))
		Expect(mps[1].ReadOnly).To(BeFalse())
	})

	t.Run("no volumes -> empty", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(toMountPoints(types.ServiceConfig{})).To(BeEmpty())
	})
}

// ---- toDependsOn --------------------------------------------------------

func TestToDependsOn(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		condition string
		want      string
	}{
		{name: "service_healthy -> HEALTHY", condition: "service_healthy", want: "HEALTHY"},
		{name: "service_started -> START", condition: "service_started", want: "START"},
		{name: "unknown condition -> HEALTHY (default)", condition: "service_completed_successfully", want: "HEALTHY"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			on := types.DependsOnConfig{"db": types.ServiceDependency{Condition: tc.condition}}
			res := toDependsOn(on)
			Expect(res).To(HaveLen(1))
			Expect(res[0].Container).To(Equal("db"))
			Expect(res[0].Condition).To(Equal(tc.want))
		})
	}

	t.Run("empty depends-on -> empty slice", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(toDependsOn(types.DependsOnConfig{})).To(BeEmpty())
	})
}

// ---- toRunEnv -----------------------------------------------------------

func TestToRunEnv(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil-valued entries are dropped, set ones kept", func(t *testing.T) {
		RegisterTestingT(t)
		a := "1"
		b := "two"
		env := types.MappingWithEquals{
			"A":       &a,
			"B":       &b,
			"DROPPED": nil, // no value -> excluded
		}
		res := toRunEnv(env)
		Expect(res).To(HaveLen(2))
		Expect(res).To(HaveKeyWithValue("A", "1"))
		Expect(res).To(HaveKeyWithValue("B", "two"))
		Expect(res).ToNot(HaveKey("DROPPED"))
	})

	t.Run("empty mapping -> empty result", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(toRunEnv(types.MappingWithEquals{})).To(BeEmpty())
	})
}

// ---- toRunSecrets -------------------------------------------------------

// toRunSecrets is currently a stub (the ${secret:...} feature is unimplemented):
// it always returns an empty, non-nil map and no error regardless of input.
func TestToRunSecrets(t *testing.T) {
	RegisterTestingT(t)
	val := "x"
	res, err := toRunSecrets(types.MappingWithEquals{"SECRET": &val})
	Expect(err).ToNot(HaveOccurred())
	Expect(res).ToNot(BeNil())
	Expect(res).To(BeEmpty())
}

// ---- FromHealthCheck / toStartupProbe / toLivenessProbe ------------------

func TestFromHealthCheck(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil healthcheck leaves probe zero", func(t *testing.T) {
		RegisterTestingT(t)
		p := EcsFargateProbe{}
		p.FromHealthCheck(types.ServiceConfig{}, 8080)
		Expect(p).To(Equal(EcsFargateProbe{}))
	})

	t.Run("durations, retries and Test command captured", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{HealthCheck: &types.HealthCheckConfig{
			Test:        types.HealthCheckTest{"CMD", "curl", "localhost"},
			Interval:    durPtr(30 * time.Second),
			Timeout:     durPtr(5 * time.Second),
			StartPeriod: durPtr(10 * time.Second),
			Retries:     uint64Ptr(4),
		}}
		p := EcsFargateProbe{}
		p.FromHealthCheck(svc, 8080)
		Expect(p.IntervalSeconds).To(Equal(30))
		Expect(p.TimeoutSeconds).To(Equal(5))
		Expect(p.InitialDelaySeconds).To(Equal(10))
		Expect(p.Retries).To(Equal(4))
		Expect(p.Command).To(Equal([]string{"CMD", "curl", "localhost"}))
		// With a Test command present, HttpGet stays empty.
		Expect(p.HttpGet).To(Equal(ProbeHttpGet{}))
	})

	t.Run("no Test command -> default HTTP GET on / and the port", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{HealthCheck: &types.HealthCheckConfig{
			Interval: durPtr(15 * time.Second),
		}}
		p := EcsFargateProbe{}
		p.FromHealthCheck(svc, 8080)
		Expect(p.Command).To(BeEmpty())
		Expect(p.HttpGet.Path).To(Equal("/"))
		Expect(p.HttpGet.Port).To(Equal(8080))
	})

	t.Run("healthcheck labels override path/port/success/threshold", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{
			HealthCheck: &types.HealthCheckConfig{}, // no Test -> HttpGet default branch
			Labels: map[string]string{
				api.ComposeLabelHealthcheckPath:             "/healthz",
				api.ComposeLabelHealthcheckPort:             "9000",
				api.ComposeLabelHealthcheckSuccessCodes:     "200-299",
				api.ComposeLabelHealthcheckHealthyThreshold: "3",
			},
		}
		p := EcsFargateProbe{}
		p.FromHealthCheck(svc, 8080)
		Expect(p.HttpGet.Path).To(Equal("/healthz"))
		Expect(p.HttpGet.Port).To(Equal(9000))
		Expect(p.HttpGet.SuccessCodes).To(Equal("200-299"))
		Expect(p.HttpGet.HealthyThreshold).To(Equal(3))
	})

	t.Run("non-numeric threshold/port labels are ignored (logged, not fatal)", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{
			HealthCheck: &types.HealthCheckConfig{},
			Labels: map[string]string{
				api.ComposeLabelHealthcheckHealthyThreshold: "notnum",
				api.ComposeLabelHealthcheckPort:             "notnum",
			},
		}
		p := EcsFargateProbe{}
		p.FromHealthCheck(svc, 8080)
		// HealthyThreshold stays zero; port stays the default passed in.
		Expect(p.HttpGet.HealthyThreshold).To(Equal(0))
		Expect(p.HttpGet.Port).To(Equal(8080))
	})
}

func TestToStartupAndLivenessProbe(t *testing.T) {
	RegisterTestingT(t)
	svc := types.ServiceConfig{HealthCheck: &types.HealthCheckConfig{
		Interval: durPtr(20 * time.Second),
		Retries:  uint64Ptr(2),
	}}

	t.Run("startup probe", func(t *testing.T) {
		RegisterTestingT(t)
		p, err := toStartupProbe(svc, 8080)
		Expect(err).ToNot(HaveOccurred())
		Expect(p.IntervalSeconds).To(Equal(20))
		Expect(p.Retries).To(Equal(2))
		Expect(p.HttpGet.Port).To(Equal(8080))
	})

	t.Run("liveness probe", func(t *testing.T) {
		RegisterTestingT(t)
		p, err := toLivenessProbe(svc, 9090)
		Expect(err).ToNot(HaveOccurred())
		Expect(p.IntervalSeconds).To(Equal(20))
		Expect(p.HttpGet.Port).To(Equal(9090))
	})
}

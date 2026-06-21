// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/compose-spec/compose-go/types"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestBytesSizeToHuman_Zero(t *testing.T) {
	RegisterTestingT(t)
	// Regression: log(0) is -Inf; the explicit zero guard returns "0".
	Expect(bytesSizeToHuman(0)).To(Equal("0"))
}

func TestBytesSizeToHuman_AboveTiClamps(t *testing.T) {
	RegisterTestingT(t)
	// Beyond Ti the unit index is clamped to the last entry ("Ti").
	Expect(bytesSizeToHuman(5 * 1024 * 1024 * 1024 * 1024 * 1024)).To(Equal("5120Ti"))
}

func TestToHeaders(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil headers returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(ToHeaders(nil)).To(BeNil())
	})

	t.Run("copies header map", func(t *testing.T) {
		RegisterTestingT(t)
		src := api.Headers{"X-Foo": "bar", "X-Baz": "qux"}
		got := ToHeaders(&src)
		Expect(got).To(HaveKeyWithValue("X-Foo", "bar"))
		Expect(got).To(HaveKeyWithValue("X-Baz", "qux"))
		// lo.Assign returns a fresh map (mutating it must not touch the source).
		got["X-Foo"] = "mutated"
		Expect(src["X-Foo"]).To(Equal("bar"))
	})
}

func TestToSimpleTextVolumes(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil text volumes yields empty slice", func(t *testing.T) {
		RegisterTestingT(t)
		got := ToSimpleTextVolumes(&api.StackConfigCompose{})
		Expect(got).To(BeEmpty())
	})

	t.Run("maps each text volume", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{
			TextVolumes: &[]api.TextVolume{
				{Name: "conf", MountPath: "/etc/app.conf", Content: "a=b"},
				{Name: "cert", MountPath: "/etc/cert.pem", Content: "----"},
			},
		}
		got := ToSimpleTextVolumes(cfg)
		Expect(got).To(HaveLen(2))
		Expect(got[0].Name).To(Equal("conf"))
		Expect(got[0].Content).To(Equal("a=b"))
		Expect(got[1].MountPath).To(Equal("/etc/cert.pem"))
	})
}

func TestToCpuLimit(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		cfg     *api.StackConfigCompose
		svc     types.ServiceConfig
		want    int64
		wantErr bool
	}{
		{
			name: "explicit size.limits.cpu",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Cpu: "500"}},
			},
			svc:  types.ServiceConfig{Name: "s"},
			want: 500,
		},
		{
			name: "invalid size.limits.cpu errors",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Cpu: "abc"}},
			},
			svc:     types.ServiceConfig{Name: "s"},
			wantErr: true,
		},
		{
			name: "legacy size.cpu",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Cpu: "750"},
			},
			svc:  types.ServiceConfig{Name: "s"},
			want: 750,
		},
		{
			name: "invalid legacy size.cpu errors",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Cpu: "xx"},
			},
			svc:     types.ServiceConfig{Name: "s"},
			wantErr: true,
		},
		{
			name: "docker compose nanocpus limit",
			cfg:  &api.StackConfigCompose{Runs: []string{"s"}},
			svc: types.ServiceConfig{
				Name: "s",
				Deploy: &types.DeployConfig{Resources: types.Resources{
					Limits: &types.Resource{NanoCPUs: "0.5"},
				}},
			},
			want: 512, // 1024 * 0.5
		},
		{
			name: "invalid nanocpus limit errors",
			cfg:  &api.StackConfigCompose{Runs: []string{"s"}},
			svc: types.ServiceConfig{
				Name: "s",
				Deploy: &types.DeployConfig{Resources: types.Resources{
					Limits: &types.Resource{NanoCPUs: "bad"},
				}},
			},
			wantErr: true,
		},
		{
			name: "default when nothing specified",
			cfg:  &api.StackConfigCompose{Runs: []string{"s"}},
			svc:  types.ServiceConfig{Name: "s"},
			want: 256,
		},
		{
			name: "multiple runs ignores size config and defaults",
			cfg: &api.StackConfigCompose{
				Runs: []string{"a", "b"},
				Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Cpu: "999"}},
			},
			svc:  types.ServiceConfig{Name: "a"},
			want: 256,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := toCpuLimit(tc.cfg, tc.svc)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestToCpuRequest(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		cfg      *api.StackConfigCompose
		svc      types.ServiceConfig
		cpuLimit int64
		want     int64
		wantErr  bool
	}{
		{
			name: "explicit size.requests.cpu",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Requests: &api.StackConfigComposeResources{Cpu: "200"}},
			},
			svc:      types.ServiceConfig{Name: "s"},
			cpuLimit: 500,
			want:     200,
		},
		{
			name: "invalid size.requests.cpu errors",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Requests: &api.StackConfigComposeResources{Cpu: "no"}},
			},
			svc:      types.ServiceConfig{Name: "s"},
			cpuLimit: 500,
			wantErr:  true,
		},
		{
			name: "docker compose nanocpus reservation",
			cfg:  &api.StackConfigCompose{Runs: []string{"s"}},
			svc: types.ServiceConfig{
				Name: "s",
				Deploy: &types.DeployConfig{Resources: types.Resources{
					Reservations: &types.Resource{NanoCPUs: "0.25"},
				}},
			},
			cpuLimit: 1024,
			want:     256, // 1024 * 0.25
		},
		{
			name: "invalid nanocpus reservation errors",
			cfg:  &api.StackConfigCompose{Runs: []string{"s"}},
			svc: types.ServiceConfig{
				Name: "s",
				Deploy: &types.DeployConfig{Resources: types.Resources{
					Reservations: &types.Resource{NanoCPUs: "junk"},
				}},
			},
			cpuLimit: 1024,
			wantErr:  true,
		},
		{
			name:     "fallback to half of limit",
			cfg:      &api.StackConfigCompose{Runs: []string{"s"}},
			svc:      types.ServiceConfig{Name: "s"},
			cpuLimit: 800,
			want:     400,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := toCpuRequest(tc.cfg, tc.svc, tc.cpuLimit)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestToMemoryLimit(t *testing.T) {
	RegisterTestingT(t)

	const mib = int64(1024 * 1024)
	tests := []struct {
		name    string
		cfg     *api.StackConfigCompose
		svc     types.ServiceConfig
		want    int64
		wantErr bool
	}{
		{
			name: "explicit size.limits.memory in MB",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Memory: "256"}},
			},
			svc:  types.ServiceConfig{Name: "s"},
			want: 256 * mib,
		},
		{
			name: "invalid size.limits.memory errors",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Memory: "bad"}},
			},
			svc:     types.ServiceConfig{Name: "s"},
			wantErr: true,
		},
		{
			name: "legacy size.memory in MB",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Memory: "128"},
			},
			svc:  types.ServiceConfig{Name: "s"},
			want: 128 * mib,
		},
		{
			name: "invalid legacy size.memory errors",
			cfg: &api.StackConfigCompose{
				Runs: []string{"s"},
				Size: &api.StackConfigComposeSize{Memory: "huge"},
			},
			svc:     types.ServiceConfig{Name: "s"},
			wantErr: true,
		},
		{
			name: "docker compose memory bytes",
			cfg:  &api.StackConfigCompose{Runs: []string{"s"}},
			svc: types.ServiceConfig{
				Name: "s",
				Deploy: &types.DeployConfig{Resources: types.Resources{
					Limits: &types.Resource{MemoryBytes: types.UnitBytes(64 * mib)},
				}},
			},
			want: 64 * mib,
		},
		{
			name: "default memory limit",
			cfg:  &api.StackConfigCompose{Runs: []string{"s"}},
			svc:  types.ServiceConfig{Name: "s"},
			want: 512 * mib,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := toMemoryLimit(tc.cfg, tc.svc)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestToResources(t *testing.T) {
	RegisterTestingT(t)

	t.Run("defaults produce expected limits and requests", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{Runs: []string{"s"}}
		res, err := ToResources(cfg, types.ServiceConfig{Name: "s"})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Limits).To(HaveKeyWithValue("cpu", "256m"))
		Expect(res.Limits).To(HaveKeyWithValue("memory", "512Mi"))
		Expect(res.Requests).To(HaveKeyWithValue("cpu", "128m"))
		Expect(res.Requests).To(HaveKeyWithValue("memory", "256Mi"))
	})

	t.Run("explicit size limits and requests", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{
			Runs: []string{"s"},
			Size: &api.StackConfigComposeSize{
				Limits:   &api.StackConfigComposeResources{Cpu: "1000", Memory: "1024"},
				Requests: &api.StackConfigComposeResources{Cpu: "500", Memory: "512"},
			},
		}
		res, err := ToResources(cfg, types.ServiceConfig{Name: "s"})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Limits).To(HaveKeyWithValue("cpu", "1000m"))
		// 1024 MB == 1 GiB worth of bytes, so bytesSizeToHuman renders it as "1Gi".
		Expect(res.Limits).To(HaveKeyWithValue("memory", "1Gi"))
		Expect(res.Requests).To(HaveKeyWithValue("cpu", "500m"))
		Expect(res.Requests).To(HaveKeyWithValue("memory", "512Mi"))
	})

	t.Run("cpu limit parse error propagates", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{
			Runs: []string{"s"},
			Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Cpu: "bad"}},
		}
		_, err := ToResources(cfg, types.ServiceConfig{Name: "s"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert CPU limits"))
	})

	t.Run("memory limit parse error propagates", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{
			Runs: []string{"s"},
			Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Memory: "bad"}},
		}
		_, err := ToResources(cfg, types.ServiceConfig{Name: "s"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert memory limits"))
	})

	t.Run("cpu request parse error propagates", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{
			Runs: []string{"s"},
			Size: &api.StackConfigComposeSize{Requests: &api.StackConfigComposeResources{Cpu: "bad"}},
		}
		_, err := ToResources(cfg, types.ServiceConfig{Name: "s"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert CPU requests"))
	})

	t.Run("memory request parse error propagates", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.StackConfigCompose{
			Runs: []string{"s"},
			Size: &api.StackConfigComposeSize{Requests: &api.StackConfigComposeResources{Memory: "bad"}},
		}
		_, err := ToResources(cfg, types.ServiceConfig{Name: "s"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert memory requests"))
	})
}

func TestToPersistentVolumes(t *testing.T) {
	RegisterTestingT(t)

	t.Run("tmpfs volume sizes from tmpfs spec", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{
			Volumes: []types.ServiceVolumeConfig{
				{Source: "tmp", Target: "/tmp", Tmpfs: &types.ServiceVolumeTmpfs{Size: types.UnitBytes(1024 * 1024)}},
			},
		}
		got := ToPersistentVolumes(svc, compose.Config{Project: &types.Project{}})
		Expect(got).To(HaveLen(1))
		Expect(got[0].Name).To(Equal("tmp"))
		Expect(got[0].MountPath).To(Equal("/tmp"))
		Expect(got[0].Storage).To(Equal("1Mi"))
	})

	t.Run("named volume size and access modes from labels", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{
			Volumes: []types.ServiceVolumeConfig{
				{Source: "data", Target: "/data"},
			},
		}
		cfg := compose.Config{Project: &types.Project{
			Volumes: types.Volumes{
				"data": types.VolumeConfig{Labels: types.Labels{
					api.ComposeLabelVolumeSize:         "20Gi",
					api.ComposeLabelVolumeAccessModes:  "ReadWriteOnce,ReadOnlyMany",
					api.ComposeLabelVolumeStorageClass: "fast-ssd",
				}},
			},
		}}
		got := ToPersistentVolumes(svc, cfg)
		Expect(got).To(HaveLen(1))
		Expect(got[0].Storage).To(Equal("20Gi"))
		Expect(got[0].AccessModes).To(ConsistOf("ReadWriteOnce", "ReadOnlyMany"))
		Expect(got[0].StorageClassName).ToNot(BeNil())
		Expect(*got[0].StorageClassName).To(Equal("fast-ssd"))
	})

	t.Run("volume without matching project volume keeps bare mapping", func(t *testing.T) {
		RegisterTestingT(t)
		svc := types.ServiceConfig{
			Volumes: []types.ServiceVolumeConfig{
				{Source: "orphan", Target: "/orphan"},
			},
		}
		got := ToPersistentVolumes(svc, compose.Config{Project: &types.Project{Volumes: types.Volumes{}}})
		Expect(got).To(HaveLen(1))
		Expect(got[0].Name).To(Equal("orphan"))
		Expect(got[0].Storage).To(Equal(""))
		Expect(got[0].AccessModes).To(BeNil())
		Expect(got[0].StorageClassName).To(BeNil())
	})

	t.Run("no volumes returns nil", func(t *testing.T) {
		RegisterTestingT(t)
		got := ToPersistentVolumes(types.ServiceConfig{}, compose.Config{Project: &types.Project{}})
		Expect(got).To(BeNil())
	})
}

func TestToRunPorts(t *testing.T) {
	RegisterTestingT(t)

	ports := []types.ServicePortConfig{{Target: 8080}, {Target: 9090}}
	Expect(toRunPorts(ports)).To(Equal([]int{8080, 9090}))
	Expect(toRunPorts(nil)).To(BeEmpty())
}

func TestToRunEnv(t *testing.T) {
	RegisterTestingT(t)

	val := "value"
	env := types.MappingWithEquals{
		"FOO":     &val,
		"NIL_VAR": nil, // nil values are skipped (no resolved value)
	}
	got := toRunEnv(env)
	Expect(got).To(HaveKeyWithValue("FOO", "value"))
	Expect(got).ToNot(HaveKey("NIL_VAR"))
}

func TestToRunSecrets(t *testing.T) {
	RegisterTestingT(t)
	// Currently a stub returning an empty (non-nil) map.
	got := toRunSecrets(types.MappingWithEquals{"SECRET": lo.ToPtr("x")})
	Expect(got).ToNot(BeNil())
	Expect(got).To(BeEmpty())
}

func TestProbeConversions(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil healthcheck returns nil probes", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(toStartupProbe(nil)).To(BeNil())
		Expect(toReadinessProbe(nil)).To(BeNil())
	})

	t.Run("populated healthcheck maps interval/retries/timeout", func(t *testing.T) {
		RegisterTestingT(t)
		interval := types.Duration(5 * time.Second)
		timeout := types.Duration(3 * time.Second)
		retries := uint64(4)
		check := &types.HealthCheckConfig{
			Interval: &interval,
			Timeout:  &timeout,
			Retries:  &retries,
		}

		rp := toReadinessProbe(check)
		Expect(rp).ToNot(BeNil())
		Expect(rp.Interval).ToNot(BeNil())
		Expect(*rp.Interval).To(Equal(5 * time.Second))
		Expect(rp.FailureThreshold).ToNot(BeNil())
		Expect(*rp.FailureThreshold).To(Equal(4))
		Expect(rp.TimeoutSeconds).ToNot(BeNil())
		Expect(*rp.TimeoutSeconds).To(Equal(3))
		// StartInterval unset => InitialDelaySeconds stays nil.
		Expect(rp.InitialDelaySeconds).To(BeNil())
	})

	// Quirk: InitialDelaySeconds is GATED on StartInterval being non-nil, but the
	// value it reads is StartPeriod. So a config with StartInterval set but
	// StartPeriod unset yields InitialDelaySeconds == 0 (seconds of a nil/zero
	// StartPeriod), not the StartInterval value.
	t.Run("startInterval gate reads startPeriod value", func(t *testing.T) {
		RegisterTestingT(t)
		startInterval := types.Duration(2 * time.Second)
		startPeriod := types.Duration(30 * time.Second)
		check := &types.HealthCheckConfig{
			StartInterval: &startInterval,
			StartPeriod:   &startPeriod,
		}

		sp := toStartupProbe(check)
		Expect(sp).ToNot(BeNil())
		Expect(sp.InitialDelaySeconds).ToNot(BeNil())
		Expect(*sp.InitialDelaySeconds).To(Equal(30)) // reads StartPeriod, not StartInterval
	})

	t.Run("startInterval set but startPeriod unset yields zero initial delay", func(t *testing.T) {
		RegisterTestingT(t)
		startInterval := types.Duration(2 * time.Second)
		check := &types.HealthCheckConfig{StartInterval: &startInterval}

		sp := toStartupProbe(check)
		Expect(sp).ToNot(BeNil())
		Expect(sp.InitialDelaySeconds).ToNot(BeNil())
		Expect(*sp.InitialDelaySeconds).To(Equal(0))
	})
}

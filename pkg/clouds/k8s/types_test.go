package k8s

import (
	"testing"
	"time"

	"github.com/compose-spec/compose-go/types"

	"github.com/simple-container-com/api/pkg/api"
)

func TestHTTPHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    HTTPHeader
		wantName  string
		wantValue string
	}{
		{
			name: "basic header",
			header: HTTPHeader{
				Name:  "Authorization",
				Value: "Bearer token123",
			},
			wantName:  "Authorization",
			wantValue: "Bearer token123",
		},
		{
			name: "custom header",
			header: HTTPHeader{
				Name:  "X-Custom-Header",
				Value: "custom-value",
			},
			wantName:  "X-Custom-Header",
			wantValue: "custom-value",
		},
		{
			name: "health check header",
			header: HTTPHeader{
				Name:  "X-Health-Check",
				Value: "true",
			},
			wantName:  "X-Health-Check",
			wantValue: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.header.Name != tt.wantName {
				t.Errorf("HTTPHeader.Name = %v, want %v", tt.header.Name, tt.wantName)
			}
			if tt.header.Value != tt.wantValue {
				t.Errorf("HTTPHeader.Value = %v, want %v", tt.header.Value, tt.wantValue)
			}
		})
	}
}

func TestProbeHttpGet_WithHeaders(t *testing.T) {
	tests := []struct {
		name    string
		probe   ProbeHttpGet
		wantLen int
	}{
		{
			name: "probe with single header",
			probe: ProbeHttpGet{
				Path: "/health",
				Port: 8080,
				HTTPHeaders: []HTTPHeader{
					{Name: "X-Health-Check", Value: "true"},
				},
			},
			wantLen: 1,
		},
		{
			name: "probe with multiple headers",
			probe: ProbeHttpGet{
				Path: "/health",
				Port: 8080,
				HTTPHeaders: []HTTPHeader{
					{Name: "X-Health-Check", Value: "true"},
					{Name: "Authorization", Value: "Bearer token123"},
					{Name: "X-Custom", Value: "custom-value"},
				},
			},
			wantLen: 3,
		},
		{
			name: "probe without headers",
			probe: ProbeHttpGet{
				Path: "/health",
				Port: 8080,
			},
			wantLen: 0,
		},
		{
			name: "probe with empty headers slice",
			probe: ProbeHttpGet{
				Path:        "/health",
				Port:        8080,
				HTTPHeaders: []HTTPHeader{},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.probe.HTTPHeaders) != tt.wantLen {
				t.Errorf("ProbeHttpGet.HTTPHeaders length = %v, want %v", len(tt.probe.HTTPHeaders), tt.wantLen)
			}
		})
	}
}

func TestCloudRunProbe_WithHeaders(t *testing.T) {
	initialDelay := 10
	interval := 5 * time.Second
	failureThreshold := 3

	probe := &CloudRunProbe{
		HttpGet: ProbeHttpGet{
			Path: "/health",
			Port: 8080,
			HTTPHeaders: []HTTPHeader{
				{Name: "X-Health-Check", Value: "true"},
				{Name: "Authorization", Value: "Bearer token123"},
			},
		},
		InitialDelaySeconds: &initialDelay,
		Interval:            &interval,
		FailureThreshold:    &failureThreshold,
	}

	if probe.HttpGet.Path != "/health" {
		t.Errorf("CloudRunProbe.HttpGet.Path = %v, want /health", probe.HttpGet.Path)
	}
	if probe.HttpGet.Port != 8080 {
		t.Errorf("CloudRunProbe.HttpGet.Port = %v, want 8080", probe.HttpGet.Port)
	}
	if len(probe.HttpGet.HTTPHeaders) != 2 {
		t.Errorf("CloudRunProbe.HttpGet.HTTPHeaders length = %v, want 2", len(probe.HttpGet.HTTPHeaders))
	}
	if probe.HttpGet.HTTPHeaders[0].Name != "X-Health-Check" {
		t.Errorf("First header name = %v, want X-Health-Check", probe.HttpGet.HTTPHeaders[0].Name)
	}
	if probe.HttpGet.HTTPHeaders[1].Name != "Authorization" {
		t.Errorf("Second header name = %v, want Authorization", probe.HttpGet.HTTPHeaders[1].Name)
	}
}

func Test_toMebibytesFormat(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{
			name: "bytes",
			size: 100,
			want: "100",
		},
		{
			name: "kilobytes",
			size: 100 * 1024,
			want: "100Ki",
		},
		{
			name: "megabytes",
			size: 100 * 1024 * 1024,
			want: "100Mi",
		},
		{
			name: "gigabytes",
			size: 100 * 1024 * 1024 * 1024,
			want: "100Gi",
		},
		{
			name: "terabytes",
			size: 100 * 1024 * 1024 * 1024 * 1024,
			want: "100Ti",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesSizeToHuman(tt.size); got != tt.want {
				t.Errorf("bytesSizeToHuman() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_toMemoryRequest(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *api.StackConfigCompose
		svc         types.ServiceConfig
		memoryLimit int64
		want        int64
		wantErr     bool
	}{
		{
			name: "explicit requests in size configuration",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
				Size: &api.StackConfigComposeSize{
					Requests: &api.StackConfigComposeResources{
						Memory: "256", // 256MB
					},
				},
			},
			svc:         types.ServiceConfig{Name: "test-service"},
			memoryLimit: 512 * 1024 * 1024, // 512MB in bytes
			want:        256 * 1024 * 1024, // 256MB in bytes
			wantErr:     false,
		},
		{
			name: "invalid memory request value",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
				Size: &api.StackConfigComposeSize{
					Requests: &api.StackConfigComposeResources{
						Memory: "invalid",
					},
				},
			},
			svc:         types.ServiceConfig{Name: "test-service"},
			memoryLimit: 512 * 1024 * 1024,
			want:        0,
			wantErr:     true,
		},
		{
			name: "docker compose deploy resources reservations",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
			},
			svc: types.ServiceConfig{
				Name: "test-service",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							MemoryBytes: 128 * 1024 * 1024, // 128MB in bytes
						},
					},
				},
			},
			memoryLimit: 512 * 1024 * 1024,
			want:        128 * 1024 * 1024, // 128MB in bytes
			wantErr:     false,
		},
		{
			name: "fallback to 50% of limit",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
			},
			svc:         types.ServiceConfig{Name: "test-service"},
			memoryLimit: 1024 * 1024 * 1024, // 1GB in bytes
			want:        512 * 1024 * 1024,  // 512MB in bytes (50% of limit)
			wantErr:     false,
		},
		{
			name: "multiple services - should not use size config",
			cfg: &api.StackConfigCompose{
				Runs: []string{"service1", "service2"}, // Multiple services
				Size: &api.StackConfigComposeSize{
					Requests: &api.StackConfigComposeResources{
						Memory: "256",
					},
				},
			},
			svc:         types.ServiceConfig{Name: "service1"},
			memoryLimit: 1024 * 1024 * 1024, // 1GB in bytes
			want:        512 * 1024 * 1024,  // 512MB in bytes (50% of limit, ignores size config)
			wantErr:     false,
		},
		{
			name: "empty memory request string",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
				Size: &api.StackConfigComposeSize{
					Requests: &api.StackConfigComposeResources{
						Memory: "", // Empty string
					},
				},
			},
			svc:         types.ServiceConfig{Name: "test-service"},
			memoryLimit: 800 * 1024 * 1024, // 800MB in bytes
			want:        400 * 1024 * 1024, // 400MB in bytes (50% of limit)
			wantErr:     false,
		},
		{
			name: "nil size config",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
				Size: nil,
			},
			svc:         types.ServiceConfig{Name: "test-service"},
			memoryLimit: 2048 * 1024 * 1024, // 2GB in bytes
			want:        1024 * 1024 * 1024, // 1GB in bytes (50% of limit)
			wantErr:     false,
		},
		{
			name: "nil requests in size config",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
				Size: &api.StackConfigComposeSize{
					Requests: nil,
				},
			},
			svc:         types.ServiceConfig{Name: "test-service"},
			memoryLimit: 1536 * 1024 * 1024, // 1.5GB in bytes
			want:        768 * 1024 * 1024,  // 768MB in bytes (50% of limit)
			wantErr:     false,
		},
		{
			name: "zero memory limit fallback",
			cfg: &api.StackConfigCompose{
				Runs: []string{"test-service"},
			},
			svc:         types.ServiceConfig{Name: "test-service"},
			memoryLimit: 0,
			want:        0, // 50% of 0 is 0
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toMemoryRequest(tt.cfg, tt.svc, tt.memoryLimit)
			if (err != nil) != tt.wantErr {
				t.Errorf("toMemoryRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("toMemoryRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

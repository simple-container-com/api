package k8s

import (
	"testing"

	"github.com/compose-spec/compose-go/types"

	"github.com/simple-container-com/api/pkg/api"
)

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

package kubernetes

import (
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestValidateHPAConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		scale     *k8s.Scale
		resources *k8s.Resources
		wantErr   bool
		errMsg    string
	}{
		{
			name: "Valid CPU-based HPA",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 2,
				MaxReplicas: 10,
				CPUTarget:   intPtr(70),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{
					"cpu":    "100m",
					"memory": "128Mi",
				},
			},
			wantErr: false,
		},
		{
			name: "Valid Memory-based HPA",
			scale: &k8s.Scale{
				EnableHPA:    true,
				MinReplicas:  1,
				MaxReplicas:  5,
				MemoryTarget: intPtr(80),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{
					"cpu":    "50m",
					"memory": "64Mi",
				},
			},
			wantErr: false,
		},
		{
			name: "Valid CPU and Memory HPA",
			scale: &k8s.Scale{
				EnableHPA:    true,
				MinReplicas:  3,
				MaxReplicas:  20,
				CPUTarget:    intPtr(60),
				MemoryTarget: intPtr(75),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{
					"cpu":    "200m",
					"memory": "256Mi",
				},
			},
			wantErr: false,
		},
		{
			name: "HPA disabled - no validation",
			scale: &k8s.Scale{
				EnableHPA:   false,
				MinReplicas: 2,
				MaxReplicas: 2,
			},
			resources: nil, // No resources needed when HPA is disabled
			wantErr:   false,
		},
		{
			name:      "Nil scale - no validation",
			scale:     nil,
			resources: nil,
			wantErr:   false,
		},
		{
			name: "Invalid min replicas - zero",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 0,
				MaxReplicas: 5,
				CPUTarget:   intPtr(70),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{"cpu": "100m"},
			},
			wantErr: true,
			errMsg:  "minReplicas must be greater than 0",
		},
		{
			name: "Invalid min replicas - negative",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: -1,
				MaxReplicas: 5,
				CPUTarget:   intPtr(70),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{"cpu": "100m"},
			},
			wantErr: true,
			errMsg:  "minReplicas must be greater than 0",
		},
		{
			name: "Invalid max replicas - not greater than min",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 5,
				MaxReplicas: 5,
				CPUTarget:   intPtr(70),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{"cpu": "100m"},
			},
			wantErr: true,
			errMsg:  "maxReplicas must be greater than minReplicas",
		},
		{
			name: "Invalid max replicas - less than min",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 10,
				MaxReplicas: 5,
				CPUTarget:   intPtr(70),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{"cpu": "100m"},
			},
			wantErr: true,
			errMsg:  "maxReplicas must be greater than minReplicas",
		},
		{
			name: "Invalid CPU target - zero",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 2,
				MaxReplicas: 10,
				CPUTarget:   intPtr(0),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{"cpu": "100m"},
			},
			wantErr: true,
			errMsg:  "CPU target must be between 1-100%, got 0",
		},
		{
			name: "Invalid CPU target - over 100",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 2,
				MaxReplicas: 10,
				CPUTarget:   intPtr(150),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{"cpu": "100m"},
			},
			wantErr: true,
			errMsg:  "CPU target must be between 1-100%, got 150",
		},
		{
			name: "Invalid Memory target - negative",
			scale: &k8s.Scale{
				EnableHPA:    true,
				MinReplicas:  2,
				MaxReplicas:  10,
				MemoryTarget: intPtr(-10),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{"memory": "128Mi"},
			},
			wantErr: true,
			errMsg:  "Memory target must be between 1-100%, got -10",
		},
		{
			name: "Missing CPU resource requests",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 2,
				MaxReplicas: 10,
				CPUTarget:   intPtr(70),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{
					"memory": "128Mi",
					// CPU request missing
				},
			},
			wantErr: true,
			errMsg:  "CPU resource requests must be defined when using CPU-based scaling",
		},
		{
			name: "Missing Memory resource requests",
			scale: &k8s.Scale{
				EnableHPA:    true,
				MinReplicas:  2,
				MaxReplicas:  10,
				MemoryTarget: intPtr(80),
			},
			resources: &k8s.Resources{
				Requests: map[string]string{
					"cpu": "100m",
					// Memory request missing
				},
			},
			wantErr: true,
			errMsg:  "Memory resource requests must be defined when using memory-based scaling",
		},
		{
			name: "Nil resources with CPU target",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 2,
				MaxReplicas: 10,
				CPUTarget:   intPtr(70),
			},
			resources: nil,
			wantErr:   true,
			errMsg:    "CPU resource requests must be defined when using CPU-based scaling",
		},
		{
			name: "No metrics configured",
			scale: &k8s.Scale{
				EnableHPA:   true,
				MinReplicas: 2,
				MaxReplicas: 10,
				// No CPU or Memory targets
			},
			resources: &k8s.Resources{
				Requests: map[string]string{
					"cpu":    "100m",
					"memory": "128Mi",
				},
			},
			wantErr: true,
			errMsg:  "at least one scaling metric (CPU or Memory) must be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHPAConfiguration(tt.scale, tt.resources)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.errMsg)
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

// Helper function for testing
func intPtr(i int) *int {
	return &i
}

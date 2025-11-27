package k8s

import (
	"testing"

	"github.com/simple-container-com/api/pkg/api"
)

func TestToScale_HPADetection(t *testing.T) {
	tests := []struct {
		name     string
		config   *api.StackConfigCompose
		expected *Scale
	}{
		{
			name: "HPA enabled - CPU scaling",
			config: &api.StackConfigCompose{
				Scale: &api.StackConfigComposeScale{
					Min: 2,
					Max: 10,
					Policy: &api.StackConfigComposeScalePolicy{
						Cpu: &api.StackConfigComposeScaleCpu{
							Max: 70,
						},
					},
				},
			},
			expected: &Scale{
				Replicas:     2,
				EnableHPA:    true,
				MinReplicas:  2,
				MaxReplicas:  10,
				CPUTarget:    intPtr(70),
				MemoryTarget: nil,
			},
		},
		{
			name: "HPA enabled - Memory scaling",
			config: &api.StackConfigCompose{
				Scale: &api.StackConfigComposeScale{
					Min: 1,
					Max: 5,
					Policy: &api.StackConfigComposeScalePolicy{
						Memory: &api.StackConfigComposeScaleMemory{
							Max: 80,
						},
					},
				},
			},
			expected: &Scale{
				Replicas:     1,
				EnableHPA:    true,
				MinReplicas:  1,
				MaxReplicas:  5,
				CPUTarget:    nil,
				MemoryTarget: intPtr(80),
			},
		},
		{
			name: "HPA enabled - CPU and Memory scaling",
			config: &api.StackConfigCompose{
				Scale: &api.StackConfigComposeScale{
					Min: 3,
					Max: 20,
					Policy: &api.StackConfigComposeScalePolicy{
						Cpu: &api.StackConfigComposeScaleCpu{
							Max: 60,
						},
						Memory: &api.StackConfigComposeScaleMemory{
							Max: 75,
						},
					},
				},
			},
			expected: &Scale{
				Replicas:     3,
				EnableHPA:    true,
				MinReplicas:  3,
				MaxReplicas:  20,
				CPUTarget:    intPtr(60),
				MemoryTarget: intPtr(75),
			},
		},
		{
			name: "Static scaling - min equals max",
			config: &api.StackConfigCompose{
				Scale: &api.StackConfigComposeScale{
					Min: 5,
					Max: 5,
					Policy: &api.StackConfigComposeScalePolicy{
						Cpu: &api.StackConfigComposeScaleCpu{
							Max: 70,
						},
					},
				},
			},
			expected: &Scale{
				Replicas:     5,
				EnableHPA:    false,
				MinReplicas:  5,
				MaxReplicas:  5,
				CPUTarget:    intPtr(70),
				MemoryTarget: nil,
			},
		},
		{
			name: "Static scaling - no policy",
			config: &api.StackConfigCompose{
				Scale: &api.StackConfigComposeScale{
					Min: 2,
					Max: 10,
					// No policy defined
				},
			},
			expected: &Scale{
				Replicas:     2,
				EnableHPA:    false,
				MinReplicas:  2,
				MaxReplicas:  10,
				CPUTarget:    nil,
				MemoryTarget: nil,
			},
		},
		{
			name:   "No scale config",
			config: &api.StackConfigCompose{
				// No scale config
			},
			expected: nil,
		},
		{
			name:     "Nil config",
			config:   nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToScale(tt.config)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("Expected %+v, got nil", tt.expected)
				return
			}

			// Compare all fields
			if result.Replicas != tt.expected.Replicas {
				t.Errorf("Replicas: expected %d, got %d", tt.expected.Replicas, result.Replicas)
			}
			if result.EnableHPA != tt.expected.EnableHPA {
				t.Errorf("EnableHPA: expected %t, got %t", tt.expected.EnableHPA, result.EnableHPA)
			}
			if result.MinReplicas != tt.expected.MinReplicas {
				t.Errorf("MinReplicas: expected %d, got %d", tt.expected.MinReplicas, result.MinReplicas)
			}
			if result.MaxReplicas != tt.expected.MaxReplicas {
				t.Errorf("MaxReplicas: expected %d, got %d", tt.expected.MaxReplicas, result.MaxReplicas)
			}

			// Compare CPU target
			if !intPtrEqual(result.CPUTarget, tt.expected.CPUTarget) {
				t.Errorf("CPUTarget: expected %v, got %v", ptrValue(tt.expected.CPUTarget), ptrValue(result.CPUTarget))
			}

			// Compare Memory target
			if !intPtrEqual(result.MemoryTarget, tt.expected.MemoryTarget) {
				t.Errorf("MemoryTarget: expected %v, got %v", ptrValue(tt.expected.MemoryTarget), ptrValue(result.MemoryTarget))
			}
		})
	}
}

// Helper functions for testing
func intPtr(i int) *int {
	return &i
}

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrValue(ptr *int) interface{} {
	if ptr == nil {
		return nil
	}
	return *ptr
}

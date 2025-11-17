package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// Test structs for MustClone testing
type TestStruct struct {
	Name   string            `yaml:"name"`
	Age    int               `yaml:"age"`
	Active bool              `yaml:"active"`
	Tags   []string          `yaml:"tags"`
	Meta   map[string]string `yaml:"meta"`
	Nested *NestedStruct     `yaml:"nested,omitempty"`
}

type NestedStruct struct {
	Value string `yaml:"value"`
	Count int    `yaml:"count"`
}

func TestMustClone(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		input    interface{}
		validate func(original, cloned interface{})
	}{
		{
			name: "struct with nested fields",
			input: TestStruct{
				Name:   "test",
				Age:    25,
				Active: true,
				Tags:   []string{"tag1", "tag2"},
				Meta:   map[string]string{"key1": "value1", "key2": "value2"},
				Nested: &NestedStruct{Value: "nested", Count: 42},
			},
			validate: func(original, cloned interface{}) {
				orig := original.(TestStruct)
				// YAML marshaling converts structs to maps
				clone := cloned.(map[string]interface{})

				// Verify all fields are copied correctly
				Expect(clone["name"]).To(Equal(orig.Name))
				Expect(clone["age"]).To(Equal(orig.Age))
				Expect(clone["active"]).To(Equal(orig.Active))

				// YAML converts []string to []interface{}, so we need to compare the values
				clonedTags := clone["tags"].([]interface{})
				Expect(len(clonedTags)).To(Equal(len(orig.Tags)))
				for i, tag := range orig.Tags {
					Expect(clonedTags[i]).To(Equal(tag))
				}

				// YAML converts map[string]string to map[string]interface{}, so we need to compare values
				clonedMeta := clone["meta"].(map[string]interface{})
				Expect(len(clonedMeta)).To(Equal(len(orig.Meta)))
				for k, v := range orig.Meta {
					Expect(clonedMeta[k]).To(Equal(v))
				}

				nested := clone["nested"].(map[string]interface{})
				Expect(nested["value"]).To(Equal(orig.Nested.Value))
				Expect(nested["count"]).To(Equal(orig.Nested.Count))

				// Verify it's a deep copy (different memory addresses)
				Expect(clone).ToNot(BeIdenticalTo(orig))
			},
		},
		{
			name: "pointer to struct",
			input: &TestStruct{
				Name: "pointer_test",
				Age:  30,
			},
			validate: func(original, cloned interface{}) {
				orig := original.(*TestStruct)
				// YAML marshaling converts pointer to struct to map
				clone := cloned.(map[string]interface{})

				// Verify values are copied
				Expect(clone["name"]).To(Equal(orig.Name))
				Expect(clone["age"]).To(Equal(orig.Age))

				// Verify it's a different object
				Expect(clone).ToNot(BeIdenticalTo(orig))
			},
		},
		{
			name:  "nil pointer",
			input: (*TestStruct)(nil),
			validate: func(original, cloned interface{}) {
				Expect(cloned).To(BeNil())
			},
		},
		{
			name: "map with various types",
			input: map[string]interface{}{
				"string": "value",
				"number": 42,
				"bool":   true,
				"nested": map[string]string{"key": "value"},
				"slice":  []string{"item1", "item2"},
			},
			validate: func(original, cloned interface{}) {
				orig := original.(map[string]interface{})
				clone := cloned.(map[string]interface{})

				// Verify all values are copied
				Expect(clone["string"]).To(Equal(orig["string"]))
				Expect(clone["number"]).To(Equal(orig["number"]))
				Expect(clone["bool"]).To(Equal(orig["bool"]))

				// Verify it's a deep copy
				Expect(clone).ToNot(BeIdenticalTo(orig))
			},
		},
		{
			name:  "slice of strings",
			input: []string{"item1", "item2", "item3"},
			validate: func(original, cloned interface{}) {
				orig := original.([]string)
				// YAML converts []string to []interface{}
				clone := cloned.([]interface{})

				// Verify values are copied
				Expect(len(clone)).To(Equal(len(orig)))
				for i, item := range orig {
					Expect(clone[i]).To(Equal(item))
				}

				// Verify it's a different slice
				Expect(clone).ToNot(BeIdenticalTo(orig))
			},
		},
		{
			name:  "string primitive",
			input: "test",
			validate: func(original, cloned interface{}) {
				Expect(cloned).To(Equal("test"))
			},
		},
		{
			name:  "int primitive",
			input: 42,
			validate: func(original, cloned interface{}) {
				Expect(cloned).To(Equal(42))
			},
		},
		{
			name:  "bool primitive",
			input: true,
			validate: func(original, cloned interface{}) {
				Expect(cloned).To(Equal(true))
			},
		},
		{
			name:  "float primitive",
			input: 3.14,
			validate: func(original, cloned interface{}) {
				Expect(cloned).To(Equal(3.14))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := MustClone(tt.input)
			tt.validate(tt.input, cloned)
		})
	}
}

func TestStackConfigCompose_Copy(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		input    *StackConfigCompose
		validate func(original, copied *StackConfigCompose)
	}{
		{
			name: "basic configuration",
			input: &StackConfigCompose{
				DockerComposeFile: "docker-compose.yml",
				Domain:            "example.com",
				Uses:              []string{"postgres", "redis"},
				Env:               map[string]string{"KEY1": "value1", "KEY2": "value2"},
				Secrets:           map[string]string{"SECRET1": "secret1"},
			},
			validate: func(original, copied *StackConfigCompose) {
				// Verify basic fields are copied
				Expect(copied.DockerComposeFile).To(Equal(original.DockerComposeFile))
				Expect(copied.Domain).To(Equal(original.Domain))
				Expect(copied.Uses).To(Equal(original.Uses))
				Expect(copied.Env).To(Equal(original.Env))
				Expect(copied.Secrets).To(Equal(original.Secrets))

				// Verify it's a different instance
				Expect(copied).ToNot(BeIdenticalTo(original))

				// Verify maps are deep copied
				Expect(copied.Env).ToNot(BeIdenticalTo(original.Env))
				Expect(copied.Secrets).ToNot(BeIdenticalTo(original.Secrets))
			},
		},
		{
			name: "VPA configuration in CloudExtras",
			input: func() *StackConfigCompose {
				vpaConfig := map[string]interface{}{
					"vpa": map[string]interface{}{
						"enabled":    true,
						"updateMode": "Auto",
						"minAllowed": map[string]interface{}{
							"cpu":    "50m",
							"memory": "128Mi",
						},
						"maxAllowed": map[string]interface{}{
							"cpu":    "1",
							"memory": "2Gi",
						},
						"controlledResources": []interface{}{"cpu", "memory"},
					},
				}
				cloudExtras := any(vpaConfig)
				return &StackConfigCompose{
					DockerComposeFile: "docker-compose.yml",
					CloudExtras:       &cloudExtras,
				}
			}(),
			validate: func(original, copied *StackConfigCompose) {
				// Verify CloudExtras is copied
				Expect(copied.CloudExtras).ToNot(BeNil())
				Expect(copied.CloudExtras).ToNot(BeIdenticalTo(original.CloudExtras))

				// Verify VPA configuration is preserved
				originalVPA := (*original.CloudExtras).(map[string]interface{})
				copiedVPA := (*copied.CloudExtras).(map[string]interface{})
				Expect(copiedVPA["vpa"]).To(Equal(originalVPA["vpa"]))

				// Verify it's a deep copy - modify original and check copy is unaffected
				originalVPAConfig := originalVPA["vpa"].(map[string]interface{})
				originalVPAConfig["enabled"] = false
				originalVPAConfig["updateMode"] = "Off"

				copiedVPAConfig := copiedVPA["vpa"].(map[string]interface{})
				Expect(copiedVPAConfig["enabled"]).To(Equal(true))
				Expect(copiedVPAConfig["updateMode"]).To(Equal("Auto"))
			},
		},
		{
			name: "nil CloudExtras",
			input: &StackConfigCompose{
				DockerComposeFile: "docker-compose.yml",
				CloudExtras:       nil,
			},
			validate: func(original, copied *StackConfigCompose) {
				Expect(copied.CloudExtras).To(BeNil())
			},
		},
		{
			name: "complex CloudExtras with multiple configurations",
			input: func() *StackConfigCompose {
				complexConfig := map[string]interface{}{
					"vpa": map[string]interface{}{
						"enabled":    true,
						"updateMode": "Auto",
						"minAllowed": map[string]interface{}{
							"cpu":    "100m",
							"memory": "256Mi",
						},
					},
					"nodeSelector": map[string]interface{}{
						"disktype": "ssd",
						"zone":     "us-west1",
					},
					"affinity": map[string]interface{}{
						"nodePool":     "high-memory",
						"computeClass": "n1-highmem-4",
					},
					"disruptionBudget": map[string]interface{}{
						"maxUnavailable": 1,
					},
				}
				cloudExtras := any(complexConfig)
				return &StackConfigCompose{
					DockerComposeFile: "docker-compose.yml",
					CloudExtras:       &cloudExtras,
				}
			}(),
			validate: func(original, copied *StackConfigCompose) {
				// Verify all configurations are preserved
				Expect(copied.CloudExtras).ToNot(BeNil())
				originalConfig := (*original.CloudExtras).(map[string]interface{})
				copiedConfig := (*copied.CloudExtras).(map[string]interface{})

				Expect(copiedConfig["vpa"]).To(Equal(originalConfig["vpa"]))
				Expect(copiedConfig["nodeSelector"]).To(Equal(originalConfig["nodeSelector"]))
				Expect(copiedConfig["affinity"]).To(Equal(originalConfig["affinity"]))
				Expect(copiedConfig["disruptionBudget"]).To(Equal(originalConfig["disruptionBudget"]))

				// Verify deep copy by modifying nested values
				originalVPA := originalConfig["vpa"].(map[string]interface{})
				originalVPA["enabled"] = false

				copiedVPA := copiedConfig["vpa"].(map[string]interface{})
				Expect(copiedVPA["enabled"]).To(Equal(true)) // Should remain unchanged
			},
		},
		{
			name: "dependencies handling",
			input: &StackConfigCompose{
				DockerComposeFile: "docker-compose.yml",
				Dependencies: []StackConfigDependencyResource{
					{Name: "postgres"},
					{Name: "redis"},
				},
			},
			validate: func(original, copied *StackConfigCompose) {
				Expect(copied.Dependencies).To(Equal(original.Dependencies))
			},
		},
		{
			name: "nil dependencies",
			input: &StackConfigCompose{
				DockerComposeFile: "docker-compose.yml",
				Dependencies:      nil,
			},
			validate: func(original, copied *StackConfigCompose) {
				Expect(copied.Dependencies).ToNot(BeNil()) // Should be empty slice, not nil
				Expect(copied.Dependencies).To(HaveLen(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.input.Copy().(*StackConfigCompose)
			tt.validate(tt.input, copied)
		})
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestCaddyAnnotations(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *k8s.CaddyConfig
		want map[string]string
	}{
		{name: "nil_config", cfg: nil, want: map[string]string{"pulumi.com/patchForce": "true"}},
		{name: "unset_field", cfg: &k8s.CaddyConfig{}, want: map[string]string{"pulumi.com/patchForce": "true"}},
		{
			name: "user_annotation_added",
			cfg:  &k8s.CaddyConfig{Annotations: map[string]string{"cluster-autoscaler.kubernetes.io/safe-to-evict": "false"}},
			want: map[string]string{
				"pulumi.com/patchForce":                          "true",
				"cluster-autoscaler.kubernetes.io/safe-to-evict": "false",
			},
		},
		{
			name: "user_cannot_override_patch_force",
			cfg:  &k8s.CaddyConfig{Annotations: map[string]string{"pulumi.com/patchForce": "false"}},
			want: map[string]string{"pulumi.com/patchForce": "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(caddyAnnotations(tt.cfg)).To(Equal(tt.want))
		})
	}
}

func TestCaddyAnnotations_DoesNotMutateConfig(t *testing.T) {
	RegisterTestingT(t)
	cfg := &k8s.CaddyConfig{Annotations: map[string]string{"a": "b"}}
	_ = caddyAnnotations(cfg)
	Expect(cfg.Annotations).To(Equal(map[string]string{"a": "b"}))
}

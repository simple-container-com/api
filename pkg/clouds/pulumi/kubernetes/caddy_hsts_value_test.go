// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestCaddyfileEmbedHSTSPlaceholder(t *testing.T) {
	RegisterTestingT(t)
	content, err := Caddyconfig.ReadFile("embed/caddy/Caddyfile")
	Expect(err).ToNot(HaveOccurred())
	Expect(string(content)).To(ContainSubstring(`{$HSTS_VALUE:max-age=31536000; includeSubDomains; preload}`))
}

func TestCaddyConfig_HSTSValue_FieldShape(t *testing.T) {
	RegisterTestingT(t)
	ft, ok := reflect.TypeOf(k8s.CaddyConfig{}).FieldByName("HSTSValue")
	Expect(ok).To(BeTrue())
	Expect(ft.Type.String()).To(Equal("*string"))
	Expect(ft.Tag.Get("json")).To(Equal("hstsValue,omitempty"))
	Expect(ft.Tag.Get("yaml")).To(Equal("hstsValue,omitempty"))
}

func TestCaddyHSTSEnv_WiringContract(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cfg  *k8s.CaddyConfig
		want map[string]string
	}{
		{name: "nil_config", cfg: nil, want: nil},
		{name: "unset_field", cfg: &k8s.CaddyConfig{}, want: nil},
		{name: "empty_string_treated_as_unset", cfg: &k8s.CaddyConfig{HSTSValue: lo.ToPtr("")}, want: nil},
		{
			name: "explicit_value_overrides",
			cfg:  &k8s.CaddyConfig{HSTSValue: lo.ToPtr("max-age=31536000; includeSubDomains")},
			want: map[string]string{"HSTS_VALUE": "max-age=31536000; includeSubDomains"},
		},
		{
			name: "max_age_zero_to_disable",
			cfg:  &k8s.CaddyConfig{HSTSValue: lo.ToPtr("max-age=0")},
			want: map[string]string{"HSTS_VALUE": "max-age=0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := caddyHSTSEnv(tt.cfg)
			if tt.want == nil {
				Expect(got).To(BeEmpty())
			} else {
				Expect(got).To(Equal(tt.want))
			}
		})
	}
}

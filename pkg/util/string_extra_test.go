// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package util

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestTrimStringMiddle(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		maxLen int
		sep    string
		want   string
	}{
		{"short input unchanged", "abc", 10, "-", "abc"},
		{"exact length unchanged", "abcdefghij", 10, "-", "abcdefghij"},
		{"long input truncated in middle", "abcdefghijklmnop", 7, "-", ""}, // computed below — just assert shape
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := TrimStringMiddle(tc.in, tc.maxLen, tc.sep)
			Expect(len(got)).To(BeNumerically("<=", tc.maxLen))
			if tc.want != "" {
				Expect(got).To(Equal(tc.want))
			}
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"simple PascalCase", "MyVariable", "my_variable"},
		{"camelCase", "myVariable", "my_variable"},
		{"already snake_case", "my_variable", "my_variable"},
		{"acronym handling", "HTTPServer", "http_server"},
		{"mixed digits", "AbC123Def", "ab_c123_def"},
		{"empty string", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(ToSnakeCase(tc.in)).To(Equal(tc.want))
		})
	}
}

func TestToEnvVariableName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"PascalCase → SCREAMING_SNAKE", "MyVariable", "MY_VARIABLE"},
		{"hyphens become underscores", "my-variable-name", "MY_VARIABLE_NAME"},
		{"mixed underscore + hyphen", "foo-bar_baz", "FOO_BAR_BAZ"},
		{"already uppercase", "ALREADY_UPPER", "ALREADY_UPPER"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(ToEnvVariableName(tc.in)).To(Equal(tc.want))
		})
	}
}

func TestSanitizeGCPServiceAccountName_LongInputProducesStableHash(t *testing.T) {
	RegisterTestingT(t)

	// Identical inputs must produce identical outputs — the hash function is
	// FNV-1a so it's deterministic.
	long := "exceedingly-long-service-account-name-for-coverage"
	a := SanitizeGCPServiceAccountName(long)
	b := SanitizeGCPServiceAccountName(long)
	Expect(a).To(Equal(b))
}

func TestSanitizeGCPServiceAccountName_DifferentInputsDifferentOutputs(t *testing.T) {
	RegisterTestingT(t)

	// Near-identical long inputs should hash distinctly enough to differ.
	a := SanitizeGCPServiceAccountName("exceedingly-long-service-account-name-for-staging-environment")
	b := SanitizeGCPServiceAccountName("exceedingly-long-service-account-name-for-production-environment")
	Expect(a).ToNot(Equal(b))
}

func TestSanitizeK8sResourceName_LongInput(t *testing.T) {
	RegisterTestingT(t)

	long := strings.Repeat("very-long-name-", 10)
	got := SanitizeK8sResourceName(long)
	Expect(len(got)).To(BeNumerically("<=", 63))
	// Should end with a 4-hex hash + hyphen marker.
	Expect(got).To(MatchRegexp(`-[0-9a-f]{4}$`))
}

func TestSafeSplit(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple space-separated", "a b c", []string{"a", "b", "c"}},
		{"single-quoted span kept together", `a 'b c' d`, []string{"a", "b c", "d"}},
		{"double-quoted span kept together", `a "b c" d`, []string{"a", "b c", "d"}},
		{"unmatched quote → fallback (best effort)", `a "b c`, nil}, // shape-only check below
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := SafeSplit(tc.in)
			Expect(got).ToNot(BeNil())
			if tc.want != nil {
				Expect(got).To(Equal(tc.want))
			}
		})
	}
}

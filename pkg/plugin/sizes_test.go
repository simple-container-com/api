// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestGibibytes(t *testing.T) {
	RegisterTestingT(t)

	// Whole-GiB inputs: the common case, round-trip to the integer the API wants.
	for _, tc := range []struct {
		in     string
		expect int
	}{
		{"4Gi", 4},
		{"60Gi", 60},
		{"1024Mi", 1},     // 1024 MiB = 1 GiB, still whole.
		{"1073741824", 1}, // raw bytes = 1 GiB.
	} {
		got, err := Gibibytes(tc.in, "size.disk")
		Expect(err).ToNot(HaveOccurred(), "input %q", tc.in)
		Expect(got).To(Equal(tc.expect), "input %q", tc.in)
	}

	// Fractional / unparseable / zero inputs: the two shapes of refusal, each
	// with the field name in the message so an operator reads which input is
	// the one at fault.
	for _, tc := range []struct {
		in   string
		msg  string
		kind string
	}{
		{"4.5Gi", "whole number of GiB", "fractional"},
		{"500Mi", "whole number of GiB", "sub-GiB"},
		{"0Gi", "whole number of GiB", "zero"},
		{"four gigabytes", "not a size", "unparseable"},
	} {
		_, err := Gibibytes(tc.in, "size.memory")
		Expect(err).To(HaveOccurred(), "kind %q input %q", tc.kind, tc.in)
		Expect(err.Error()).To(ContainSubstring(tc.msg), "kind %q input %q", tc.kind, tc.in)
		Expect(err.Error()).To(ContainSubstring("size.memory"), "kind %q — error must name the field", tc.kind)
	}
}

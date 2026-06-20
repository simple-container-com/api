// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestGetValue_SliceIndexBounds pins the bounds-checking of a numeric key used
// to index a []interface{}: a negative index must return an "index out of
// bounds" error rather than panicking (regression for the CWE-190/681 fix in
// getValPart), and an out-of-range positive index must error too.
func TestGetValue_SliceIndexBounds(t *testing.T) {
	arr := []interface{}{"a", "b", "c"}

	cases := []struct {
		name    string
		key     string
		want    interface{}
		wantErr string
	}{
		{"valid index 0", "0", "a", ""},
		{"valid last index", "2", "c", ""},
		{"negative index errors (no panic)", "-1", nil, "index out of bounds"},
		{"index == len errors", "3", nil, "index out of bounds"},
		{"large index errors", "9999", nil, "index out of bounds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := GetValue(tc.key, arr)
			if tc.wantErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.wantErr))
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

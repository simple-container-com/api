// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestGetValue_AcrossSupportedContainerTypes(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   interface{}
		want    interface{}
		wantErr bool
	}{
		{
			name:  "map[string]map[string]interface{} top-level hit",
			key:   "outer",
			value: map[string]map[string]interface{}{"outer": {"k": "v"}},
			want:  map[string]interface{}{"k": "v"},
		},
		{
			name:  "map[string]string lookup",
			key:   "host",
			value: map[string]string{"host": "example.com"},
			want:  "example.com",
		},
		{
			name:  "Data type lookup",
			key:   "answer",
			value: Data{"answer": 42},
			want:  42,
		},
		{
			name:  "slice index access by numeric key",
			key:   "1",
			value: []interface{}{"zero", "one", "two"},
			want:  "one",
		},
		{
			name:    "slice index out of bounds errors",
			key:     "9",
			value:   []interface{}{"only"},
			wantErr: true,
		},
		{
			name:    "unsupported value type errors",
			key:     "x",
			value:   12345, // plain int is not a supported container
			wantErr: true,
		},
		{
			name:    "missing key in map[string]string errors",
			key:     "nope",
			value:   map[string]string{"present": "1"},
			wantErr: true,
		},
		{
			name:    "missing key in map[string]interface{} errors",
			key:     "nope",
			value:   map[string]interface{}{"present": "1"},
			wantErr: true,
		},
		{
			name:    "missing key in Data errors",
			key:     "nope",
			value:   Data{"present": "1"},
			wantErr: true,
		},
		{
			name:    "missing key in map[string]map[string]interface{} errors",
			key:     "nope",
			value:   map[string]map[string]interface{}{"present": {"a": "b"}},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := GetValue(tc.key, tc.value)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestGetValue_NestedThroughSliceAndMaps(t *testing.T) {
	RegisterTestingT(t)

	// Dotted path traversal that walks map -> slice -> map.
	value := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"name": "first"},
			map[string]interface{}{"name": "second"},
		},
	}
	got, err := GetValue("items.1.name", value)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal("second"))
}

func TestGetValue_NonNumericSliceKeyErrors(t *testing.T) {
	RegisterTestingT(t)

	// For a slice, a non-numeric key fails strconv.ParseInt. That error is the
	// value getValPart returns, so GetValue surfaces a ParseInt error rather
	// than a "key not present" / "out of bounds" message. We assert the actual
	// current behaviour (the wrapped ParseInt error propagates).
	_, err := GetValue("notanumber", []interface{}{"a", "b"})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("invalid syntax"))
}

func TestCopyMap_DeepCopiesNestedMaps(t *testing.T) {
	RegisterTestingT(t)

	orig := map[string]interface{}{
		"nested": map[string]interface{}{"inner": "original"},
		"flat":   "value",
	}
	dup := CopyMap(orig)
	Expect(dup).To(Equal(orig))

	// Mutating the nested map in the copy must not touch the original — proving
	// the recursive CopyMap branch ran.
	dup["nested"].(map[string]interface{})["inner"] = "MUTATED"
	Expect(orig["nested"].(map[string]interface{})["inner"]).To(Equal("original"))
}

func TestData_AddAllIfNotExist_IntoEmpty(t *testing.T) {
	RegisterTestingT(t)

	base := Data{}
	base.AddAllIfNotExist(Data{"a": 1, "b": 2})
	Expect(base).To(HaveLen(2))
	Expect(base).To(HaveKeyWithValue("a", 1))
	Expect(base).To(HaveKeyWithValue("b", 2))
}

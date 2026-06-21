// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package util

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
)

func TestMapErr_HappyPath(t *testing.T) {
	RegisterTestingT(t)

	in := []int{1, 2, 3, 4}
	out, err := MapErr(in, func(v, _ int) (int, error) { return v * v, nil })
	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(Equal([]int{1, 4, 9, 16}))
}

func TestMapErr_StopsOnFirstError(t *testing.T) {
	RegisterTestingT(t)

	in := []int{1, 2, 3, 4}
	calls := 0
	_, err := MapErr(in, func(v, _ int) (int, error) {
		calls++
		if v == 2 {
			return 0, errors.New("boom")
		}
		return v, nil
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(Equal("boom"))
	// Iteratee should not have been called on items past the failing index.
	Expect(calls).To(Equal(2))
}

func TestMapErr_EmptyCollection(t *testing.T) {
	RegisterTestingT(t)

	out, err := MapErr([]int{}, func(v, _ int) (int, error) { return v, nil })
	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(BeEmpty())
}

func TestAddIfNotExist(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		add  string
		want []string
	}{
		{"add to empty", []string{}, "a", []string{"a"}},
		{"add new value", []string{"a", "b"}, "c", []string{"a", "b", "c"}},
		{"duplicate is skipped", []string{"a", "b"}, "b", []string{"a", "b"}},
		{"empty string is added", []string{"a"}, "", []string{"a", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(AddIfNotExist(tc.in, tc.add)).To(Equal(tc.want))
		})
	}
}

func TestCopyStringMap(t *testing.T) {
	RegisterTestingT(t)

	orig := map[string]string{"a": "1", "b": "2"}
	dup := CopyStringMap(orig)

	Expect(dup).To(Equal(orig))
	// Verify it's a true copy: mutating the duplicate doesn't affect the original.
	dup["a"] = "MUTATED"
	Expect(orig["a"]).To(Equal("1"))
}

func TestCopyStringMap_Nil(t *testing.T) {
	RegisterTestingT(t)

	Expect(CopyStringMap(nil)).To(BeEmpty())
}

func TestCopyMap(t *testing.T) {
	RegisterTestingT(t)

	orig := map[string]any{"a": 1, "b": "two", "c": []int{1, 2, 3}}
	dup := CopyMap(orig)

	Expect(dup).To(Equal(orig))
	// Top-level isolation: changing a top-level key in the dup doesn't affect orig.
	dup["a"] = "MUTATED"
	Expect(orig["a"]).To(Equal(1))
}

func TestCopyMap_Nil(t *testing.T) {
	RegisterTestingT(t)

	Expect(CopyMap(nil)).To(BeEmpty())
}

func TestData_AddAllIfNotExist(t *testing.T) {
	RegisterTestingT(t)

	base := Data{"a": 1, "b": 2}
	overlay := Data{"b": "ignored-because-key-exists", "c": 3}

	base.AddAllIfNotExist(overlay)

	Expect(base).To(HaveLen(3))
	Expect(base["a"]).To(Equal(1))
	Expect(base["b"]).To(Equal(2)) // unchanged — key already existed
	Expect(base["c"]).To(Equal(3))
}

func TestGetValue_NilInput(t *testing.T) {
	RegisterTestingT(t)

	got, err := GetValue("a.b.c", nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(BeNil())
}

func TestGetValue_TopLevelKey(t *testing.T) {
	RegisterTestingT(t)

	v := map[string]any{"hello": "world"}
	got, err := GetValue("hello", v)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal("world"))
}

func TestGetValue_NestedDotPath(t *testing.T) {
	RegisterTestingT(t)

	v := map[string]any{"a": map[string]any{"b": map[string]any{"c": "deep-value"}}}
	got, err := GetValue("a.b.c", v)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal("deep-value"))
}

func TestGetValue_MissingKey(t *testing.T) {
	RegisterTestingT(t)

	v := map[string]any{"a": "1"}
	_, err := GetValue("does.not.exist", v)
	Expect(err).To(HaveOccurred())
}

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package tests

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
	"github.com/simple-container-com/api/pkg/template"
)

// configWithCollections stands in for the config structs the walker actually
// traverses: optional lists and maps whose ZERO VALUE carries meaning.
type configWithCollections struct {
	Name     string
	Disable  []string
	Tags     map[string]string
	Nested   []nestedItem
	Pointers *string
}

type nestedItem struct {
	Value string
}

func upperExt(value string) template.Extension {
	return func(noSubs, path string, _ *string) (string, error) {
		return value, nil
	}
}

// Test_Apply_PreservesNilCollections pins the distinction between a nil
// collection and an empty one across the placeholder walk.
//
// reflect.MakeSlice / reflect.MakeMap always return a NON-nil value, so the
// walker used to rewrite every unset slice and map in every parsed SC config to
// an empty-but-present one. Any config field that means "not configured, apply
// the default" when nil therefore lost that meaning before a single provisioner
// read it — e.g. k3s-cluster's `disable`, where unset means "turn traefik off"
// and `[]` is the operator's explicit opt-in to keep it. See
// clouds/pulumi/k3s/cluster.go:211.
func Test_Apply_PreservesNilCollections(t *testing.T) {
	RegisterTestingT(t)

	cfg := configWithCollections{Name: "${test:value}"}

	p := placeholders.New()
	Expect(p.Apply(&cfg, placeholders.WithExtensions(map[string]template.Extension{
		"test": upperExt("substituted"),
	}))).To(BeNil())

	Expect(cfg.Name).To(Equal("substituted"))
	Expect(cfg.Disable).To(BeNil())
	Expect(cfg.Tags).To(BeNil())
	Expect(cfg.Nested).To(BeNil())
	Expect(cfg.Pointers).To(BeNil())
}

// Test_Apply_KeepsEmptyCollectionsDistinctFromNil is the other half: an
// explicitly empty list/map must NOT come back nil either, because that is the
// value an operator writes to override a nil-means-default.
func Test_Apply_KeepsEmptyCollectionsDistinctFromNil(t *testing.T) {
	RegisterTestingT(t)

	cfg := configWithCollections{
		Disable: []string{},
		Tags:    map[string]string{},
	}

	p := placeholders.New()
	Expect(p.Apply(&cfg)).To(BeNil())

	Expect(cfg.Disable).NotTo(BeNil())
	Expect(cfg.Disable).To(BeEmpty())
	Expect(cfg.Tags).NotTo(BeNil())
	Expect(cfg.Tags).To(BeEmpty())
}

// Test_Apply_SubstitutesInsideCollections guards the obvious regression risk of
// the early return above: populated slices and maps must still have their
// placeholders resolved, at every depth.
func Test_Apply_SubstitutesInsideCollections(t *testing.T) {
	RegisterTestingT(t)

	cfg := configWithCollections{
		Disable: []string{"${test:value}", "literal"},
		Tags:    map[string]string{"key": "${test:value}"},
		Nested:  []nestedItem{{Value: "${test:value}"}},
	}

	p := placeholders.New()
	Expect(p.Apply(&cfg, placeholders.WithExtensions(map[string]template.Extension{
		"test": upperExt("substituted"),
	}))).To(BeNil())

	Expect(cfg.Disable).To(Equal([]string{"substituted", "literal"}))
	Expect(cfg.Tags).To(Equal(map[string]string{"key": "substituted"}))
	Expect(cfg.Nested).To(Equal([]nestedItem{{Value: "substituted"}}))
}

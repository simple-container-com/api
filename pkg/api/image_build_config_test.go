// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
)

func TestReuseExistingCommitTagEnabled(t *testing.T) {
	RegisterTestingT(t)

	var nilClient *ClientDescriptor
	Expect(nilClient.ReuseExistingCommitTagEnabled()).To(BeFalse())

	Expect((&ClientDescriptor{}).ReuseExistingCommitTagEnabled()).To(BeFalse())
	Expect((&ClientDescriptor{ImageBuild: &ImageBuildDescriptor{}}).ReuseExistingCommitTagEnabled()).To(BeFalse())
	Expect((&ClientDescriptor{
		ImageBuild: &ImageBuildDescriptor{ReuseExistingCommitTag: true},
	}).ReuseExistingCommitTagEnabled()).To(BeTrue())
}

// ClientDescriptor.Copy enumerates its fields by hand and every deploy goes
// through it, so a field added to the struct and not to the copier is silently
// dropped before it reaches any provider. That is how reuseExistingCommitTag
// first shipped inert. Reflection over the exported fields is what makes the
// next one fail here instead of in production.
func TestClientDescriptorCopyPreservesEveryField(t *testing.T) {
	RegisterTestingT(t)

	src := ClientDescriptor{
		SchemaVersion: "1.0",
		Defaults:      map[string]interface{}{"k": "v"},
		Stacks:        map[string]StackClientDescriptor{"prod": {Type: "cloud-compose"}},
		Security:      &SecurityDescriptor{Enabled: true},
		ImageBuild:    &ImageBuildDescriptor{ReuseExistingCommitTag: true},
	}

	srcValue := reflect.ValueOf(src)
	for i := 0; i < srcValue.NumField(); i++ {
		field := srcValue.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		Expect(srcValue.Field(i).IsZero()).To(BeFalse(),
			"the fixture leaves %s zero, so this test cannot detect it being dropped", field.Name)
	}

	got := reflect.ValueOf(src.Copy())
	for i := 0; i < got.NumField(); i++ {
		field := got.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		Expect(got.Field(i).IsZero()).To(BeFalse(),
			"ClientDescriptor.Copy() dropped %s", field.Name)
	}
}

// ChildStack is the copy the provisioners actually receive.
func TestChildStackCarriesImageBuild(t *testing.T) {
	RegisterTestingT(t)

	parent := Stack{
		Name: "parent",
		Client: ClientDescriptor{
			ImageBuild: &ImageBuildDescriptor{ReuseExistingCommitTag: true},
		},
	}
	child := parent.ChildStack("child")
	Expect(child.Client.ReuseExistingCommitTagEnabled()).To(BeTrue())
}

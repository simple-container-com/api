// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package main

import (
	"reflect"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// Every config struct in the repo that composes behaviour does it by inlining another
// struct, and the generated schema is what editors autocomplete a server.yaml from. An
// inline field's tag is `,inline`, whose name splits to the empty string — so until this
// was handled, each inline field landed under the single property key "" and overwrote
// the last. k8s.CloudrunTemplate inlines three structs; the published schema described
// one of them and silently lost `dockerRegistryURL`.
func TestInlineEmbedsAreMergedNotNested(t *testing.T) {
	RegisterTestingT(t)

	sg := NewSchemaGenerator(t.TempDir())
	schema := sg.generateJSONSchema(reflect.TypeOf(k8s.CloudrunTemplate{}))
	props := schema["properties"].(map[string]interface{})

	// The empty key is the bug's signature: one nested object standing in for however
	// many inline structs the type has.
	Expect(props).ToNot(HaveKey(""), "an inline field is still being nested under the empty key")
	Expect(schema["required"]).ToNot(ContainElement(""))

	// One field from each of the three inline structs, plus a direct field.
	Expect(props).To(HaveKey("dockerRegistryURL"), "docker.RegistryCredentials")
	Expect(props).To(HaveKey("kubeconfig"), "k8s.KubernetesConfig")
	Expect(props).To(HaveKey("useSSL"), "k8s.TlsConfig")
	Expect(props).To(HaveKey("clientTls"), "k8s.TlsConfig")
	Expect(props).To(HaveKey("originTls"), "k8s.TlsConfig")
	Expect(props).To(HaveKey("caddyResource"), "CloudrunTemplate itself")
}

// A named field tagged `yaml:",inline"` is inlined by yaml.v3 exactly as an embedded one
// is, and the yaml shape is what these schemas describe.
func TestNamedInlineFieldsAreMergedToo(t *testing.T) {
	RegisterTestingT(t)

	type inner struct {
		A string `json:"a" yaml:"a"`
	}
	type outer struct {
		Named inner  `json:",inline" yaml:",inline"`
		Own   string `json:"own" yaml:"own"`
	}

	sg := NewSchemaGenerator(t.TempDir())
	props := sg.generateJSONSchema(reflect.TypeOf(outer{}))["properties"].(map[string]interface{})

	Expect(props).ToNot(HaveKey(""))
	Expect(props).To(HaveKey("a"))
	Expect(props).To(HaveKey("own"))
}

// A field whose tag merely omits a name without asking for inlining must NOT be merged —
// `fieldName == ""` alone is not the test.
func TestOmitemptyWithoutANameIsNotInlined(t *testing.T) {
	RegisterTestingT(t)

	type inner struct {
		A string `json:"a" yaml:"a"`
	}
	type outer struct {
		Named inner `json:",omitempty" yaml:",omitempty"`
	}

	sg := NewSchemaGenerator(t.TempDir())
	props := sg.generateJSONSchema(reflect.TypeOf(outer{}))["properties"].(map[string]interface{})

	Expect(props).ToNot(HaveKey("a"))
}

// An outer field shadows a promoted one in Go, so the schema must describe the outer
// one's type — and must do so whichever way the two field names sort, since a one-pass
// first-writer-wins walk would hand the answer to the alphabet.
func TestAnOuterFieldShadowsAnInlinedOne(t *testing.T) {
	RegisterTestingT(t)

	sg := NewSchemaGenerator(t.TempDir())

	// Outer field name sorts BEFORE the embedded type's name.
	type lowerInner struct {
		Clash string `json:"clash" yaml:"clash"`
	}
	type earlyOuter struct {
		lowerInner `json:",inline" yaml:",inline"`
		Clash      bool `json:"clash" yaml:"clash"`
	}
	props := sg.generateJSONSchema(reflect.TypeOf(earlyOuter{}))["properties"].(map[string]interface{})
	Expect(props["clash"]).To(HaveKeyWithValue("type", "boolean"))

	// And AFTER it. `Zclash` > `AnInner`, so a single ordered pass would let the
	// inlined `string` win here.
	type AnInner struct {
		Clash string `json:"zclash" yaml:"zclash"`
	}
	type lateOuter struct {
		AnInner `json:",inline" yaml:",inline"`
		Zclash  bool `json:"zclash" yaml:"zclash"`
	}
	props = sg.generateJSONSchema(reflect.TypeOf(lateOuter{}))["properties"].(map[string]interface{})
	Expect(props["zclash"]).To(HaveKeyWithValue("type", "boolean"))
}

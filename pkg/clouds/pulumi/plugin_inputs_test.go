// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	scPlugin "github.com/simple-container-com/api/pkg/plugin"
)

type nestedSite struct {
	Domain        string `json:"domain" yaml:"domain"`
	IndexDocument string `json:"indexDocument" yaml:"indexDocument"`
}

type nestedHost struct {
	Zone string `json:"zone" yaml:"zone"`
}

type nestedInput struct {
	FolderID string       `json:"folderId" yaml:"folderId"`
	Public   bool         `json:"public" yaml:"public"`
	DiskSize int          `json:"diskSize" yaml:"diskSize"`
	Site     nestedSite   `json:"site" yaml:"site"`
	Hosts    []nestedHost `json:"hosts" yaml:"hosts"`
	Tags     []string     `json:"tags" yaml:"tags"`
	Missing  *nestedSite  `json:"missing" yaml:"missing"`
}

// Before nested values travelled, a plugin received only the top-level scalars of
// its config: a static site's `site` block and a database's `hosts` list vanished
// without an error. The round trip core-serialise → plugin InputsToPlain → YAML
// decode must give the plugin back the struct core started from.
func TestSerializeConfigToInputs_NestedRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	in := nestedInput{
		FolderID: "b1g",
		Public:   true,
		DiskSize: 10,
		Site:     nestedSite{Domain: "a.example.ru", IndexDocument: "index.html"},
		Hosts:    []nestedHost{{Zone: "ru-central1-a"}, {Zone: "ru-central1-b"}},
		Tags:     []string{"x", "y"},
	}
	inputs := serializeConfigToInputs(&in)
	Expect(inputs).To(HaveKey("site"))
	Expect(inputs).To(HaveKey("hosts"))
	Expect(inputs).NotTo(HaveKey("missing"), "a JSON null is omitted, not sent as an empty value")

	b, err := yaml.Marshal(scPlugin.InputsToPlain(inputs))
	Expect(err).ToNot(HaveOccurred())
	var out nestedInput
	Expect(yaml.Unmarshal(b, &out)).To(Succeed())
	Expect(out).To(Equal(in))
}

func TestSerializeConfigToInputs_NilAndScalars(t *testing.T) {
	RegisterTestingT(t)

	Expect(serializeConfigToInputs(nil)).To(BeEmpty())
	inputs := serializeConfigToInputs(map[string]any{"s": "v", "b": false, "n": 2})
	Expect(inputs["s"].GetStringValue()).To(Equal("v"))
	Expect(inputs["b"].GetBoolValue()).To(BeFalse())
	Expect(inputs["n"].GetDoubleValue()).To(Equal(2.0))
}

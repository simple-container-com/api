// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_stack

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestMarshalOutputs(t *testing.T) {
	RegisterTestingT(t)

	res := api.OutputsResult{
		StackName: "infra",
		Outputs: map[string]any{
			"infra--mongodb--staging-mongo-uri":  "mongodb+srv://cluster0.example.net",
			"forge-storage--staging-bucket-name": "forge-storage-staging",
		},
	}

	t.Run("indented by default", func(t *testing.T) {
		RegisterTestingT(t)

		j, err := marshalOutputs(res, false)
		Expect(err).ToNot(HaveOccurred())

		// Two-space indent, one key per line. forge's infra returns ~20 keys;
		// on a single line that wraps into a block nobody can read.
		Expect(string(j)).To(ContainSubstring("\n  \"outputs\": {"))
		Expect(strings.Count(string(j), "\n")).To(BeNumerically(">", 3),
			"output is not actually indented: %s", j)
	})

	t.Run("--compact restores the single line", func(t *testing.T) {
		RegisterTestingT(t)

		j, err := marshalOutputs(res, true)
		Expect(err).ToNot(HaveOccurred())

		// The escape hatch for anything that depended on the old format.
		Expect(string(j)).ToNot(ContainSubstring("\n"))
	})

	t.Run("both shapes parse to the same document", func(t *testing.T) {
		RegisterTestingT(t)

		// Indenting is a presentation change only — a consumer piping either
		// form into jq has to see identical data.
		var pretty, oneLine map[string]any
		j1, err := marshalOutputs(res, false)
		Expect(err).ToNot(HaveOccurred())
		j2, err := marshalOutputs(res, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(json.Unmarshal(j1, &pretty)).To(Succeed())
		Expect(json.Unmarshal(j2, &oneLine)).To(Succeed())
		Expect(pretty).To(Equal(oneLine))
	})
}

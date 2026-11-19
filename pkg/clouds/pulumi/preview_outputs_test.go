// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"encoding/json"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// A byo vm resource exports its host-port and host-count as `sdk.Int` and its
// ingress flag as `sdk.Bool`. Those arrive back off the checkpoint's JSON as
// native Go scalars, which used to miss every branch of toOutputsResult's type
// switch and render as the zero-value "" — so `sc stack outputs` reported them
// as empty strings. Pin the real values.
func TestToOutputsResultRendersNonStringScalars(t *testing.T) {
	p := &pulumi{}

	for name, tc := range map[string]struct {
		value any
		want  string
	}{
		// The numeric shapes Pulumi is known to hand back for these exports —
		// see toInt in pkg/clouds/pulumi/vm/compose_stack.go, which accepts the
		// same set for the very same host-count/host-port outputs.
		"host-port as float64":     {value: float64(2222), want: "2222"},
		"host-count as float64":    {value: float64(1), want: "1"},
		"host-port as json.Number": {value: json.Number("2222"), want: "2222"},
		"host-count as int":        {value: 3, want: "3"},
		"host-port as int64":       {value: int64(22), want: "22"},
		// Large ports/counts must not come back in scientific notation, and a
		// whole number must not grow a fractional part.
		"big count stays decimal": {value: float64(1000000), want: "1000000"},
		// Booleans: `sdk.Bool`, e.g. a vm's ingress-enabled output.
		"ingress-enabled true":  {value: true, want: "true"},
		"ingress-enabled false": {value: false, want: "false"},

		// Regression guards — the branches that already worked must be untouched.
		"string passes through unquoted": {value: "203.0.113.5", want: "203.0.113.5"},
		"empty string stays empty":       {value: "", want: ""},
		// An absent output must keep rendering as "", not the literal "null".
		"nil renders empty": {value: nil, want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			out := p.toOutputsResult("my-stack", auto.OutputMap{
				"out": auto.OutputValue{Value: tc.value},
			})
			if got := out.Outputs["out"]; got != tc.want {
				t.Errorf("value %#v rendered as %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// A map output is marshalled as JSON, and a secret keeps being marshalled
// through the secret branch rather than leaking its bare value.
func TestToOutputsResultRendersMapsAndSecrets(t *testing.T) {
	p := &pulumi{}

	out := p.toOutputsResult("my-stack", auto.OutputMap{
		"obj":      auto.OutputValue{Value: map[string]any{"a": "b"}},
		"secret":   auto.OutputValue{Value: "s3cret", Secret: true},
		"secretno": auto.OutputValue{Value: float64(42), Secret: true},
	})

	if got, want := out.Outputs["obj"], `{"a":"b"}`; got != want {
		t.Errorf("map output rendered as %q, want %q", got, want)
	}
	// A string-valued secret hits the string branch first — that is pre-existing
	// behaviour and this test exists to notice if it ever changes.
	if got, want := out.Outputs["secret"], "s3cret"; got != want {
		t.Errorf("string secret rendered as %q, want %q", got, want)
	}
	// A numeric secret goes through the secret branch, which marshals the whole
	// OutputValue; the new scalar branch must not have stolen it.
	if got, want := out.Outputs["secretno"], `{"Value":42,"Secret":true}`; got != want {
		t.Errorf("numeric secret rendered as %q, want %q", got, want)
	}

	if out.StackName != "my-stack" {
		t.Errorf("StackName = %q, want %q", out.StackName, "my-stack")
	}
}

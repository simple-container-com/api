// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"gopkg.in/yaml.v3"

	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// DecodeInputs unmarshals a plugin RPC's inputs map into a typed Go struct
// via a yaml round-trip. Uses scPlugin.InputsToPlain to flatten the proto
// InputValue tree first — nested maps and lists (e.g. stackConfig.image.*
// in an aws lambda deploy) round-trip correctly, which the earlier
// scalar-only variants in aws/mongodb/cloudru's main.go lost. Live-caught
// against forge-notifier staging preview 2026-10-30.
//
// Returns yaml.Unmarshal's error directly; callers typically wrap it with
// the resource-type context.
func DecodeInputs[T any](inputs map[string]*scpluginv1.InputValue, out *T) error {
	plain := scPlugin.InputsToPlain(inputs)
	b, err := yaml.Marshal(plain)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, out)
}

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"github.com/simple-container-com/api/pkg/api"
)

// NewComponentConfigValidator returns an api.ComponentValidator backed by
// this package's fleet-wide registration maps. Used by SC core's provisioner
// (pkg/clouds/pulumi/provisioner.go) to run parse-time validation of SC
// Component resources after RegisterSidecars has populated the maps.
//
// Kept in this package (not pkg/api) so that the validator has direct access
// to ComponentResourceTypes / ComponentAllowedOverrides / ProvisionFuncByType
// without pkg/api needing a dependency on pkg/clouds/pulumi/api.
func NewComponentConfigValidator() api.ComponentValidator {
	return &componentConfigValidator{}
}

type componentConfigValidator struct{}

func (componentConfigValidator) IsComponentType(typeToken string) bool {
	return IsComponentResourceType(typeToken)
}

func (componentConfigValidator) IsOverrideAllowed(typeToken, key string) bool {
	return IsComponentOverrideAllowed(typeToken, key)
}

func (componentConfigValidator) TemplateTypeExists(templateType string) bool {
	_, ok := ProvisionFuncByType[templateType]
	return ok
}

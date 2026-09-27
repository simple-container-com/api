// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	TemplateTypeYandexServerlessContainer = "yc-serverless-container"
)

// ServerlessContainerInput is what the provisioning tier receives for a
// `yc-serverless-container` stack. It mirrors aws.LambdaInput: resolved account
// credentials plus the client stack config, kept whole.
//
// Keeping the whole StackConfig (rather than flattening the fields the converter
// happens to care about) is what lets the provisioning tier decode cloudExtras
// later — the same reason ToAwsLambdaConfig does it.
type ServerlessContainerInput struct {
	AccountConfig `json:",inline" yaml:",inline"`
	// RegistryID and ServiceAccountID are carried over from the template, since
	// ConvertAuth rehydrates only credential fields.
	RegistryID       string                     `json:"registryId,omitempty" yaml:"registryId,omitempty"`
	ServiceAccountID string                     `json:"serviceAccountId,omitempty" yaml:"serviceAccountId,omitempty"`
	StackConfig      api.StackConfigSingleImage `json:"stackConfig" yaml:"stackConfig"`
}

func (i *ServerlessContainerInput) Uses() []string {
	return i.StackConfig.Uses
}

func (i *ServerlessContainerInput) DependsOnResources() []api.StackConfigDependencyResource {
	return i.StackConfig.Dependencies
}

func (i *ServerlessContainerInput) OverriddenBaseZone() string {
	return i.StackConfig.BaseDnsZone
}

// CloudExtras decodes and validates the client stack's cloudExtras block.
func (i *ServerlessContainerInput) CloudExtras() (*CloudExtras, error) {
	return ReadCloudExtras(i.StackConfig.CloudExtras)
}

// ToServerlessContainerConfig converts a `yc-serverless-container` template plus a
// single-image client stack into the provisioning input.
func ToServerlessContainerConfig(tpl any, stackCfg *api.StackConfigSingleImage) (any, error) {
	templateCfg, ok := tpl.(*TemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type *yandex.TemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}
	if stackCfg == nil {
		return nil, errors.Errorf("stack config cannot be nil")
	}

	accountConfig := &AccountConfig{}
	if err := api.ConvertAuth(&templateCfg.AccountConfig, accountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert yandex account config")
	}

	res := &ServerlessContainerInput{
		AccountConfig:    *accountConfig,
		RegistryID:       templateCfg.RegistryID,
		ServiceAccountID: templateCfg.ServiceAccountID,
		StackConfig:      *stackCfg,
	}

	// Fail here rather than mid-provision: a malformed schedule is a config error
	// and the operator is looking at the config right now.
	extras, err := res.CloudExtras()
	if err != nil {
		return nil, err
	}
	// A per-service identity overrides the template's.
	if extras.ServiceAccountID != "" {
		res.ServiceAccountID = extras.ServiceAccountID
	}

	return res, nil
}

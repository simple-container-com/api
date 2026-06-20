// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package fs

import (
	"github.com/simple-container-com/api/pkg/api"
)

func init() {
	api.RegisterProvisionerFieldConfig(api.ProvisionerFieldConfigRegister{
		StateStorageTypeFileSystem: func(config *api.Config) (api.Config, error) {
			return api.ConvertConfig(config, &FileSystemStateStorage{})
		},
		SecretsProviderTypePassphrase: func(config *api.Config) (api.Config, error) {
			return api.ConvertConfig(config, &PassphraseSecretsProvider{})
		},
	})
}

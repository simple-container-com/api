// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

// AuthTypeCloudruEvolutionKey is the `type:` of an `auth:` entry holding a
// Cloud.ru **Evolution** IAM service-account key.
//
// A second Cloud.ru auth type rather than more fields on AuthTypeCloudruAccessKey,
// because Cloud.ru sells two unrelated clouds under one brand and a credential for
// one does not authenticate against the other:
//
//   - **Advanced** (ex-SberCloud) is OpenStack-descended and authenticates a
//     Keystone access/secret pair against `iam.<region>.hc.sbercloud.ru` — that is
//     AuthTypeCloudruAccessKey.
//   - **Evolution** is Cloud.ru's own platform and authenticates a service-account
//     key id + secret against `iam.api.cloud.ru`, returning a one-hour bearer
//     token — this one.
//
// Sharing one auth type between them would make every field conditionally
// meaningful and would let a config authenticate successfully against the wrong
// cloud, which is an estate with different resources and a different bill.
const AuthTypeCloudruEvolutionKey = "cloudru-evolution-key"

// EvolutionAccountConfig is the credential blob a `${auth:<name>}` reference
// resolves to for Cloud.ru Evolution.
type EvolutionAccountConfig struct {
	// KeyID / KeySecret are the service-account key issued in the Cloud.ru console
	// (Customer → service accounts → keys). KeySecret is a credential: write it as
	// `${secret:<name>}`.
	//
	// The same pair is also the `docker login` credential for the project's
	// Artifact Registry, so there is deliberately no separate registry credential
	// to configure.
	KeyID     string `json:"keyId" yaml:"keyId"`
	KeySecret string `json:"keySecret" yaml:"keySecret"`

	// ProjectID is the Evolution project (a UUID, visible in the console URL as
	// `projectId=`). Required, and not inferable from the key: every Evolution
	// endpoint is project-scoped and a call that omits it answers 403
	// `non_authorized` — which reads exactly like a missing role, and sends the
	// operator off granting permissions that were never the problem.
	ProjectID string `json:"projectId" yaml:"projectId"`

	api.Credentials `json:",inline" yaml:",inline"`
}

func (r *EvolutionAccountConfig) ProviderType() string {
	return ProviderType
}

// CredentialsValue serialises the whole account config when no opaque credentials
// blob was supplied — api.ConvertAuth round-trips this string back into an
// EvolutionAccountConfig, so every field above must survive JSON.
func (r *EvolutionAccountConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", api.AuthToString(r)).Else(r.Credentials.Credentials)
}

// ProjectIdValue is what `${auth:<name>.projectId}` interpolates to. Unlike the
// Advanced config — where the scoping input is a Keystone project *name* — this is
// a real id, and it is the value the REST API wants.
func (r *EvolutionAccountConfig) ProjectIdValue() string {
	return r.ProjectID
}

// Validate refuses a config that cannot authenticate, naming the field, rather
// than letting the first API call fail with a 401 or a 403 that says nothing
// about which half is missing.
func (r *EvolutionAccountConfig) Validate() error {
	if r.KeyID == "" {
		return errors.Errorf("`keyId` is required for auth type %q", AuthTypeCloudruEvolutionKey)
	}
	if r.KeySecret == "" {
		return errors.Errorf("`keySecret` is required for auth type %q — write it as `${secret:<name>}`",
			AuthTypeCloudruEvolutionKey)
	}
	if r.ProjectID == "" {
		return errors.Errorf("`projectId` is required for auth type %q: every Cloud.ru Evolution endpoint is "+
			"project-scoped and answers 403 `non_authorized` without it", AuthTypeCloudruEvolutionKey)
	}
	return nil
}

func ReadAuthEvolutionKeyConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &EvolutionAccountConfig{})
}

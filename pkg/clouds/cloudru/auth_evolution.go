// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"sort"
	"strings"

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
	// (Customer → service accounts → keys).
	//
	// Both go in VERBATIM. An `auth:` entry lives in `secrets.yaml`, which is itself
	// the encrypted store, and placeholders are NOT expanded inside it — a
	// `${secret:<name>}` written here reaches the cloud literally (see Validate,
	// which refuses it rather than letting IAM answer `key_id does not match
	// required format`). `${secret:…}` is for the plaintext files: `server.yaml` and
	// `client.yaml`.
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
		return errors.Errorf("`keySecret` is required for auth type %q", AuthTypeCloudruEvolutionKey)
	}
	if r.ProjectID == "" {
		return errors.Errorf("`projectId` is required for auth type %q: every Cloud.ru Evolution endpoint is "+
			"project-scoped and answers 403 `non_authorized` without it", AuthTypeCloudruEvolutionKey)
	}
	return unexpandedPlaceholder(AuthTypeCloudruEvolutionKey, map[string]string{
		"keyId":     r.KeyID,
		"keySecret": r.KeySecret,
		"projectId": r.ProjectID,
	})
}

// unexpandedPlaceholder refuses an auth field that still holds a `${…}` placeholder.
//
// An `auth:` entry lives in `secrets.yaml` — the encrypted store itself — and SC
// does not expand placeholders inside it. So a `${secret:CLOUDRU_KEY_ID}` written
// there is passed to the cloud VERBATIM, which validates fine and then fails much
// later and much further away: Cloud.ru IAM answers `HTTP 400 … (key_id) is invalid
// argument: key_id does not match required format`, a message that reads like a
// revoked or mistyped key rather than like a config that was never expanded.
// Live-caught 2026-10-24, mid-deploy, after the stack had already created resources.
//
// Shared by both Cloud.ru auth types because the mistake is a property of
// `secrets.yaml`, not of either cloud.
func unexpandedPlaceholder(authType string, fields map[string]string) error {
	// Sorted so the message does not depend on map iteration order.
	names := lo.Keys(fields)
	sort.Strings(names)
	for _, field := range names {
		if !strings.Contains(fields[field], "${") {
			continue
		}
		return errors.Errorf(
			"`%s` in auth type %q still holds a `${…}` placeholder. An `auth:` entry lives in `secrets.yaml`, "+
				"which IS the encrypted store, so placeholders are not expanded there — write the value verbatim. "+
				"`${secret:<name>}` belongs in `server.yaml` / `client.yaml`", field, authType)
	}
	return nil
}

func ReadAuthEvolutionKeyConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &EvolutionAccountConfig{})
}

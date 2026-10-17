// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/secretsmanager"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
)

// bundleEnvName — the env name every opted-in service's bootstrap reads to
// locate the bundle ARN (SDK: `awsutil.SecretsBundleEnv`). Must stay in
// lockstep with `go-aws-lambda-sdk/pkg/awsutil/secrets.go` constant of
// the same value. Design 2026-10-17 §3.3.
const bundleEnvName = "FORGE_SECRETS_BUNDLE_ARN"

// buildBundlePayload gathers every secret the Lambda / ECS service would
// otherwise get as a per-key Secrets Manager ARN env, and returns the
// map that becomes the bundle's JSON payload. Keys are env-var names
// (verbatim what the service reads via `os.Getenv` / `GetEnvOrSecret`);
// values are the plaintext resolved at deploy time.
//
// The two sources mirror the current `secrets` list build in
// aws_lambda.go:189-204 + ecs_fargate.go:161-182 so bundle mode is a
// drop-in replacement, not a different source of truth.
func buildBundlePayload(ctxVars []secretValue, stackSecrets map[string]string) map[string]string {
	out := make(map[string]string, len(ctxVars)+len(stackSecrets))
	for _, v := range ctxVars {
		out[v.Name] = v.Value
	}
	for name, value := range stackSecrets {
		out[name] = value
	}
	return out
}

// secretValue — minimal {Name, Value} slice of the pApi.ComputeEnvVariable
// shape used by buildBundlePayload. Local type so the helper doesn't
// depend on the pulumi/api package graph (keeps unit testing hermetic).
type secretValue struct {
	Name  string
	Value string
}

// createSecretsBundle creates ONE AWS Secrets Manager secret with a JSON
// payload of {envName → plaintext} drawn from BOTH `ctxVars` (the
// stackConfig's declared secret envs) and `stackSecrets` (the
// `.sc/stacks/<stack>/secrets.yaml` `values:` registry), plus ONE
// `SecretVersion`. Returns a `*CreatedSecret` whose `EnvVar` is the
// well-known `FORGE_SECRETS_BUNDLE_ARN` so the Lambda env map gets ONE
// entry instead of N. Design 2026-10-17 §3.2 + §3.3.
//
// MaxEntries from the operator's `cloudExtras.secretsBundle.maxEntries`
// gates runaway `env:` growth — zero means no check.
//
// Opts should carry the same provider + dependency wiring the caller
// uses for `createSecret`, so the bundle secret deploys under the same
// stack as the Lambda / ECS service.
func createSecretsBundle(
	ctx *sdk.Context,
	deployParams api.StackParams,
	cfg *aws.SecretsBundleConfig,
	ctxVars []secretValue,
	stackSecrets map[string]string,
	tags sdk.StringMap,
	opts ...sdk.ResourceOption,
) (*CreatedSecret, error) {
	if cfg == nil || !cfg.IsEnabled() {
		return nil, fmt.Errorf("createSecretsBundle: nil or disabled config (caller should branch on IsEnabled)")
	}

	payload := buildBundlePayload(ctxVars, stackSecrets)

	if cfg.MaxEntries > 0 && len(payload) > cfg.MaxEntries {
		return nil, errors.Errorf(
			"secretsBundle: %d entries exceeds configured maxEntries %d (raise maxEntries or trim the service's `env:` block)",
			len(payload), cfg.MaxEntries,
		)
	}

	// JSON-marshal is deterministic by key order (Go spec guarantees
	// map iteration order is NOT deterministic, but json.Marshal sorts
	// keys alphabetically — see encoding/json docs). This keeps the
	// SecretVersion diff tight across deploys when nothing changed.
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "secretsBundle: marshal payload")
	}

	// AWS Secrets Manager caps a single secret value at 65 536 bytes.
	// Reject early so the operator sees the real cause instead of a
	// cryptic AWS 400 at deploy time.
	if len(raw) > 65536 {
		return nil, errors.Errorf(
			"secretsBundle: payload %d bytes exceeds AWS Secrets Manager 65 KB cap (shrink values or split the service)",
			len(raw),
		)
	}

	secretName := cfg.Name
	if secretName == "" {
		secretName = toSecretName(deployParams, "bundle", "", "secrets", deployParams.Version)
	}

	args := &secretsmanager.SecretArgs{
		Name:                 sdk.String(secretName),
		RecoveryWindowInDays: sdk.IntPtr(0),
		Tags:                 tags,
	}
	if cfg.KMSKeyID != "" {
		args.KmsKeyId = sdk.String(cfg.KMSKeyID)
	}

	secret, err := secretsmanager.NewSecret(ctx, secretName, args, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "secretsBundle: create Secret")
	}
	_, err = secretsmanager.NewSecretVersion(ctx, secretName, &secretsmanager.SecretVersionArgs{
		SecretId:     secret.Arn,
		SecretString: sdk.String(string(raw)),
	}, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "secretsBundle: create SecretVersion")
	}

	return &CreatedSecret{
		Secret: secret,
		EnvVar: bundleEnvName,
	}, nil
}

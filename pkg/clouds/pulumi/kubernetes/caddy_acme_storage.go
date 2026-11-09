// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	caddyassets "github.com/simple-container-com/api/pkg/clouds/caddy"
)

// caddyGlobalOptsIndent is one level inside the global options block
// BuildCaddyfileGlobalOptions wraps. Two spaces, matching
// BuildTrustedProxiesBlock's `  servers {` — the two blocks sit side by side in
// one `{ ... }` and a mixed-indent global block is the first thing a reviewer
// mistrusts in a generated file.
const caddyGlobalOptsIndent = "  "

// caddyAcmeStorageBlock renders this edge's `storage s3` block and the `persist_config off`
// that keeps its credentials out of Caddy's config autosave, or "" when the edge has no
// shared store. It also validates the store, so that a refusal happens here rather than
// inside a Pulumi apply where the message reaches the operator wrapped in a stack trace.
//
// Rendered by pkg/clouds/caddy, the same code the vm edge's Caddyfile goes through —
// certmagic-s3's two silent parser traps are encoded there once. The credentials it
// renders as `{$...}` placeholders are delivered as secret envs by DeployCaddyService.
//
// The returned block has no trailing newline: BuildCaddyfileGlobalOptions joins the
// blocks it is given with "\n".
//
// `namespace` and `deploymentName` only furnish the comment that tells an operator whose
// certificates never issued which log to read. Spelled out rather than left generic
// because the namespace defaults to `caddy` and the deployment name is derived from the
// environment, so a reader of the file has no way to guess either.
func caddyAcmeStorageBlock(caddy CaddyDeployment, namespace, deploymentName string) (string, error) {
	cfg := lo.FromPtr(caddy.CaddyConfig)
	if !cfg.AcmeStorageEnabled() {
		// A set-but-bucketless store is still validated: it is the shape an operator
		// reaches by commenting out one line, and silently serving without a shared
		// store is exactly the failure this field exists to prevent.
		return "", cfg.AcmeStorage.Validate("caddy.acmeStorage")
	}
	if err := cfg.AcmeStorage.Validate("caddy.acmeStorage"); err != nil {
		return "", err
	}
	// GKE's cluster resource provisions its own GCS bucket and renders a `storage gcs`
	// block into CaddyfilePrefixOut (gcp/gke_autopilot.go). Two `storage` directives in
	// one global options block is a Caddy config-load failure, so one of them would have
	// to be dropped — and dropping either silently is worse than refusing: the operator
	// would be reading the field SC ignored.
	if isPulumiOutputSet(caddy.CaddyfilePrefixOut) {
		return "", errors.Errorf(
			"`caddy.acmeStorage` is set on a cluster that already provisions its own certificate store " +
				"(this cloud's cluster resource renders a `storage` block itself). Caddy accepts only one " +
				"`storage` directive, so remove `caddy.acmeStorage` and let the cluster's own store serve",
		)
	}

	b := &strings.Builder{}
	caddyassets.WriteAcmeStorageGlobals(b, cfg.AcmeStorage, caddyGlobalOptsIndent,
		fmt.Sprintf("`kubectl logs -n %s deploy/%s`", namespace, deploymentName))
	return strings.TrimRight(b.String(), "\n"), nil
}

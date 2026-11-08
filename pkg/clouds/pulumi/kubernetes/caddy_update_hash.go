// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"crypto/md5"
	"encoding/hex"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// caddyUpdateHashValue is the decision CaddyUpdateHash makes once its outputs have
// resolved: prefer the caddyfile-entry annotation as applied to the Service, fall back to
// the statically rendered entry.
//
// The fallback covers a workload with no ports (no Service at all) and a Service that
// carries no caddyfile-entry annotation. Both already produced a static-entry hash before
// this helper existed, so neither changes behaviour.
func caddyUpdateHashValue(appliedEntry, staticEntry string) string {
	if appliedEntry != "" {
		return md5Hex(appliedEntry)
	}
	return md5Hex(staticEntry)
}

// CaddyUpdateHash is the value of AnnotationCaddyUpdateHash: md5 of the caddyfile-entry
// annotation as APPLIED to the Service. It lives on the Caddy deployment's pod template,
// so changing it rolls the edge's pods.
//
// Taken over the applied annotation rather than over the statically rendered entry, which
// buys two things:
//
//   - **Ordering.** PatchDeployment registers no Pulumi resource — it performs the patch
//     inside an apply — so sdk.DependsOn passed to it has never had any effect, and the
//     patch could land before the annotation it hashed was visible to a `kubectl get`
//     (measured 2026-11-08: pod template at 16:16:30, Service annotation at 16:16:39).
//     A pod rolled in that window renders the previous config and serves it for its whole
//     lifetime. Reading the value out of the Service's applied state makes the patch's
//     own input depend on that state, which is the one ordering mechanism an
//     apply-driven patch has.
//   - **Reproducibility.** The Caddy init container reads the annotation back off the live
//     Service and compares digests to decide whether the state it rendered from is new
//     enough (see caddyfileRenderScript). A migrated stack's annotation is re-rendered
//     against the legacy namespace while sc.CaddyfileEntry holds the static render, so
//     hashing the static one gives a digest the init container can never produce, and
//     every Caddy pod on such a stack would wait out the full fail-open window.
//
// Stacks whose applied and static entries differ (migrated ones) see one extra Caddy
// rollout the first time this lands, which is a rolling update with maxUnavailable: 0.
func CaddyUpdateHash(svc *corev1.Service, staticEntry sdk.StringInput) sdk.StringOutput {
	static := staticEntry.ToStringOutput()
	if svc == nil {
		return static.ApplyT(func(entry string) string {
			return caddyUpdateHashValue("", entry)
		}).(sdk.StringOutput)
	}
	return sdk.All(svc.Metadata.Annotations(), static).ApplyT(func(vals []any) string {
		applied := ""
		if annotations, ok := vals[0].(map[string]string); ok {
			applied = annotations[AnnotationCaddyfileEntry]
		}
		return caddyUpdateHashValue(applied, vals[1].(string))
	}).(sdk.StringOutput)
}

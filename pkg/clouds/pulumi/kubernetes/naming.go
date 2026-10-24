// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// generateResourceName creates resource names with environment-specific suffixes for custom stacks
// When parentEnv differs from stackEnv, it adds the stackEnv as a suffix to ensure uniqueness
func generateResourceName(serviceName, stackEnv, parentEnv string, resourceType string) string {
	baseName := serviceName

	// Add stack environment suffix for custom stacks (when parentEnv differs from stackEnv)
	if parentEnv != "" && parentEnv != stackEnv {
		baseName = fmt.Sprintf("%s-%s", serviceName, stackEnv)
	}

	// Add resource type suffix if specified
	if resourceType != "" {
		return fmt.Sprintf("%s-%s", baseName, resourceType)
	}

	return baseName
}

// generateDeploymentName creates deployment name with environment suffix for custom stacks
func generateDeploymentName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "")
}

// generateServiceName creates service name with environment suffix for custom stacks
func generateServiceName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "")
}

// generateConfigMapName creates configmap name with environment suffix for custom stacks
func generateConfigMapName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "config")
}

// generateSecretName creates secret name with environment suffix for custom stacks
func generateSecretName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "secrets")
}

// generateHPAName creates HPA name with environment suffix for custom stacks
func generateHPAName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "hpa")
}

// generateVPAName creates VPA name with environment suffix for custom stacks
func generateVPAName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "vpa")
}

// generateConfigVolumesName creates config volumes configmap name with environment suffix for custom stacks
func generateConfigVolumesName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "cfg-volumes")
}

// generateSecretVolumesName creates secret volumes secret name with environment suffix for custom stacks
func generateSecretVolumesName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "secret-volumes")
}

// generateImagePullSecretName creates image pull secret name with environment suffix for custom stacks
func generateImagePullSecretName(serviceName, stackEnv, parentEnv string) string {
	return generateResourceName(serviceName, stackEnv, parentEnv, "docker-config")
}

// isCustomStack determines if this is a custom stack deployment
// Returns true when parentEnv is set and differs from stackEnv
func isCustomStack(stackEnv, parentEnv string) bool {
	return parentEnv != "" && parentEnv != stackEnv
}

// GenerateNamespaceName derives the physical k8s namespace name for a stack.
// Standard stacks (parentEnv unset, or parentEnv == stackEnv) keep baseNamespace
// (typically the stackName). Custom stacks (parentEnv set and differing from
// stackEnv, e.g. parentEnv=production with stackEnv=tenant-a/tenant-b/...) get
// baseNamespace suffixed with stackEnv, mirroring the per-stackEnv suffix every
// other resource type (Deployment, Service, Secret, ConfigMap, HPA, VPA,
// ImagePullSecret) already gets via generateResourceName.
//
// The result is sanitized to comply with Kubernetes RFC 1123 (lowercase
// alphanumeric and `-`, ≤63 chars with FNV-1a truncation hash) so callers can
// pass it directly into `metadata.namespace` without an extra sanitization
// step. Sanitization is idempotent — callers that pre-sanitize their inputs
// see no behavioural change.
//
// Without this isolation, sibling sub-env stacks share one physical namespace,
// and any `pulumi destroy` on a sub-env cascade-deletes every sibling's resources
// via the k8s namespace delete API. Migrating an existing custom stack to its
// dedicated namespace is automatic on the next `pulumi up`: Pulumi sees the
// namespace metadata.Name change and Replaces the namespace plus its
// namespace-scoped resources. The namespace is created with RetainOnDelete (see
// NewSimpleContainer), so the parent's shared namespace is left in place — the
// parent stack's resources continue running through the migration.
func GenerateNamespaceName(baseNamespace, stackEnv, parentEnv string) string {
	name := baseNamespace
	if isCustomStack(stackEnv, parentEnv) {
		name = fmt.Sprintf("%s-%s", baseNamespace, stackEnv)
	}
	return SanitizeK8sName(name)
}

// GenerateCaddyDeploymentName creates the Caddy deployment name with environment suffix
// Caddy deployments always include the environment suffix for backwards compatibility
// This is exported so it can be used by both kubernetes and gcp packages for consistency
func GenerateCaddyDeploymentName(stackEnv string) string {
	// Always add environment suffix for Caddy deployments (backwards compatibility)
	if stackEnv != "" {
		return fmt.Sprintf("caddy-%s", stackEnv)
	}
	return "caddy"
}

// CaddyDeploymentNameForChild returns the Caddy deployment name a *child* (client) stack
// must target when patching annotations to trigger a Caddy rolling restart.
//
// Caddy is provisioned by the parent infra stack, so its deployment name is keyed on
// parentEnv (e.g. caddy-production). For sub-env client stacks where parentEnv differs
// from stackEnv (e.g. parentEnv=production, stackEnv=tenant-a), passing stackEnv would
// produce caddy-tenant-a — which doesn't exist — and the patch would fail silently.
// For single-env stacks (parentEnv empty or equal to stackEnv) this falls back to stackEnv.
//
// Note: this is the call site asymmetric to GenerateCaddyDeploymentName, which is used
// from the parent stack's own provisioning where Environment is the correct input.
func CaddyDeploymentNameForChild(stackEnv, parentEnv string) string {
	if isCustomStack(stackEnv, parentEnv) {
		return GenerateCaddyDeploymentName(parentEnv)
	}
	return GenerateCaddyDeploymentName(stackEnv)
}

// KubernetesCaddyDeploymentName names the objects of a kubernetes-caddy resource. It
// returns "" (GenerateCaddyDeploymentName: caddy-<env>, the name every Caddy had before)
// unless the resource sets useResourceName, which names them <resource>-<env>.
//
// The rule reads only the resource itself, never its neighbours: if it depended on
// whether the environment holds another Caddy, adding or removing that Caddy would
// rename this one, and Pulumi replaces a renamed Caddy. Two Caddies of one environment
// that end up with the same name are refused here, before Pulumi reports a duplicate URN.
//
// caddy.deploymentName is not the switch: it is an adoption field, and live configs carry
// values that do not match the deployment SC created (an Autopilot caddy with
// deploymentName "caddy" runs as caddy-<env>).
func KubernetesCaddyDeploymentName(resName, env string, useResourceName bool, envResources map[string]api.ResourceDescriptor) (string, error) {
	name := caddyObjectsName(resName, env, useResourceName)
	var clashes []string
	for other, res := range envResources {
		if other == resName {
			continue
		}
		if otherName, ok := envCaddyObjectsName(other, env, res); ok && otherName == name {
			clashes = append(clashes, other)
		}
	}
	if len(clashes) > 0 {
		sort.Strings(clashes)
		return "", errors.Errorf("kubernetes-caddy %q in %q names its objects %q, as %v already does: set useResourceName: true on %q (and give it a name other than \"caddy\")",
			resName, env, name, clashes, resName)
	}
	if name == GenerateCaddyDeploymentName(env) {
		return "", nil
	}
	return name, nil
}

func caddyObjectsName(resName, env string, useResourceName bool) string {
	if !useResourceName {
		return GenerateCaddyDeploymentName(env)
	}
	return fmt.Sprintf("%s-%s", resName, env)
}

// envCaddyObjectsName is the name the Caddy objects of another resource in the
// environment take, and false for a resource that creates none.
func envCaddyObjectsName(resName, env string, res api.ResourceDescriptor) (string, bool) {
	switch res.Type {
	case k8s.ResourceTypeCaddy:
		cfg, ok := res.Config.Config.(*k8s.CaddyResource)
		if !ok {
			var err error
			if cfg, err = api.ConvertDescriptor(res.Config.Config, &k8s.CaddyResource{}); err != nil {
				return "", false
			}
		}
		return caddyObjectsName(resName, env, cfg.UseResourceName), true
	case gcloud.ResourceTypeGkeAutopilot:
		if deploysEmbeddedCaddy(res) {
			return GenerateCaddyDeploymentName(env), true
		}
	}
	return "", false
}

// deploysEmbeddedCaddy mirrors the condition under which GkeAutopilot calls
// DeployCaddyService: an adopted cluster creates no Caddy objects of its own.
func deploysEmbeddedCaddy(res api.ResourceDescriptor) bool {
	cfg, ok := res.Config.Config.(*gcloud.GkeAutopilotResource)
	if !ok {
		var err error
		if cfg, err = api.ConvertDescriptor(res.Config.Config, &gcloud.GkeAutopilotResource{}); err != nil {
			return false
		}
	}
	return cfg.Caddy != nil && !cfg.Adopt
}

// ChildCaddyTarget returns the Caddy deployment and namespace a child stack patches,
// read from the caddy config its parent exported.
func ChildCaddyTarget(cfg *k8s.CaddyConfig, stackEnv, parentEnv string) (deployment, namespace string) {
	deployment = CaddyDeploymentNameForChild(stackEnv, parentEnv)
	namespace = "caddy"
	if cfg == nil {
		return deployment, namespace
	}
	if cfg.DeploymentName != nil {
		deployment = *cfg.DeploymentName
	}
	if cfg.Namespace != nil {
		namespace = *cfg.Namespace
	}
	return deployment, namespace
}

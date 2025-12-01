package kubernetes

import "fmt"

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

// resolveNamespace determines the target namespace based on parentEnv
// For both custom stacks and standard stacks, use the stack's own environment as namespace
// Custom stacks should deploy to their own namespace, not the parent's namespace
func resolveNamespace(stackEnv, parentEnv string) string {
	// Always use the stack's own environment as namespace
	// Custom stacks deploy to their own namespace (e.g., "preprod")
	// Standard stacks also deploy to their own namespace
	return stackEnv
}

// isCustomStack determines if this is a custom stack deployment
// Returns true when parentEnv is set and differs from stackEnv
func isCustomStack(stackEnv, parentEnv string) bool {
	return parentEnv != "" && parentEnv != stackEnv
}

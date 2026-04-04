package kubernetes

import (
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// BuildTrustedProxiesBlock builds the Caddy servers { trusted_proxies ... } block
// from the CaddyConfig. Returns empty string if no trusted proxies are configured.
func BuildTrustedProxiesBlock(cfg k8s.CaddyConfig) (string, error) {
	if len(cfg.TrustedProxies) == 0 {
		return "", nil
	}

	for _, cidr := range cfg.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			if net.ParseIP(cidr) == nil {
				return "", errors.Errorf("invalid trusted proxy entry %q: must be a valid CIDR or IP address", cidr)
			}
		}
	}

	cidrs := strings.Join(cfg.TrustedProxies, " ")
	return fmt.Sprintf("  servers {\n    trusted_proxies static %s\n  }", cidrs), nil
}

// BuildCaddyfileGlobalOptions builds the Caddyfile global options block.
// storageBlock is optional (empty string if not needed, e.g., non-GKE deployments).
// userPrefix is appended after the global block if provided.
func BuildCaddyfileGlobalOptions(storageBlock string, trustedProxiesBlock string, userPrefix string) string {
	var globalOpts []string

	if storageBlock != "" {
		globalOpts = append(globalOpts, storageBlock)
	}
	if trustedProxiesBlock != "" {
		globalOpts = append(globalOpts, trustedProxiesBlock)
	}

	var result string
	if len(globalOpts) > 0 {
		result = fmt.Sprintf("{\n%s\n}", strings.Join(globalOpts, "\n"))
	}

	if userPrefix != "" {
		if result != "" {
			return fmt.Sprintf("%s\n\n%s", result, userPrefix)
		}
		return userPrefix
	}

	return result
}

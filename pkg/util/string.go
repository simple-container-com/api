package util

import (
	"fmt"
	"regexp"
	"strings"
)

func TrimStringMiddle(str string, maxLen int, sep string) string {
	if len(str) > maxLen {
		return str[:maxLen/2] + sep + str[len(str)-maxLen/2:]
	}
	return str
}

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

func ToSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

func ToEnvVariableName(str string) string {
	return strings.ReplaceAll(strings.ToUpper(ToSnakeCase(str)), "-", "_")
}

// SanitizeGCPServiceAccountName sanitizes and truncates a name to comply with GCP service account naming requirements.
// GCP service account IDs must match: ^[a-z](?:[-a-z0-9]{4,28}[a-z0-9])$ and be at most 28 characters.
func SanitizeGCPServiceAccountName(name string) string {
	// Replace underscores with hyphens to comply with GCP naming requirements
	sanitizedName := strings.ReplaceAll(name, "_", "-")

	// Improved name truncation to avoid malformed names
	accountName := sanitizedName
	if len(sanitizedName) > 28 {
		// Use a 4-character hash suffix for uniqueness
		hash := fmt.Sprintf("%04x", len(sanitizedName)+int(sanitizedName[0])+int(sanitizedName[len(sanitizedName)-1]))
		// Calculate max prefix length to fit: prefix + "-" + hash <= 28
		maxPrefixLen := 28 - 1 - len(hash) // 28 - 1 (hyphen) - 4 (hash) = 23
		prefix := sanitizedName[:maxPrefixLen]
		// Remove trailing hyphens from prefix
		prefix = strings.TrimRight(prefix, "-")
		accountName = fmt.Sprintf("%s-%s", prefix, hash)
	}

	// Clean up any double hyphens that might have been created
	accountName = strings.ReplaceAll(accountName, "--", "-")

	return accountName
}

// SanitizeK8sResourceName sanitizes and truncates a name to comply with Kubernetes resource naming requirements.
// Kubernetes resource names must be at most 63 characters and match: ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
func SanitizeK8sResourceName(name string) string {
	// Replace underscores with hyphens to comply with Kubernetes RFC 1123
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), "_", "-")

	// Remove any invalid characters (keep only a-z, 0-9, -)
	reg := regexp.MustCompile(`[^a-z0-9\-]`)
	sanitizedName = reg.ReplaceAllString(sanitizedName, "")

	// Ensure it starts and ends with alphanumeric (trim leading/trailing hyphens)
	sanitizedName = strings.Trim(sanitizedName, "-")

	// Truncate if too long (63 character limit)
	if len(sanitizedName) > 63 {
		// Use a 4-character hash suffix for uniqueness
		hash := fmt.Sprintf("%04x", len(sanitizedName)+int(sanitizedName[0])+int(sanitizedName[len(sanitizedName)-1]))
		// Calculate max prefix length to fit: prefix + "-" + hash <= 63
		maxPrefixLen := 63 - 1 - len(hash) // 63 - 1 (hyphen) - 4 (hash) = 58
		prefix := sanitizedName[:maxPrefixLen]
		// Remove trailing hyphens from prefix
		prefix = strings.TrimRight(prefix, "-")
		sanitizedName = fmt.Sprintf("%s-%s", prefix, hash)
	}

	// Clean up any double hyphens that might have been created
	sanitizedName = strings.ReplaceAll(sanitizedName, "--", "-")

	return sanitizedName
}

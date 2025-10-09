package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecureFileReader provides obfuscated file reading for all file access paths
type SecureFileReader struct {
	// Future: Could include configuration, logging, etc.
}

// NewSecureFileReader creates a new secure file reader
func NewSecureFileReader() *SecureFileReader {
	return &SecureFileReader{}
}

// ReadFileSecurely reads a file and applies credential obfuscation if it contains secrets
func (sfr *SecureFileReader) ReadFileSecurely(filePath string) ([]byte, error) {
	// Read the file normally
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Apply obfuscation if this is a secrets-related file
	if sfr.isSecretsFile(filePath) {
		obfuscated := sfr.obfuscateCredentials(data, filePath)
		return obfuscated, nil
	}

	return data, nil
}

// isSecretsFile determines if a file likely contains secrets that should be obfuscated
func (sfr *SecureFileReader) isSecretsFile(filePath string) bool {
	fileName := strings.ToLower(filepath.Base(filePath))
	dirPath := strings.ToLower(filePath)

	// Explicit secrets files
	if fileName == "secrets.yaml" || fileName == "secrets.yml" {
		return true
	}

	// Files in .sc/stacks directories (likely Simple Container configs)
	if strings.Contains(dirPath, ".sc/stacks/") {
		return true
	}

	// Other potential secrets files
	secretsPatterns := []string{
		"secret", "credentials", "auth", "config", ".env",
	}

	for _, pattern := range secretsPatterns {
		if strings.Contains(fileName, pattern) {
			return true
		}
	}

	return false
}

// obfuscateCredentials applies comprehensive credential obfuscation
func (sfr *SecureFileReader) obfuscateCredentials(data []byte, filePath string) []byte {
	content := string(data)

	// Apply YAML-specific obfuscation for .yaml/.yml files
	if strings.HasSuffix(strings.ToLower(filePath), ".yaml") || strings.HasSuffix(strings.ToLower(filePath), ".yml") {
		if obfuscated := sfr.obfuscateYAMLCredentials(content); obfuscated != "" {
			return []byte(obfuscated)
		}
	}

	// Apply JSON-specific obfuscation for .json files
	if strings.HasSuffix(strings.ToLower(filePath), ".json") {
		if obfuscated := sfr.obfuscateJSONCredentials(content); obfuscated != "" {
			return []byte(obfuscated)
		}
	}

	// Fallback to general credential obfuscation
	return []byte(sfr.obfuscateGeneralCredentials(content))
}

// obfuscateYAMLCredentials handles YAML files with embedded credentials
func (sfr *SecureFileReader) obfuscateYAMLCredentials(content string) string {
	var yamlData interface{}
	if err := yaml.Unmarshal([]byte(content), &yamlData); err != nil {
		// If YAML parsing fails, use general obfuscation
		return sfr.obfuscateGeneralCredentials(content)
	}

	// Apply obfuscation to the YAML structure
	sfr.obfuscateYAMLValues(yamlData, "")

	// Marshal back to YAML
	if obfuscatedBytes, err := yaml.Marshal(yamlData); err == nil {
		return string(obfuscatedBytes)
	}

	// Fallback to general obfuscation
	return sfr.obfuscateGeneralCredentials(content)
}

// obfuscateYAMLValues recursively obfuscates sensitive values in YAML with context
func (sfr *SecureFileReader) obfuscateYAMLValues(data interface{}, sectionPath string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newSectionPath := key
			if sectionPath != "" {
				newSectionPath = sectionPath + "." + key
			}

			// Special handling for secrets.yaml 'values' section - obfuscate ALL values
			if sectionPath == "values" || key == "values" && sectionPath == "" {
				if strVal, ok := value.(string); ok {
					v[key] = sfr.obfuscateValue(strVal, key)
				} else {
					sfr.obfuscateAllStringValues(value)
				}
			} else if sfr.isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = sfr.obfuscateValue(strVal, key)
				} else {
					// Still recurse into nested structures under sensitive keys
					sfr.obfuscateYAMLValues(value, newSectionPath)
				}
			} else {
				sfr.obfuscateYAMLValues(value, newSectionPath)
			}
		}
	case map[interface{}]interface{}:
		for key, value := range v {
			keyStr := ""
			if k, ok := key.(string); ok {
				keyStr = k
			}

			newSectionPath := keyStr
			if sectionPath != "" {
				newSectionPath = sectionPath + "." + keyStr
			}

			// Special handling for secrets.yaml 'values' section
			if sectionPath == "values" || keyStr == "values" && sectionPath == "" {
				if strVal, ok := value.(string); ok {
					v[key] = sfr.obfuscateValue(strVal, keyStr)
				} else {
					sfr.obfuscateAllStringValues(value)
				}
			} else if keyStr != "" && sfr.isSensitiveKey(keyStr) {
				if strVal, ok := value.(string); ok {
					v[key] = sfr.obfuscateValue(strVal, keyStr)
				} else {
					// Still recurse into nested structures under sensitive keys
					sfr.obfuscateYAMLValues(value, newSectionPath)
				}
			} else {
				sfr.obfuscateYAMLValues(value, newSectionPath)
			}
		}
	case []interface{}:
		for _, item := range v {
			sfr.obfuscateYAMLValues(item, sectionPath)
		}
	}
}

// obfuscateAllStringValues obfuscates all string values in a data structure
func (sfr *SecureFileReader) obfuscateAllStringValues(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if strVal, ok := value.(string); ok {
				v[key] = sfr.obfuscateValue(strVal, key)
			} else {
				sfr.obfuscateAllStringValues(value)
			}
		}
	case map[interface{}]interface{}:
		for key, value := range v {
			keyStr := ""
			if k, ok := key.(string); ok {
				keyStr = k
			}
			if strVal, ok := value.(string); ok {
				v[key] = sfr.obfuscateValue(strVal, keyStr)
			} else {
				sfr.obfuscateAllStringValues(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			sfr.obfuscateAllStringValues(item)
		}
	}
}

// obfuscateJSONCredentials handles JSON files with credentials
func (sfr *SecureFileReader) obfuscateJSONCredentials(content string) string {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(content), &jsonData); err != nil {
		return sfr.obfuscateGeneralCredentials(content)
	}

	// Obfuscate sensitive fields in JSON
	sfr.obfuscateJSONValues(jsonData)

	// Marshal back to JSON
	if obfuscatedBytes, err := json.Marshal(jsonData); err == nil {
		return string(obfuscatedBytes)
	}

	return sfr.obfuscateGeneralCredentials(content)
}

// obfuscateJSONValues recursively obfuscates sensitive fields in JSON data
func (sfr *SecureFileReader) obfuscateJSONValues(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if sfr.isSensitiveKey(key) {
				if strVal, ok := value.(string); ok {
					v[key] = sfr.obfuscateValue(strVal, key)
				}
			} else {
				sfr.obfuscateJSONValues(value)
			}
		}
	case []interface{}:
		for _, item := range v {
			sfr.obfuscateJSONValues(item)
		}
	}
}

// isSensitiveKey checks if a key contains sensitive information
func (sfr *SecureFileReader) isSensitiveKey(key string) bool {
	key = strings.ToLower(key)

	sensitiveKeys := []string{
		"password", "passwd", "pwd",
		"secret", "secretkey", "secretaccesskey",
		"token", "apikey", "api_key", "accesskey", "access_key",
		"private_key", "privatekey", "private_key_id",
		"credentials", "auth", "authentication",
		"cert", "certificate", "key", "pem",
		"webhook", "webhookurl", "webhook_url",
		"dsn", "database_url", "connection_string", "connectionstring",
		"mongodb_uri", "mongo_uri", "redis_url", "postgres_url",
		"jwt_secret", "jwtsecret", "session_secret",
		// Kubernetes-specific fields
		"kubeconfig", "client-key", "client-key-data", "client-certificate-data",
		"certificate-authority-data", "client-cert", "client-cert-data",
		"user-key", "user-cert", "ca-cert", "ca-key",
		// GCP-specific fields
		"service_account_key", "client_secret", "refresh_token",
	}

	for _, sensitive := range sensitiveKeys {
		if strings.Contains(key, sensitive) {
			return true
		}
	}

	return false
}

// obfuscateValue masks a sensitive value while preserving its type/format context
func (sfr *SecureFileReader) obfuscateValue(value, key string) string {
	if value == "" {
		return value
	}

	// Preserve placeholder patterns (${secret:...}, ${env:...}, etc.)
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return value
	}

	// Check for embedded JSON credentials
	if strings.Contains(value, "\"private_key\"") || strings.Contains(value, "\"client_secret\"") {
		return sfr.obfuscateEmbeddedJSON(value)
	}

	// Check for embedded YAML credentials
	if strings.Contains(value, "apiVersion:") && strings.Contains(value, "clusters:") {
		return sfr.obfuscateEmbeddedYAML(value)
	}

	// Format-specific obfuscation
	switch {
	case strings.HasPrefix(value, "AKIA"):
		return "AKIA••••••••••••••••"
	case strings.HasPrefix(value, "sk-"):
		return "sk-•••••••••••••••••••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "ghp_"):
		return "ghp_••••••••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "pul-"):
		return "pul-••••••••••••••••••••••••••••••••••••••••"
	case strings.HasPrefix(value, "-----BEGIN"):
		return sfr.obfuscateMultilineSecret(value)
	case len(value) > 20 && sfr.isBase64Like(value):
		return value[:8] + "••••••••••••••••••••••••••••••••••••••••••••" + value[len(value)-4:]
	case len(value) > 10:
		if len(value) <= 20 {
			return "••••••••••••••••••••"
		}
		return value[:4] + "••••••••••••••••••••••••••••••••" + value[len(value)-2:]
	default:
		return "••••••••"
	}
}

// obfuscateEmbeddedJSON handles JSON structures embedded in credential values
func (sfr *SecureFileReader) obfuscateEmbeddedJSON(jsonStr string) string {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		return sfr.obfuscateGeneralCredentials(jsonStr)
	}

	sfr.obfuscateJSONValues(jsonData)

	if obfuscatedBytes, err := json.Marshal(jsonData); err == nil {
		return string(obfuscatedBytes)
	}

	return sfr.obfuscateGeneralCredentials(jsonStr)
}

// obfuscateEmbeddedYAML handles YAML structures embedded in credential values
func (sfr *SecureFileReader) obfuscateEmbeddedYAML(yamlStr string) string {
	var yamlData interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &yamlData); err != nil {
		return sfr.obfuscateYAMLStringPatterns(yamlStr)
	}

	sfr.obfuscateYAMLValues(yamlData, "")

	if obfuscatedBytes, err := yaml.Marshal(yamlData); err == nil {
		return string(obfuscatedBytes)
	}

	return sfr.obfuscateYAMLStringPatterns(yamlStr)
}

// obfuscateYAMLStringPatterns applies pattern-based obfuscation for YAML strings
func (sfr *SecureFileReader) obfuscateYAMLStringPatterns(yamlStr string) string {
	patterns := map[string]*regexp.Regexp{
		`(client-key-data|client-certificate-data|certificate-authority-data):\s*([A-Za-z0-9+/=]{20,})`: regexp.MustCompile(`(client-key-data|client-certificate-data|certificate-authority-data):\s*([A-Za-z0-9+/=]{20,})`),
		`(private_key|private-key):\s*"([^"]+)"`:                                                        regexp.MustCompile(`(private_key|private-key):\s*"([^"]+)"`),
		`(token):\s*([A-Za-z0-9._-]{20,})`:                                                              regexp.MustCompile(`(token):\s*([A-Za-z0-9._-]{20,})`),
	}

	result := yamlStr
	for _, pattern := range patterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			parts := strings.SplitN(match, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
					value = value[1 : len(value)-1]
					return key + `: "` + sfr.obfuscateValue(value, key) + `"`
				}
				return key + `: ` + sfr.obfuscateValue(value, key)
			}
			return match
		})
	}

	return result
}

// obfuscateMultilineSecret handles multi-line secrets like private keys
func (sfr *SecureFileReader) obfuscateMultilineSecret(secret string) string {
	lines := strings.Split(secret, "\n")
	if len(lines) < 3 {
		return "••••••••••••••••••••••••••••••••"
	}

	result := []string{lines[0]}
	for i := 1; i < len(lines)-1; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			result = append(result, "••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••")
		} else {
			result = append(result, lines[i])
		}
	}

	if len(lines) > 1 {
		result = append(result, lines[len(lines)-1])
	}

	return strings.Join(result, "\n")
}

// isBase64Like checks if a string looks like base64 encoding
func (sfr *SecureFileReader) isBase64Like(s string) bool {
	if len(s) < 20 {
		return false
	}

	base64Pattern := regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
	return base64Pattern.MatchString(s)
}

// obfuscateGeneralCredentials applies regex-based credential obfuscation
func (sfr *SecureFileReader) obfuscateGeneralCredentials(content string) string {
	patterns := map[*regexp.Regexp]string{
		regexp.MustCompile(`"private_key":\s*"([^"]+)"`):                           `"private_key": "••••••••••••••••••••••••••••••••"`,
		regexp.MustCompile(`"client_secret":\s*"([^"]+)"`):                         `"client_secret": "••••••••••••••••••••••••••••••••"`,
		regexp.MustCompile(`(password|secret|token|key):\s*([A-Za-z0-9+/=]{20,})`): `$1: ••••••••••••••••••••••••••••••••`,
	}

	result := content
	for pattern, replacement := range patterns {
		result = pattern.ReplaceAllString(result, replacement)
	}

	return result
}

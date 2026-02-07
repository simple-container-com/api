# Implementation Plan: Environment-Specific Secrets

## Overview

This document provides a detailed implementation plan for adding environment-specific secrets configuration to Simple Container parent stacks.

## Phases

### Phase 1: Data Structures and Schema

**Files to Modify:**
- `pkg/api/server.go`
- `pkg/api/secrets.go`

**Tasks:**

1. Add new types to `pkg/api/server.go`:
   ```go
   // EnvironmentSecretsConfig defines which secrets are available for a specific environment
   type EnvironmentSecretsConfig struct {
       Mode    string            `json:"mode" yaml:"mode"`
       Secrets map[string]string `json:"secrets" yaml:"secrets"`
   }

   // SecretsConfigMap contains per-environment secret configurations
   type SecretsConfigMap struct {
       InheritAll   bool                                `json:"inheritAll" yaml:"inheritAll"`
       Environments map[string]EnvironmentSecretsConfig `json:"environments" yaml:"environments"`
   }
   ```

2. Modify `SecretsConfigDescriptor` in `pkg/api/server.go`:
   ```go
   type SecretsConfigDescriptor struct {
       Type          string             `json:"type" yaml:"type"`
       Config        `json:",inline" yaml:",inline"`
       Inherit       `json:",inline" yaml:",inline"`
       SecretsConfig *SecretsConfigMap  `json:"secretsConfig,omitempty" yaml:"secretsConfig,omitempty"`
   }
   ```

3. Add secret resolution types to `pkg/api/secrets.go`:
   ```go
   // SecretResolutionContext holds context for resolving secret references
   type SecretResolutionContext struct {
       Environment         string
       StackName           string
       ParentStackName     string
       AvailableSecrets    map[string]string
   }

   // SecretResolver handles environment-aware secret resolution
   type SecretResolver struct {
       globalSecrets  *SecretsDescriptor
       secretsConfig  *SecretsConfigMap
   }
   ```

**Acceptance Criteria:**
- [ ] New types compile without errors
- [ ] Fields have proper JSON/YAML tags
- [ ] `SecretsConfig` field is optional (omitempty)
- [ ] Types are exported (capitalized) for JSON schema generation

**Estimated Effort:** 2-3 hours

---

### Phase 2: Configuration Reading and Detection

**Files to Modify:**
- `pkg/api/read.go`

**Tasks:**

1. Add validation function:
   ```go
   func ValidateSecretsConfig(config *SecretsConfigMap) error {
       if config == nil {
           return nil
       }

       // Validate each environment configuration
       for envName, envConfig := range config.Environments {
           // Validate mode
           if envConfig.Mode != "include" && envConfig.Mode != "exclude" && envConfig.Mode != "override" {
               return errors.Errorf("invalid mode %q for environment %q: must be 'include', 'exclude', or 'override'", envConfig.Mode, envName)
           }

           // Validate secrets map is not empty
           if len(envConfig.Secrets) == 0 {
               return errors.Errorf("secrets map cannot be empty for environment %q", envName)
           }
       }

       return nil
   }
   ```

2. Add detection function:
   ```go
   func DetectSecretsConfigType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
       if descriptor.Secrets.SecretsConfig == nil {
           return descriptor, nil // No secretsConfig, use legacy behavior
       }

       if err := ValidateSecretsConfig(descriptor.Secrets.SecretsConfig); err != nil {
           return nil, errors.Wrapf(err, "invalid secretsConfig in server descriptor")
       }

       return descriptor, nil
   }
   ```

3. Modify `ReadServerConfigs()` to call new detection:
   ```go
   func ReadServerConfigs(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
       // ... existing detection calls ...

       if withSecretsConfig, err := DetectSecretsConfigType(&res); err != nil {
           return nil, err
       } else {
           res = *withSecretsConfig
       }

       return &res, nil
   }
   ```

**Acceptance Criteria:**
- [ ] Invalid mode values are rejected with clear error messages
- [ ] Empty secrets maps are rejected
- [ ] Missing secretsConfig doesn't break existing stacks
- [ ] Configuration is validated early (at read time)

**Estimated Effort:** 3-4 hours

---

### Phase 3: Secret Resolution Logic

**Files to Modify:**
- `pkg/api/secrets.go` (add new file or extend existing)

**Tasks:**

1. Create `SecretResolver` with methods:

   ```go
   func NewSecretResolver(globalSecrets *SecretsDescriptor, config *SecretsConfigMap) *SecretResolver {
       return &SecretResolver{
           globalSecrets: globalSecrets,
           secretsConfig: config,
       }
   }

   func (r *SecretResolver) ResolveSecret(secretName string, environment string) (string, error) {
       // If no secretsConfig, use legacy behavior
       if r.secretsConfig == nil {
           if value, ok := r.globalSecrets.Values[secretName]; ok {
               return value, nil
           }
           return "", errors.Errorf("secret %q not found", secretName)
       }

       // Get environment config
       envConfig, hasEnv := r.secretsConfig.Environments[environment]
       if !hasEnv {
           // Environment not configured, apply inheritAll logic
           if r.secretsConfig.InheritAll {
               if value, ok := r.globalSecrets.Values[secretName]; ok {
                   return value, nil
               }
           }
           return "", errors.Errorf("environment %q is not configured in secretsConfig", environment)
       }

       // Resolve based on mode
       return r.resolveWithMode(secretName, environment, envConfig)
   }

   func (r *SecretResolver) resolveWithMode(secretName, environment string, envConfig EnvironmentSecretsConfig) (string, error) {
       switch envConfig.Mode {
       case "include":
           return r.resolveIncludeMode(secretName, envConfig)
       case "exclude":
           return r.resolveExcludeMode(secretName, envConfig)
       case "override":
           return r.resolveOverrideMode(secretName, envConfig)
       default:
           return "", errors.Errorf("invalid mode: %s", envConfig.Mode)
       }
   }

   func (r *SecretResolver) resolveIncludeMode(secretName string, envConfig EnvironmentSecretsConfig) (string, error) {
       // Check if secret is in include list
       reference, isIncluded := envConfig.Secrets[secretName]
       if !isIncluded {
           return "", errors.Errorf("secret %q is not available in environment (not in include list)", secretName)
       }

       // Resolve the reference
       return r.resolveReference(reference)
   }

   func (r *SecretResolver) resolveExcludeMode(secretName string, envConfig EnvironmentSecretsConfig) (string, error) {
       // Check if secret is excluded
       if _, isExcluded := envConfig.Secrets[secretName]; isExcluded {
           return "", errors.Errorf("secret %q is excluded in this environment", secretName)
       }

       // Return from global secrets
       if value, ok := r.globalSecrets.Values[secretName]; ok {
           return value, nil
       }
       return "", errors.Errorf("secret %q not found in global secrets", secretName)
   }

   func (r *SecretResolver) resolveOverrideMode(secretName string, envConfig EnvironmentSecretsConfig) (string, error) {
       // Check if secret is in override list
       value, isIncluded := envConfig.Secrets[secretName]
       if !isIncluded {
           return "", errors.Errorf("secret %q is not available in environment (not in override list)", secretName)
       }

       // Return literal value (no reference resolution)
       return value, nil
   }

   func (r *SecretResolver) resolveReference(reference string) (string, error) {
       // Pattern 1: Direct reference "~"
       if reference == "~" {
           return "", errors.New("direct reference (~) requires context to determine source key")
       }

       // Pattern 2: Mapped reference "${secret:KEY}"
       if strings.HasPrefix(reference, "${secret:") && strings.HasSuffix(reference, "}") {
           key := strings.TrimPrefix(reference, "${secret:")
           key = strings.TrimSuffix(key, "}")

           if value, ok := r.globalSecrets.Values[key]; ok {
               return value, nil
           }
           return "", errors.Errorf("referenced secret %q not found in global secrets", key)
       }

       // Pattern 3: Literal value (return as-is)
       return reference, nil
   }
   ```

2. Add helper for include mode with direct reference:
   ```go
   func (r *SecretResolver) resolveIncludeMode(secretName string, envConfig EnvironmentSecretsConfig) (string, error) {
       reference, isIncluded := envConfig.Secrets[secretName]
       if !isIncluded {
           return "", errors.Errorf("secret %q is not available in environment", secretName)
       }

       // Handle direct reference "~" - use same key
       if reference == "~" {
           if value, ok := r.globalSecrets.Values[secretName]; ok {
               return value, nil
           }
           return "", errors.Errorf("secret %q not found in global secrets", secretName)
       }

       return r.resolveReference(reference)
   }
   ```

**Acceptance Criteria:**
- [ ] Include mode: Only listed secrets are available
- [ ] Exclude mode: All secrets except listed are available
- [ ] Override mode: Only listed secrets with literal values
- [ ] Direct reference (~) resolves to same-named secret
- [ ] Mapped reference (${secret:KEY}) resolves to different secret
- [ ] Literal values returned as-is
- [ ] Clear error messages for all failure scenarios

**Estimated Effort:** 6-8 hours

---

### Phase 4: Stack Reconciliation Integration

**Files to Modify:**
- `pkg/api/models.go`

**Tasks:**

1. Modify `ReconcileForDeploy()` to create resolver and filter secrets:
   ```go
   func (m *StacksMap) ReconcileForDeploy(params StackParams) (*StacksMap, error) {
       current := *m
       iterMap := lo.Assign(current)
       for stackName, stack := range iterMap {
           if len(stack.Client.Stacks) == 0 {
               continue
           }
           clientDesc, ok := stack.Client.Stacks[params.Environment]
           if !ok && stackName != params.StackName {
               continue
           }
           if !ok {
               return nil, errors.Errorf("client stack %q is not configured for %q", stackName, params.Environment)
           }

           parentStackParts := strings.SplitN(clientDesc.ParentStack, "/", 3)
           parentStackName := parentStackParts[len(parentStackParts)-1]
           if parentStack, ok := current[parentStackName]; ok {
               stack.Server = parentStack.Server.Copy()

               // NEW: Create secret resolver for this environment
               resolver := NewSecretResolver(&parentStack.Secrets, parentStack.Server.Secrets.SecretsConfig)

               // NEW: Filter secrets based on environment configuration
               filteredSecrets, err := resolver.FilterSecretsForEnvironment(params.Environment)
               if err != nil {
                   return nil, errors.Wrapf(err, "failed to resolve secrets for environment %q", params.Environment)
               }
               stack.Secrets = *filteredSecrets
           } else {
               return nil, errors.Errorf("parent stack %q is not configured for %q in %q", clientDesc.ParentStack, stackName, params.Environment)
           }
           current[stackName] = stack
       }
       return &current, nil
   }
   ```

2. Add method to filter all secrets for an environment:
   ```go
   func (r *SecretResolver) FilterSecretsForEnvironment(environment string) (*SecretsDescriptor, error) {
       result := &SecretsDescriptor{
           SchemaVersion: r.globalSecrets.SchemaVersion,
           Auth:          make(map[string]AuthDescriptor),
           Values:        make(map[string]string),
       }

       // Copy auth (not affected by secretsConfig)
       for key, auth := range r.globalSecrets.Auth {
           result.Auth[key] = auth
       }

       // If no secretsConfig, return all secrets
       if r.secretsConfig == nil {
           for key, value := range r.globalSecrets.Values {
               result.Values[key] = value
           }
           return result, nil
       }

       // Get environment config
       envConfig, hasEnv := r.secretsConfig.Environments[environment]
       if !hasEnv {
           if r.secretsConfig.InheritAll {
               // Return all secrets
               for key, value := range r.globalSecrets.Values {
                   result.Values[key] = value
               }
               return result, nil
           }
           // Return empty secrets descriptor
           return result, nil
       }

       // Filter based on mode
       switch envConfig.Mode {
       case "include":
           return r.filterIncludeMode(envConfig, result)
       case "exclude":
           return r.filterExcludeMode(envConfig, result)
       case "override":
           return r.filterOverrideMode(envConfig, result)
       default:
           return nil, errors.Errorf("invalid mode: %s", envConfig.Mode)
       }
   }

   func (r *SecretResolver) filterIncludeMode(envConfig EnvironmentSecretsConfig, result *SecretsDescriptor) (*SecretsDescriptor, error) {
       for clientKey, reference := range envConfig.Secrets {
           var value string
           var err error

           if reference == "~" {
               // Direct reference
               var ok bool
               value, ok = r.globalSecrets.Values[clientKey]
               if !ok {
                   return nil, errors.Errorf("secret %q not found in global secrets", clientKey)
               }
           } else if strings.HasPrefix(reference, "${secret:") && strings.HasSuffix(reference, "}") {
               // Mapped reference
               key := strings.TrimPrefix(reference, "${secret:")
               key = strings.TrimSuffix(key, "}")
               var ok bool
               value, ok = r.globalSecrets.Values[key]
               if !ok {
                   return nil, errors.Errorf("referenced secret %q not found in global secrets", key)
               }
           } else {
               // Literal value
               value = reference
           }

           result.Values[clientKey] = value
       }
       return result, nil
   }

   func (r *SecretResolver) filterExcludeMode(envConfig EnvironmentSecretsConfig, result *SecretsDescriptor) (*SecretsDescriptor, error) {
       for key, value := range r.globalSecrets.Values {
           // Skip if excluded
           if _, excluded := envConfig.Secrets[key]; excluded {
               continue
           }
           result.Values[key] = value
       }
       return result, nil
   }

   func (r *SecretResolver) filterOverrideMode(envConfig EnvironmentSecretsConfig, result *SecretsDescriptor) (*SecretsDescriptor, error) {
       for key, value := range envConfig.Secrets {
           result.Values[key] = value
       }
       return result, nil
   }
   ```

**Acceptance Criteria:**
- [ ] Secret resolver created during reconciliation
- [ ] Secrets filtered based on environment configuration
- [ ] Auth configurations preserved (not affected)
- [ ] Error handling for invalid configurations
- [ ] Backwards compatible (works without secretsConfig)

**Estimated Effort:** 4-6 hours

---

### Phase 5: Validation

**Files to Create:**
- `pkg/api/validation.go` (new file)
- `pkg/api/validation_test.go` (new file)

**Tasks:**

1. Create validation functions:
   ```go
   package api

   import (
       "github.com/pkg/errors"
       "strings"
   )

   // ValidateSecretReference validates a secret reference pattern
   func ValidateSecretReference(reference string) error {
       if reference == "~" {
           return nil // Valid: direct reference
       }

       if strings.HasPrefix(reference, "${secret:") && strings.HasSuffix(reference, "}") {
           key := strings.TrimPrefix(reference, "${secret:")
           key = strings.TrimSuffix(key, "}")
           if key == "" {
               return errors.New("secret reference key cannot be empty")
           }
           return nil // Valid: mapped reference
       }

       // Anything else is treated as a literal value (valid)
       return nil
   }

   // ValidateSecretsConfig validates the entire secretsConfig structure
   func ValidateSecretsConfig(config *SecretsConfigMap, globalSecrets *SecretsDescriptor) error {
       if config == nil {
           return nil
       }

       // Validate each environment
       for envName, envConfig := range config.Environments {
           if err := ValidateEnvironmentConfig(envName, envConfig, globalSecrets); err != nil {
               return err
           }
       }

       return nil
   }

   // ValidateEnvironmentConfig validates a single environment configuration
   func ValidateEnvironmentConfig(envName string, envConfig EnvironmentSecretsConfig, globalSecrets *SecretsDescriptor) error {
       // Validate mode
       validModes := map[string]bool{"include": true, "exclude": true, "override": true}
       if !validModes[envConfig.Mode] {
           return errors.Errorf("invalid mode %q for environment %q: must be 'include', 'exclude', or 'override'", envConfig.Mode, envName)
       }

       // Validate secrets map is not empty
       if len(envConfig.Secrets) == 0 {
           return errors.Errorf("secrets map cannot be empty for environment %q", envName)
       }

       // Validate each secret reference
       for clientKey, reference := range envConfig.Secrets {
           if err := ValidateSecretReference(reference); err != nil {
               return errors.Wrapf(err, "invalid reference for secret %q in environment %q", clientKey, envName)
           }

           // For mapped references, validate the referenced secret exists
           if strings.HasPrefix(reference, "${secret:") && strings.HasSuffix(reference, "}") {
               referencedKey := strings.TrimPrefix(reference, "${secret:")
               referencedKey = strings.TrimSuffix(referencedKey, "}")

               if globalSecrets != nil {
                   if _, exists := globalSecrets.Values[referencedKey]; !exists {
                       return errors.Errorf("referenced secret %q does not exist in global secrets for secret %q in environment %q", referencedKey, clientKey, envName)
                   }
               }
           }

           // For direct references, validate the secret exists
           if reference == "~" {
               if globalSecrets != nil {
                   if _, exists := globalSecrets.Values[clientKey]; !exists {
                       return errors.Errorf("secret %q does not exist in global secrets for environment %q", clientKey, envName)
                   }
               }
           }
       }

       return nil
   }
   ```

2. Update `DetectSecretsConfigType()` to use new validation:
   ```go
   func DetectSecretsConfigType(descriptor *ServerDescriptor) (*ServerDescriptor, error) {
       if descriptor.Secrets.SecretsConfig == nil {
           return descriptor, nil
       }

       // Get global secrets from descriptor context (passed separately or loaded)
       // For now, validate structure without checking global secrets existence
       // Global secrets validation happens during stack reconciliation

       if err := ValidateSecretsConfig(descriptor.Secrets.SecretsConfig, nil); err != nil {
           return nil, errors.Wrapf(err, "invalid secretsConfig in server descriptor")
       }

       return descriptor, nil
   }
   ```

**Acceptance Criteria:**
- [ ] Invalid mode values are rejected
- [ ] Empty secrets maps are rejected
- [ ] Invalid secret reference patterns are rejected
- [ ] Referenced secrets are validated against global secrets
- [ ] Clear error messages for all validation failures

**Estimated Effort:** 4-5 hours

---

### Phase 6: JSON Schema Regeneration

**Files to Modify:**
- `cmd/schema-gen/main.go` (no changes, just run it)

**Tasks:**

1. Run schema generator:
   ```bash
   cd cmd/schema-gen
   go run main.go ../../docs/schemas
   ```

2. Verify generated schema:
   - Check `docs/schemas/core/serverdescriptor.json`
   - Verify `secretsConfig` property is present
   - Verify proper nested structure

3. Update documentation links if needed

**Acceptance Criteria:**
- [ ] Schema generated successfully
- [ ] `secretsConfig` property present with correct structure
- [ ] Mode enum includes: include, exclude, override
- [ ] Backwards compatible (secretsConfig is optional)

**Estimated Effort:** 1 hour

---

### Phase 7: Testing

**Files to Create:**
- `pkg/api/secrets_config_test.go` (new file)
- `pkg/api/secrets_resolver_test.go` (new file)
- `pkg/api/models_integration_test.go` (new file)

**Tasks:**

1. Unit tests for validation (`pkg/api/validation_test.go`):
   ```go
   func TestValidateSecretReference(t *testing.T) {
       tests := []struct {
           name      string
           reference string
           wantErr   bool
       }{
           {"direct reference", "~", false},
           {"mapped reference", "${secret:OTHER_KEY}", false},
           {"literal value", "actual-value", false},
           {"empty mapped key", "${secret:}", true},
       }
       // ... test implementation
   }

   func TestValidateEnvironmentConfig(t *testing.T) {
       // Test all modes
       // Test empty secrets map
       // Test invalid mode
       // Test missing referenced secrets
   }
   ```

2. Unit tests for secret resolver (`pkg/api/secrets_resolver_test.go`):
   ```go
   func TestSecretResolver_IncludeMode(t *testing.T) {
       // Test direct reference
       // Test mapped reference
       // Test literal value
       // Test unavailable secret
   }

   func TestSecretResolver_ExcludeMode(t *testing.T) {
       // Test excluded secret
       // Test available secret
   }

   func TestSecretResolver_OverrideMode(t *testing.T) {
       // Test override value
       // Test unavailable secret
   }

   func TestSecretResolver_NoSecretsConfig(t *testing.T) {
       // Test legacy behavior (all secrets available)
   }
   ```

3. Integration tests for stack reconciliation (`pkg/api/models_integration_test.go`):
   ```go
   func TestReconcileForDeploy_WithSecretsConfig(t *testing.T) {
       // Test full stack reconciliation with secretsConfig
       // Test environment filtering
       // Test error cases
   }
   ```

4. Backwards compatibility tests:
   ```go
   func TestBackwardsCompatibility_NoSecretsConfig(t *testing.T) {
       // Test existing stacks work unchanged
   }
   ```

**Acceptance Criteria:**
- [ ] All unit tests pass
- [ ] Integration tests pass
- [ ] Backwards compatibility verified
- [ ] Code coverage > 80% for new code

**Estimated Effort:** 6-8 hours

---

### Phase 8: Documentation

**Files to Create/Modify:**
- `docs/docs/advanced/environment-specific-secrets.md` (new file)
- Update existing documentation as needed

**Tasks:**

1. Create user-facing documentation:
   ```markdown
   # Environment-Specific Secrets

   ## Overview
   ...

   ## Configuration Examples

   ### Include Mode (Allow List)
   ...

   ### Exclude Mode (Block List)
   ...

   ### Override Mode
   ...

   ## Migration Guide
   ...
   ```

2. Add examples to `docs/docs/examples/`:
   - `environment-specific-secrets/production/server.yaml`
   - `environment-specific-secrets/production/secrets.yaml`
   - `environment-specific-secrets/production/client.yaml`

3. Update `docs/docs/reference/supported-resources.md` if needed

**Acceptance Criteria:**
- [ ] Clear documentation for all three modes
- [ ] Real-world examples provided
- [ ] Migration guide for existing stacks
- [ ] Troubleshooting section

**Estimated Effort:** 3-4 hours

---

## Total Estimated Effort

| Phase | Description | Effort |
|-------|-------------|--------|
| 1 | Data Structures and Schema | 2-3 hours |
| 2 | Configuration Reading and Detection | 3-4 hours |
| 3 | Secret Resolution Logic | 6-8 hours |
| 4 | Stack Reconciliation Integration | 4-6 hours |
| 5 | Validation | 4-5 hours |
| 6 | JSON Schema Regeneration | 1 hour |
| 7 | Testing | 6-8 hours |
| 8 | Documentation | 3-4 hours |
| **Total** | | **29-39 hours** |

---

## Implementation Order

### Recommended Sequence:
1. **Phase 1** → **Phase 2** → **Phase 5** (Foundation: types, reading, validation)
2. **Phase 3** → **Phase 4** (Core logic: resolution, reconciliation)
3. **Phase 6** → **Phase 7** (Schema and testing)
4. **Phase 8** (Documentation)

### Critical Path:
Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 7

### Can Be Done in Parallel:
- Phase 5 (validation) can be done alongside Phase 3
- Phase 8 (documentation) can start after Phase 4
- Phase 6 (schema) is quick and can be done anytime after Phase 1

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Breaking existing stacks | Make `secretsConfig` optional, extensive backwards compatibility testing |
| Performance regression | Lazy resolution, caching, benchmark before/after |
| Complex edge cases | Comprehensive test suite, validation at multiple stages |
| Schema generation issues | Verify generated schema matches expected structure |

---

## Rollback Plan

If issues arise:
1. The `secretsConfig` field is optional - simply removing it from configs reverts to legacy behavior
2. Git revert for code changes
3. No database migrations or external dependencies - clean rollback

---

## Success Criteria

- [ ] All acceptance criteria met
- [ ] All tests passing (unit, integration)
- [ ] Backwards compatibility verified
- [ ] Documentation complete
- [ ] JSON schema updated
- [ ] Code coverage > 80%
- [ ] No performance regression

# Task Breakdown: Environment-Specific Secrets in Parent Stacks

**Feature Request:** Environment-Specific Secrets in Parent Stacks
**Issue ID:** #60
**Date:** 2026-02-08

## Overview

This document breaks down the implementation of environment-specific secrets into manageable tasks. Tasks are organized by phase and include dependencies, complexity estimates, and acceptance criteria.

## Implementation Phases

### Phase 1: Schema and Data Model Changes

**Objective:** Extend the secret storage schema to support environment-specific values while maintaining backward compatibility.

#### Task 1.1: Design New Schema Version 2.0

**Description:** Design the new YAML schema structure for environment-specific secrets.

**Complexity:** Low

**Dependencies:** None

**Deliverables:**
- Schema definition document
- Example configuration files
- Migration guide from v1.0 to v2.0

**Acceptance Criteria:**
- [ ] Schema supports multiple environments
- [ ] Schema supports shared secrets across environments
- [ ] Schema includes default environment specification
- [ ] Schema is backward compatible with v1.0 structure
- [ ] Schema examples cover common use cases

**Implementation Notes:**
- Reference existing schema in `.sc/secrets.yaml`
- Follow existing YAML patterns in the codebase
- Ensure schema can be parsed with existing YAML unmarshaling logic

#### Task 1.2: Extend Data Models

**Description:** Update Go data structures in `pkg/api/secrets/` to support the new schema.

**Complexity:** Medium

**Dependencies:** Task 1.1

**Files to Modify:**
- `pkg/api/secrets/cryptor.go` (data structures)
- `pkg/api/models.go` (if secret models are defined there)

**Deliverables:**
- Updated Go structs for environment-specific secrets
- JSON/YAML tags for serialization
- Validation logic for environment names

**Acceptance Criteria:**
- [ ] `EnvironmentSecrets` struct supports multiple environments
- [ ] `SecretsDescriptor` struct includes environment field
- [ ] Backward compatibility with v1.0 secret files
- [ ] Unit tests for data model validation

**Implementation Notes:**
- Preserve existing `EncryptedSecretFiles` and related structures
- Add new structures alongside existing ones (don't break existing code)
- Use schema version field to determine which structure to use

#### Task 1.3: Implement Schema Detection and Parsing

**Description:** Implement logic to detect schema version and parse appropriately.

**Complexity:** Medium

**Dependencies:** Task 1.2

**Files to Modify:**
- `pkg/api/secrets/management.go` (unmarshal logic)

**Deliverables:**
- Schema version detection logic
- Separate parsing paths for v1.0 and v2.0
- Error handling for invalid schemas

**Acceptance Criteria:**
- [ ] v1.0 files are parsed correctly (backward compatibility)
- [ ] v2.0 files are parsed correctly
- [ ] Invalid schema versions produce clear error messages
- [ ] Unit tests for both schema versions

**Implementation Notes:**
- Check `schemaVersion` field first
- Default to v1.0 if no version specified (backward compatibility)
- Use type assertions or separate structs for different versions

### Phase 2: Environment Context Management

**Objective:** Implement mechanisms to specify and propagate environment context through the system.

#### Task 2.1: Add Environment Field to Stack Configuration

**Description:** Extend stack configuration models to include environment specification.

**Complexity:** Low

**Dependencies:** None

**Files to Modify:**
- `pkg/api/models.go` (Stack configuration structures)

**Deliverables:**
- `Environment` field added to stack configuration
- Validation logic for environment names
- Documentation for new field

**Acceptance Criteria:**
- [ ] Stack configuration includes optional `environment` field
- [ ] Environment names are validated (alphanumeric, hyphens, underscores)
- [ ] Environment field is serialized/deserialized correctly
- [ ] Unit tests for environment validation

**Implementation Notes:**
- Make environment field optional (pointer type)
- Follow existing field naming conventions
- Add JSON and YAML tags

#### Task 2.2: Implement CLI Environment Flag

**Description:** Add `--environment` flag to relevant CLI commands.

**Complexity:** Low

**Dependencies:** Task 2.1

**Files to Modify:**
- `cmd/sc/main.go`
- Related command files in `cmd/sc/`

**Deliverables:**
- `--environment` flag added to `apply`, `preview`, and other relevant commands
- Flag value is propagated to command context
- Help text for new flag

**Acceptance Criteria:**
- [ ] `--environment` flag accepts environment name
- [ ] Flag value is passed to stack processing logic
- [ ] Flag appears in command help text
- [ ] Error handling for invalid environment names

**Implementation Notes:**
- Use existing flag parsing patterns in the codebase
- Store environment value in command context or configuration object
- Support tab-completion if possible

#### Task 2.3: Implement Environment Variable Support

**Description:** Add support for `SC_ENVIRONMENT` environment variable.

**Complexity:** Low

**Dependencies:** Task 2.2

**Files to Modify:**
- `cmd/sc/main.go`
- Configuration loading logic

**Deliverables:**
- `SC_ENVIRONMENT` variable is read if CLI flag not provided
- Precedence logic: CLI flag > environment variable > default
- Documentation for environment variable usage

**Acceptance Criteria:**
- [ ] `SC_ENVIRONMENT` is read correctly
- [ ] CLI flag overrides environment variable
- [ ] Clear error messages when environment is missing
- [ ] Unit tests for environment resolution logic

**Implementation Notes:**
- Use `os.Getenv()` to read environment variable
- Implement precedence logic in configuration loading
- Provide clear error messages when environment cannot be determined

#### Task 2.4: Implement Environment Context Propagation

**Description:** Ensure environment context is propagated through placeholder resolution and stack processing.

**Complexity:** Medium

**Dependencies:** Task 2.1, Task 2.2, Task 2.3

**Files to Modify:**
- `pkg/provisioner/placeholders/placeholders.go`
- Stack processing logic in `pkg/provisioner/`

**Deliverables:**
- Environment context is available to placeholder resolution
- Context is propagated through parent/child stack inheritance
- Context is validated and defaults are applied

**Acceptance Criteria:**
- [ ] Environment context is available in placeholder resolver
- [ ] Child stacks inherit environment from parent unless overridden
- [ ] Default environment is used when no environment specified
- [ ] Integration tests for context propagation

**Implementation Notes:**
- Add environment field to placeholder context
- Update `Resolve()` method to handle environment context
- Ensure context is passed through stack inheritance chain

### Phase 3: Environment-Specific Secret Resolution

**Objective:** Implement environment-aware secret resolution in the placeholder system.

#### Task 3.1: Extend Secret Placeholder Syntax

**Description:** Add support for `${secret:name:environment}` syntax.

**Complexity:** Medium

**Dependencies:** Task 2.4

**Files to Modify:**
- `pkg/provisioner/placeholders/placeholders.go` (specifically `tplSecrets` function)

**Deliverables:**
- Parser for new placeholder syntax
- Support for explicit environment specification
- Backward compatibility with `${secret:name}` syntax

**Acceptance Criteria:**
- [ ] `${secret:name:environment}` syntax is parsed correctly
- [ ] `${secret:name}` syntax still works (backward compatibility)
- [ ] Explicit environment overrides context environment
- [ ] Unit tests for placeholder parsing

**Implementation Notes:**
- Extend existing placeholder parsing logic
- Split on `:` to detect explicit environment
- Pass environment parameter to secret lookup function
- Maintain existing behavior for old syntax

#### Task 3.2: Implement Environment-Aware Secret Lookup

**Description:** Update secret lookup logic to use environment context.

**Complexity:** High

**Dependencies:** Task 3.1, Task 1.3

**Files to Modify:**
- `pkg/provisioner/placeholders/placeholders.go` (specifically `tplSecrets` function)
- `pkg/api/secrets/management.go` (secret access logic)

**Deliverables:**
- Secret lookup respects environment context
- Shared secrets are accessible from any environment
- Environment-specific secrets take precedence over shared secrets
- Clear error messages for missing secrets

**Acceptance Criteria:**
- [ ] Secrets are resolved from correct environment
- [ ] Shared secrets are accessible from any environment
- [ ] Environment-specific secrets override shared secrets
- [ ] Missing secrets produce helpful error messages
- [ ] Parent stack secrets are resolved using child's environment
- [ ] Integration tests for various scenarios

**Implementation Notes:**
- Update `tplSecrets` function to accept environment parameter
- Implement secret lookup logic: environment-specific → shared → error
- Handle parent stack secret resolution with child's environment
- Maintain backward compatibility for v1.0 secret files

#### Task 3.3: Implement Secret Validation

**Description:** Add validation for secret references and environment access.

**Complexity:** Medium

**Dependencies:** Task 3.2

**Files to Modify:**
- `pkg/provisioner/placeholders/placeholders.go`
- Configuration validation logic

**Deliverables:**
- Secret reference validation at configuration load time
- Security warnings for inappropriate environment access
- Dry-run mode for secret resolution preview

**Acceptance Criteria:**
- [ ] Invalid secret references are caught at load time
- [ ] Missing secrets produce clear error messages with alternatives
- [ ] Security warnings for production secrets in development
- [ ] Dry-run mode shows secret resolution without applying changes
- [ ] Unit tests for validation scenarios

**Implementation Notes:**
- Add validation step before stack operations
- Check if secret exists in specified environment
- Display warning if environment seems inappropriate
- Implement dry-run flag for debugging

### Phase 4: Error Handling and User Experience

**Objective:** Provide clear error messages, warnings, and documentation for the new feature.

#### Task 4.1: Implement Error Messages

**Description:** Create clear, actionable error messages for all failure scenarios.

**Complexity:** Medium

**Dependencies:** Task 3.2, Task 3.3

**Files to Modify:**
- `pkg/provisioner/placeholders/placeholders.go`
- `pkg/api/secrets/management.go`

**Deliverables:**
- Error messages for missing secrets
- Error messages for invalid environments
- Error messages for schema issues
- Error messages for inheritance conflicts

**Acceptance Criteria:**
- [ ] Error messages include available alternatives
- [ ] Error messages suggest corrective actions
- [ ] Error messages are consistent in format
- [ ] Error messages don't expose secret values

**Implementation Notes:**
- Use structured error types
- Include context in error messages (environment, secret name, available options)
- Test error messages with various scenarios

#### Task 4.2: Implement Security Warnings

**Description:** Add warnings for potentially insecure secret usage patterns.

**Complexity:** Low

**Dependencies:** Task 4.1

**Files to Modify:**
- `pkg/provisioner/placeholders/placeholders.go`
- CLI output logic

**Deliverables:**
- Warning for production secrets in development environment
- Warning for missing environment specification
- Warning for ambiguous secret references

**Acceptance Criteria:**
- [ ] Security warnings are displayed for risky patterns
- [ ] Warnings can be suppressed if needed
- [ ] Warnings are clear and actionable
- [ ] Warnings don't break existing workflows

**Implementation Notes:**
- Add warning level to output system
- Check for common anti-patterns
- Allow warning suppression via flag for CI/CD environments

#### Task 4.3: Create User Documentation

**Description:** Write comprehensive documentation for the new feature.

**Complexity:** Medium

**Dependencies:** All previous tasks

**Files to Create:**
- `docs/features/environment-specific-secrets.md`
- `docs/guides/secrets-management.md` (update)
- `docs/migration-guide.md` (new)

**Deliverables:**
- Feature overview and use cases
- Configuration examples
- Migration guide from v1.0 to v2.0
- Troubleshooting guide
- API reference

**Acceptance Criteria:**
- [ ] Documentation covers all new features
- [ ] Examples are clear and copy-pasteable
- [ ] Migration guide is step-by-step
- [ ] Troubleshooting guide covers common issues
- [ ] Documentation is reviewed and approved

**Implementation Notes:**
- Follow existing documentation patterns
- Include real-world examples
- Provide before/after comparisons
- Add diagrams where helpful

### Phase 5: Testing and Quality Assurance

**Objective:** Ensure the implementation is thoroughly tested and meets quality standards.

#### Task 5.1: Write Unit Tests

**Description:** Create comprehensive unit tests for new functionality.

**Complexity:** High

**Dependencies:** All implementation tasks

**Files to Create:**
- `pkg/api/secrets/environment_test.go`
- `pkg/provisioner/placeholders/environment_test.go`

**Deliverables:**
- Unit tests for schema parsing (v1.0 and v2.0)
- Unit tests for environment context management
- Unit tests for secret resolution logic
- Unit tests for error handling

**Acceptance Criteria:**
- [ ] Unit test coverage > 80% for new code
- [ ] All edge cases are tested
- [ ] Tests cover both success and failure scenarios
- [ ] Tests run quickly (< 5 seconds total)

**Implementation Notes:**
- Use table-driven tests for multiple scenarios
- Mock external dependencies (git repo, file system)
- Test both happy path and error paths
- Include regression tests for existing functionality

#### Task 5.2: Write Integration Tests

**Description:** Create integration tests for end-to-end scenarios.

**Complexity:** High

**Dependencies:** Task 5.1

**Files to Create:**
- `pkg/api/secrets/testdata/environments/` (test configurations)
- Integration test files

**Deliverables:**
- Integration tests for parent/child stack inheritance
- Integration tests for CLI flag usage
- Integration tests for placeholder resolution
- Integration tests for schema migration

**Acceptance Criteria:**
- [ ] Integration tests cover real-world scenarios
- [ ] Tests use actual stack configurations
- [ ] Tests verify end-to-end functionality
- [ ] Tests can be run in CI/CD pipeline

**Implementation Notes:**
- Create realistic test configurations
- Test with both v1.0 and v2.0 schemas
- Include performance tests
- Test with multiple environments and inheritance levels

#### Task 5.3: Performance Testing

**Description:** Ensure performance requirements are met.

**Complexity:** Medium

**Dependencies:** Task 5.2

**Deliverables:**
- Performance benchmarks for secret resolution
- Performance comparison with v1.0 implementation
- Optimization if needed

**Acceptance Criteria:**
- [ ] Secret resolution < 10ms per placeholder
- [ ] Configuration parsing < 100ms for 50 environments
- [ ] No performance degradation for v1.0 files
- [ ] Benchmarks are documented

**Implementation Notes:**
- Use Go's benchmark testing framework
- Compare before/after performance
- Test with large numbers of environments and secrets
- Profile and optimize hot paths if needed

#### Task 5.4: Security Testing

**Description:** Verify security requirements are met.

**Complexity:** Medium

**Dependencies:** Task 5.2

**Deliverables:**
- Security test suite
- Penetration test results
- Security review report

**Acceptance Criteria:**
- [ ] Production secrets not accessible in development
- [ ] Secret values not exposed in error messages
- [ ] Environment context validation prevents bypass
- [ ] Audit logging captures environment access

**Implementation Notes:**
- Test for cross-environment secret access
- Verify error messages don't leak secrets
- Test with malicious input (environment names, secret names)
- Review audit log output

### Phase 6: Migration and Release

**Objective:** Provide tools and documentation for migrating existing configurations.

#### Task 6.1: Create Migration Tool

**Description:** Build optional tool to convert v1.0 secret files to v2.0 format.

**Complexity:** Medium

**Dependencies:** Task 1.3, Task 4.3

**Files to Create:**
- `cmd/sc/migrate-secrets.go` (or similar)

**Deliverables:**
- CLI command to migrate v1.0 to v2.0
- Interactive migration with user confirmation
- Backup of original files before migration

**Acceptance Criteria:**
- [ ] Migration tool converts v1.0 files to v2.0
- [ ] User confirms migration before changes are made
- [ ] Original files are backed up
- [ ] Migration can be rolled back
- [ ] Tool handles edge cases (large files, custom environments)

**Implementation Notes:**
- Prompt user for environment names during migration
- Create default environment structure
- Validate migration result
- Provide clear summary of changes

#### Task 6.2: Update Release Notes

**Description:** Prepare release notes for the new feature.

**Complexity:** Low

**Dependencies:** Task 4.3, Task 6.1

**Files to Create:**
- `CHANGELOG.md` entry
- Release announcement

**Deliverables:**
- Release notes highlighting new feature
- Migration instructions
- Breaking changes documentation
- Upgrade guide

**Acceptance Criteria:**
- [ ] Release notes are clear and comprehensive
- [ ] Migration instructions are step-by-step
- [ ] Breaking changes are clearly documented
- [ ] Examples are provided

**Implementation Notes:**
- Follow existing release note format
- Include upgrade path for existing users
- Highlight backwards compatibility
- Provide links to full documentation

#### Task 6.3: Final Quality Checks

**Description:** Perform final validation before release.

**Complexity:** Low

**Dependencies:** All previous tasks

**Deliverables:**
- Pre-release checklist
- Sign-off from stakeholders

**Acceptance Criteria:**
- [ ] All acceptance criteria from previous tasks are met
- [ ] Documentation is complete and reviewed
- [ ] Tests pass in CI/CD pipeline
- [ ] Performance benchmarks are met
- [ ] Security review is complete
- [ ] Migration tool is tested

**Implementation Notes:**
- Create comprehensive pre-release checklist
- Get sign-off from technical lead
- Verify all tasks are complete
- Document any known limitations

## Task Dependencies

### Critical Path
1. Task 1.1 → Task 1.2 → Task 1.3 → Task 3.2 → Task 5.1 → Task 6.3

### Parallel Opportunities
- Tasks 2.1, 2.2, 2.3 can be done in parallel after Task 1.1
- Tasks 4.1, 4.2, 4.3 can be done in parallel after implementation
- Tasks 5.1, 5.2, 5.3, 5.4 can be partially parallel

### Blocking Dependencies
- Task 3.2 is blocked by Task 1.3 and Task 2.4
- Task 5.2 is blocked by Task 5.1
- Task 6.1 is blocked by Task 4.3

## Complexity Estimates

| Phase | Total Complexity | Estimated Time |
|-------|-----------------|----------------|
| Phase 1: Schema and Data Model | Medium | 1-2 weeks |
| Phase 2: Environment Context | Low-Medium | 1 week |
| Phase 3: Secret Resolution | Medium-High | 2-3 weeks |
| Phase 4: Error Handling and UX | Medium | 1-2 weeks |
| Phase 5: Testing and QA | High | 2-3 weeks |
| Phase 6: Migration and Release | Low-Medium | 1 week |
| **Total** | **High** | **8-12 weeks** |

## Risk Mitigation Tasks

### High-Risk Areas
1. **Backward Compatibility** (Task 1.3, Task 5.1)
   - Mitigation: Extensive testing with existing configurations
   - Additional validation: Manual testing with real user configurations

2. **Performance Impact** (Task 5.3)
   - Mitigation: Early performance benchmarking
   - Additional validation: Continuous performance monitoring during development

3. **Security Misconfiguration** (Task 5.4)
   - Mitigation: Security review before release
   - Additional validation: Penetration testing by security team

## Success Criteria

### Phase Completion Criteria
Each phase is considered complete when:
- All tasks in the phase have acceptance criteria met
- All tests pass
- Code has been reviewed and approved
- Documentation is updated

### Overall Completion Criteria
The feature is considered complete when:
- All phases are complete
- Migration tool is tested and documented
- Release notes are prepared
- Stakeholder sign-off is obtained
- CI/CD pipeline passes all tests

## Notes

- This task breakdown is based on the requirements document
- Tasks may be adjusted as implementation progresses
- Regular review points should be scheduled to assess progress
- Dependencies should be re-evaluated at each review point
- Complexity estimates are approximate and may change based on actual implementation

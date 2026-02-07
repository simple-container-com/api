# Environment-Specific Secrets in Parent Stacks - Documentation Index

This directory contains the complete Product Manager requirements documentation for implementing environment-specific secrets in parent stacks (GitHub Issue #60).

## Feature Overview

Add environment-specific secret configuration to parent stack `server.yaml` files to control which secrets are available in which environments, with support for secret mapping, filtering, and literal values.

## Documentation Files

### [requirements.md](./requirements.md)
**Main Requirements Document**

Contains:
- Problem statement and current issues
- Proposed solution overview
- Functional and non-functional requirements
- Acceptance criteria with testable scenarios
- Configuration examples
- Implementation scope (in/out of scope)
- Dependencies, risks, and success metrics

**Who should read:** Product Managers, Technical Leads, Architects

---

### [technical-specification.md](./technical-specification.md)
**Technical Implementation Specification**

Contains:
- Data structures and type definitions
- Secret resolution algorithm
- Configuration reading changes
- Validation logic
- JSON schema updates
- Integration points with existing code
- Error handling
- Backwards compatibility strategy
- Testing requirements
- Performance and security considerations

**Who should read:** Architects, Backend Developers, DevOps Engineers

---

### [examples.md](./examples.md)
**Configuration Examples and Use Cases**

Contains:
- Basic environment isolation
- Secret mapping patterns
- Literal value configuration
- Exclusion mode
- Mixed configurations
- Real-world SaaS application example
- Multi-tenant platform example
- Gradual migration phases
- Common patterns reference
- Validation examples

**Who should read:** Developers, DevOps Engineers, Users

---

### [validation-and-migration.md](./validation-and-migration.md)
**Validation Requirements and Migration Guide**

Contains:
- Pre-deployment validation checks
- Validation commands and error messages
- 4-phase migration strategy
- Rollback procedures
- Testing strategy and checklist
- Common issues and solutions
- Best practices
- Troubleshooting guide

**Who should read:** DevOps Engineers, Site Reliability Engineers, Users

## Quick Start

1. **Read [requirements.md](./requirements.md)** to understand what we're building and why
2. **Review [examples.md](./examples.md)** to see how the feature works
3. **Study [technical-specification.md](./technical-specification.md)** if implementing the feature
4. **Follow [validation-and-migration.md](./validation-and-migration.md)** when adopting the feature

## Key Features

- ✅ **Environment-specific secret isolation** - Block production secrets from dev/staging
- ✅ **Secret mapping** - Use consistent secret names across environments with different values
- ✅ **Literal values** - Store non-sensitive configuration in server.yaml
- ✅ **Exclusion mode** - Start with all secrets, block specific ones
- ✅ **Inclusion mode** - Whitelist approach for maximum security
- ✅ **Backwards compatible** - Existing stacks work without modification
- ✅ **Validation** - Catch configuration errors before deployment

## Configuration Syntax

### Include Mode (Explicit Allow List)
```yaml
secretsConfig:
  inheritAll: false
  environments:
    staging:
      include:
        SLACK_WEBHOOK: ~                           # Same key from secrets.yaml
        DATABASE_URL: "${secret:STAGING_DB_URL}"   # Mapped reference
        LOG_LEVEL: "debug"                         # Literal value
```

### Exclude Mode (Block Specific Secrets)
```yaml
secretsConfig:
  inheritAll: true
  environments:
    staging:
      exclude:
        - PROD_API_KEY
      override:
        API_KEY: "${secret:TEST_API_KEY}"
```

## Background

**Issue:** #60 - Feature Request: Environment-Specific Secrets in Parent Stacks
**Labels:** feature
**State:** Open
**Repository:** simple-container-com/api

## Contributing

When implementing this feature:

1. Start with the [requirements.md](./requirements.md) to understand acceptance criteria
2. Follow the [technical-specification.md](./technical-specification.md) for implementation details
3. Use [examples.md](./examples.md) for test cases
4. Ensure all validation checks from [validation-and-migration.md](./validation-and-migration.md) are implemented

## Questions or Issues?

Refer to the main GitHub issue #60 or contact the Product Manager team.

# Simple Container GitHub Actions Implementation

This directory contains the implementation plan and documentation for 4 reusable GitHub Actions that simplify Simple Container usage in CI/CD pipelines.

## Overview

Instead of maintaining complex, hardcoded workflows for each project, these actions provide standardized, reusable components that handle all Simple Container operations without complexity.

## Self-Contained Actions Repository

These actions are completely self-contained Docker-based actions that embed ALL functionality:
- **Repository**: `https://github.com/simple-container-com/api`
- **Actions Location**: `.github/actions/` within the main repository
- **Usage Pattern**: `simple-container-com/api/.github/actions/<action-name>@v2025.10.4` (or `@main` for latest)
- **Zero External Dependencies**: No `actions/checkout`, no external tools, no composite dependencies
- **Complete Embedded Functionality**: All 467+ lines of workflow logic built into Docker images
- **Drop-in Replacement**: Single action call replaces entire complex workflows

## Actions Available

| Action                     | Purpose                    | Usage                                                                | Replaces Workflow             |
|----------------------------|----------------------------|----------------------------------------------------------------------|-------------------------------|
| **deploy-client-stack**    | Deploy application stacks  | `simple-container-com/api/.github/actions/deploy@v2025.10.4`         | build-and-deploy-service.yaml |
| **provision-parent-stack** | Provision infrastructure   | `simple-container-com/api/.github/actions/provision@v2025.10.4`      | provision.yaml                |
| **destroy-client-stack**   | Destroy application stacks | `simple-container-com/api/.github/actions/destroy@v2025.10.4`        | destroy-service.yaml          |
| **destroy-parent-stack**   | Destroy infrastructure     | `simple-container-com/api/.github/actions/destroy-parent@v2025.10.4` | *(new capability)*            |

**Key Features:**
- 🐳 **Docker-based**: Each action is a complete Docker container with all tools
- ⚡ **Zero Dependencies**: No external GitHub Actions required  
- 🔧 **All Tools Embedded**: Pre-built SC CLI, Git, Docker, Pulumi, notifications, etc.
- 📋 **Complete Functionality**: Version generation, secrets, notifications, cleanup
- 🏷️ **CalVer Versioning**: Use `@v2025.10.4` for production, `@main` for latest

## Benefits

### ✅ **Complexity Reduction**
- **Before**: 467+ line workflows with complex job dependencies
- **After**: Single action call with zero external dependencies  
- **Real Customer**: 117 lines → 15 lines (87% reduction, complete workflow replacement)

### ✅ **Standardization**
- Consistent behavior across all projects
- Centralized updates and bug fixes
- Professional error handling and notifications

### ✅ **Maintainability**
- Single source of truth for Simple Container operations
- Easy to update for new features or CLI changes
- Reduced duplication across repositories

### ✅ **User Experience**
- Simple, intuitive action interfaces
- Comprehensive documentation and examples
- Built-in best practices and optimizations

## Usage Examples

### Complete Production Deployment (Single Step!)

```yaml
name: Deploy Application
on: [push]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Deploy Stack  # ONLY STEP NEEDED - embeds all 467+ lines!
        uses: simple-container-com/api/.github/actions/deploy@main
        with:
          stack-name: "my-app"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
        # NO actions/checkout@v4 needed!
        # NO setup steps needed!
        # ALL functionality embedded!
```

### Complete Infrastructure Management

```yaml
jobs:
  provision:
    runs-on: ubuntu-latest
    steps:
      - name: Provision Infrastructure  # Complete self-contained operation
        uses: simple-container-com/api/.github/actions/provision@main
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
```

### Real Customer Migration Result

**Before**: 117 lines of complex workflow logic
**After**: 15 lines total (87% reduction)

```yaml
name: Deploy Production App  
on: [workflow_dispatch]

jobs:
  deploy:
    runs-on: blacksmith-8vcpu-ubuntu-2204
    environment: production
    steps:
      - uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: "everworker"
          environment: "production"  
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          validation-command: "curl -f https://api.mycompany.com/health"
```

## Implementation Files

### Core Documentation
- **[IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md)** - Comprehensive technical plan and architecture
- **[MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md)** - Guide for migrating from hardcoded workflows

### Action-Specific Documentation
- **[DEPLOY_CLIENT_ACTION.md](./DEPLOY_CLIENT_ACTION.md)** - Deploy client stack action specification
- **[PROVISION_PARENT_ACTION.md](./PROVISION_PARENT_ACTION.md)** - Provision parent stack action specification
- **[DESTROY_CLIENT_ACTION.md](./DESTROY_CLIENT_ACTION.md)** - Destroy client stack action specification
- **[DESTROY_PARENT_ACTION.md](./DESTROY_PARENT_ACTION.md)** - Destroy parent stack action specification

### Action Files
- **[actions/](./actions/)** - Actual GitHub Action files (action.yml) for each of the 4 actions

### Usage Examples
- **[UPDATED_USAGE_EXAMPLES.md](./UPDATED_USAGE_EXAMPLES.md)** - Complete real-world usage examples
- **[REAL_CUSTOMER_MIGRATION_EXAMPLE.md](./REAL_CUSTOMER_MIGRATION_EXAMPLE.md)** - Real production customer migration example  
- **[SELF_CONTAINED_USAGE_EXAMPLES.md](./SELF_CONTAINED_USAGE_EXAMPLES.md)** - Self-contained actions with zero external dependencies

### Implementation Design
- **[EMBEDDED_ACTION_DESIGN.md](./EMBEDDED_ACTION_DESIGN.md)** - Complete self-contained Docker-based action design
- **[GOLANG_ACTION_DESIGN.md](./GOLANG_ACTION_DESIGN.md)** - **RECOMMENDED**: Professional Golang implementation with type safety and enterprise architecture

## Architecture

### Common Components
All actions share these standardized components:

- **🔧 Simple Container CLI** - Pre-built SC binary embedded in action images
- **🔐 Secrets Management** - Secure handling of SC_CONFIG and related secrets  
- **📊 Progress Tracking** - Duration calculation and progress reporting
- **🔔 Notifications** - Slack/Discord integration with professional formatting
- **❌ Error Handling** - Graceful failure handling with proper cleanup
- **🏷️ Version Management** - CalVer versioning with automated tagging

### Security Features
- **Credential Protection**: Secure handling of Simple Container configurations
- **Access Control**: Build-in permission requirements
- **Audit Trail**: Comprehensive logging and notification system

### Performance Features
- **Parallel Operations**: Where applicable (e.g., multi-file generation)
- **Caching**: Optimal use of GitHub Actions caching
- **Resource Management**: Efficient runner utilization

## Development Status

- ✅ **Analysis Complete**: Existing workflow analysis finished
- 🔄 **Documentation In Progress**: Creating comprehensive action specifications
- ⏳ **Implementation Pending**: Action files will be created after documentation
- ⏳ **Testing Pending**: Integration testing with real Simple Container projects

## Getting Started

1. **Read the Implementation Plan**: Start with [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md) for technical overview
2. **Choose Your Action**: Review action-specific documentation for your use case
3. **Migration**: Follow [MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md) to migrate existing workflows
4. **Integration**: Use the provided examples to integrate actions into your workflows

## Support

For questions or issues related to these GitHub Actions:

1. Check action-specific documentation
2. Review the migration guide for common issues
3. Consult Simple Container documentation for CLI-specific questions
4. Submit issues with detailed workflow examples and error messages

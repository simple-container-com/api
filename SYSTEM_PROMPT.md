# Simple Container API - System Prompt

## Project Overview
This is the Simple Container API project with MkDocs documentation. The project provides infrastructure-as-code capabilities for deploying applications across multiple cloud providers including AWS, GCP, and others.

## Important Guidelines

### Documentation Requirements
- User expects SYSTEM_PROMPT.md to be read before working on the project
- User expects SYSTEM_PROMPT.md to be updated whenever new knowledge is gained
- This is a project-specific requirement that must be followed

### Examples and References
When working on documentation or configuration examples, consult real-world examples from these directories containing `.sc/stacks`.

**IMPORTANT: Refer to `REAL_WORLD_EXAMPLES_MAP.md` for comprehensive documentation of all available examples with specific file paths, line numbers, and configuration patterns.**

**NEW: Comprehensive Examples Directory**: `docs/docs/examples`
- **Static Websites**: Documentation sites, landing pages, admin dashboards, customer portals, media stores
- **ECS Deployments**: Backend services, vector databases, blockchain services, blog platforms, Meteor.js apps  
- **Lambda Functions**: AI gateways (Bedrock), storage services (cron jobs), billing systems, schedulers, cost analytics
- **GKE Autopilot**: Comprehensive GCP setups with all major services
- **Kubernetes Native**: Streaming platforms, high-resource environments, zero-downtime configs
- **Advanced Configs**: Mixed environments, high-resource (32GB/16CPU), AI integration, blockchain testnet
- **Parent Stacks**: AWS multi-region, GCP comprehensive, hybrid cloud configurations

All examples are anonymized production-tested configurations with detailed README files explaining usage patterns, parent stack requirements, and best practices.

#### Practical Examples (Use These First)
Use these production-tested, anonymized examples from `docs/examples/`:

**Static Websites:**
- `docs/docs/examples/static-websites/documentation-site/` - MkDocs documentation deployment
- `docs/docs/examples/static-websites/landing-page/` - Main website with SPA configuration
- `docs/docs/examples/static-websites/admin-dashboard/` - Admin UI with multi-environment setup
- `docs/docs/examples/static-websites/customer-portal/` - Customer-facing UI deployment

**ECS Deployments:**
- `docs/docs/examples/ecs-deployments/backend-service/` - Node.js backend with MongoDB and GraphQL
- `docs/docs/examples/ecs-deployments/vector-database/` - High-performance vector database with NLB
- `docs/docs/examples/ecs-deployments/blockchain-service/` - Blockchain integration with cross-service dependencies
- `docs/docs/examples/ecs-deployments/blog-platform/` - Multi-service deployment with reverse proxy
- `docs/docs/examples/ecs-deployments/meteor-app/` - Meteor.js application deployment

**Lambda Functions:**
- `docs/docs/examples/lambda-functions/ai-gateway/` - AWS Bedrock integration with specific IAM roles
- `docs/docs/examples/lambda-functions/storage-service/` - Scheduled cleanup with cron expressions
- `docs/docs/examples/lambda-functions/scheduler/` - High-frequency scheduling (every minute)
- `docs/docs/examples/lambda-functions/cost-analytics/` - AWS cost analysis with comprehensive IAM
- `docs/docs/examples/lambda-functions/billing-system/` - Multi-environment with YAML anchors

**GKE Autopilot:**
- `docs/docs/examples/gke-autopilot/comprehensive-setup/` - Complete GCP setup with all major services

**Advanced Configurations:**
- `docs/docs/examples/kubernetes-native/streaming-platform/` - Hardcoded IPs, N8N integration, zero-downtime
- `docs/docs/examples/advanced-configs/high-resource/` - 32GB/16CPU AI development environment
- `docs/docs/examples/parent-stacks/aws-multi-region/` - Multi-region AWS with extensive DNS

#### Reference Examples (For Research)
For comprehensive patterns research, refer to `REAL_WORLD_EXAMPLES_MAP.md` which catalogs 50+ production directories across 5 organizations with detailed configuration patterns and cross-reference tables.

### Resource Configuration Standards
- Always use actual Go struct properties from the codebase, not fictional ones
- Verify resource types and properties by examining the actual Go structs with yaml tags
- Follow established patterns: `ResourceType` for resources, `TemplateType` for templates
- Use proper YAML structure: `resources.resources.<env>` not `stacks`

### Template and Resource Patterns
- **GKE Autopilot**: 
  - Template type: `gcp-gke-autopilot`
  - Resource type: `gcp-gke-autopilot-cluster`
  - Template references resources via `gkeClusterResource` and `artifactRegistryResource` fields
- **Resource References**: Templates use string fields to reference resource names defined in per-environment resources blocks

### Documentation Formatting
- MkDocs requires blank lines before bullet point lists
- Avoid double pipes (||) in tables
- Convert Unicode symbols to standard markdown
- Ensure proper spacing in documentation sections

### Authentication and Configuration
- Use `"${auth:gcloud}"` for GCP credentials
- Use `"${auth:gcloud.projectId}"` for GCP project references
- Use `"${secret:SECRET_NAME}"` for sensitive values
- Use `"${env:ENV_VAR}"` for environment variables

## Project Structure
- `/docs/` - MkDocs documentation
- `/pkg/clouds/` - Cloud provider implementations
- `/pkg/api/` - Core API structures
- `/cmd/` - Command-line tools
- `/.sc/` - Simple Container configuration

## Recent Updates
- **MAJOR: Restructured Documentation** - Reorganized entire documentation structure for better user experience
  - Created logical user journey: Getting Started → Core Concepts → Guides → Examples → Reference → Advanced
  - Moved files from scattered `howto/`, `motivation/` directories into organized structure
  - Added comprehensive navigation with tabs and sections in mkdocs.yml
  - Created index pages for each major section with clear descriptions
- Fixed GKE Autopilot documentation with correct resource types and comprehensive examples
- Updated supported-resources.md with real-world configuration patterns
- Corrected resource type from `gke-autopilot` to `gcp-gke-autopilot-cluster`
- Added complete template usage examples based on production configurations

## Key Learnings
- Always verify actual struct definitions before documenting resource properties
- Use real-world examples from the aiwayz-sc-config project for accurate documentation
- Template and resource separation allows flexible deployment patterns across environments
- Resource references enable reusable templates with environment-specific configurations

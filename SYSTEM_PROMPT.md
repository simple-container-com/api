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
- **CRITICAL: ALWAYS use JSON schemas when checking supported properties for any resource**
  - JSON schemas in `docs/schemas/` are the authoritative source for all supported properties
  - Never guess or assume properties - always reference the generated JSON schema files
  - 37+ resources across 7 providers have complete JSON schema definitions
  - Example: Check `docs/schemas/aws/s3bucket.json` for S3Bucket properties
- Always use actual Go struct properties from the codebase, not fictional ones  
- Verify resource types and properties by examining JSON schemas first, then Go structs with yaml tags
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
- **MAJOR: Complete JSON Schema Ecosystem** - Implemented comprehensive JSON Schema generation for ALL Simple Container configurations
  - **EXPANDED**: Now generates schemas for both cloud resources AND core configuration files
  - **54 TOTAL SCHEMAS**: 37 cloud resources + 6 configuration file schemas + index files across 8 providers
  - **Configuration File Schemas**: Added schemas for `client.yaml` (ClientDescriptor, StackConfig types), `server.yaml` (ServerDescriptor), and project config (ConfigFile)
  - **Dynamic Discovery**: Uses dependency injection framework to auto-discover resources (no hard-coding)
  - **Self-Maintaining**: New resources automatically included when registered in any provider's `init()` function
  - **8 Providers**: AWS, GCP, Kubernetes, MongoDB, Cloudflare, FS, GitHub, Core (configuration files)
  - **Authoritative Source**: JSON schemas in `docs/schemas/` are the definitive reference for all supported properties
  - Added public API functions: `GetRegisteredProviderConfigs()`, `GetRegisteredProvisionerFieldConfigs()`, `GetRegisteredCloudHelpers()`
  - Integrated with Welder build process: `welder run generate-schemas` generates complete schema ecosystem
  - Updated `supported-resources.md` with JSON Schema references for validation and IDE support
- **MAJOR: Comprehensive Schema Validation Completed** - Systematically validated ALL documentation examples against JSON schemas
  - **FICTIONAL PROPERTIES ELIMINATED**: Fixed `minCapacity`/`maxCapacity` (should be `min`/`max`), removed fictional `scaling:` sections, eliminated `multiAZ`/`backupRetention`/`nodeType`/`numCacheNodes` properties
  - **FICTIONAL RESOURCE TYPES FIXED**: Corrected `aws-ecs-cluster`→`s3-bucket`, `aws-elasticache-redis`→`gcp-redis`, `gcp-bigquery`→`gcp-bucket`, `gcp-sql-postgres`→`gcp-cloudsql-postgres`, `aws-s3-bucket`→`s3-bucket`
  - **VALIDATION METHODOLOGY**: Used 54 JSON schemas across 8 providers as authoritative source, verified against AWS (12 resources) and GCP (14 resources) schema indexes
  - **RESULT**: All documentation examples now use only real Simple Container properties and resource types validated against actual Go struct schemas
- **MAJOR: Complete Compute Processor Validation** - Validated ALL compute processor environment variables against actual source code implementations
  - **SOURCE CODE VALIDATION**: Examined actual compute processor implementations in `/pkg/clouds/pulumi/` to determine exact environment variables
  - **FICTIONAL ENVIRONMENT VARIABLES ELIMINATED**: Removed fictional variables for GKE Autopilot (not implemented), GCP Bucket (not implemented), RabbitMQ (`RABBITMQ_VHOST`, corrected `RABBITMQ_URL`→`RABBITMQ_URI`), Redis (removed `REDIS_PASSWORD`, `REDIS_URL`, named variants)
  - **VALIDATED LEGITIMATE VARIABLES**: Confirmed AWS (RDS PostgreSQL/MySQL, S3), GCP (PostgreSQL Cloud SQL), Kubernetes (Helm Postgres, RabbitMQ, Redis), MongoDB Atlas environment variables against actual `AddEnvVariableIfNotExist` calls
  - **ENHANCED DOCUMENTATION**: Added comprehensive, source-code-validated environment variable documentation to `template-placeholders-advanced.md`
- **MAJOR: Complete YAML File Structure Validation** - Systematically validated all server.yaml, client.yaml, and secrets.yaml files across documentation
  - **SERVER.YAML VALIDATION**: Confirmed proper structure with `provisioner`, `templates`, `resources` sections and legitimate properties
  - **CLIENT.YAML VALIDATION**: Confirmed proper structure with `stacks` section, correct `uses`/`runs`/`dependencies` usage, and legitimate template placeholders
  - **SECRETS.YAML VALIDATION**: Confirmed proper structure with `auth` and `values` sections using exact literal values
  - **SEPARATION OF CONCERNS**: Validated that server.yaml contains infrastructure (DevOps), client.yaml contains stacks (Developer), secrets.yaml contains authentication
  - **TEMPLATE PLACEHOLDERS**: Validated correct usage of `${resource:name.prop}`, `${secret:name}`, `${dependency:name.resource.prop}`, `${auth:provider}` patterns
  - **RESULT**: All YAML files follow proper Simple Container patterns with 0 fictional properties found in actual configuration files
- **MAJOR: Restructured Documentation** - Reorganized entire documentation structure for better user experience
  - Created logical user journey: Getting Started → Core Concepts → Guides → Examples → Reference → Advanced
  - Moved files from scattered `howto/`, `motivation/` directories into organized structure
  - Added comprehensive navigation with tabs and sections in mkdocs.yml
  - Created index pages for each major section with clear descriptions
- Fixed GKE Autopilot documentation with correct resource types and comprehensive examples
- Updated supported-resources.md with real-world configuration patterns
- Corrected resource type from `gke-autopilot` to `gcp-gke-autopilot-cluster`
- Added complete template usage examples based on production configurations
- **COMPLETED: Fixed MkDocs list formatting issues in examples documentation**
  - Fixed deployment command formatting in 7 example index.md files
  - Changed plain text commands to bold format (e.g., "Deploy to staging:" → "**Deploy to staging:**")
  - Ensured proper MkDocs rendering for all deployment command sections
  - Verified individual README.md files are properly formatted
  - **EXTENDED: Fixed all **Features:** list formatting issues across examples documentation**
    - Fixed 20+ **Features:** sections missing blank lines before bullet points
    - Applied fixes to lambda-functions, gke-autopilot, kubernetes-native, ecs-deployments, advanced-configs, static-websites
    - Ensured all lists render properly in MkDocs instead of appearing as plain text
    - Verified **IAM Permissions Required:** and other sections are properly formatted
  - All examples documentation now follows complete MkDocs formatting standards
- **COMPLETED: Validated and eliminated fictional 'connectionString' property**
  - Discovered fictional `connectionString` property used in 4 documentation files
  - Validated against actual JSON schemas - property does not exist in any resource schemas
  - Fixed ecs-deployments/index.md, lambda-functions/index.md, advanced-configs/index.md, concepts/template-placeholders.md
  - Replaced fictional template placeholders with correct Simple Container pattern (auto-injection)
  - Verified MongoDB Atlas and Redis resources rely on compute processor auto-injection of environment variables
  - Final verification confirmed zero remaining instances of fictional `connectionString` property

## Cryptographic Capabilities
### Public Key Support
- **RSA 2048+**: Traditional RSA encryption using OAEP padding with SHA-256/SHA-512
- **ED25519**: Modern elliptic curve cryptography using HKDF-based approach
  - **Key Derivation**: HKDF-SHA256 derives encryption keys from ed25519 public key + random salt
  - **Symmetric Encryption**: ChaCha20-Poly1305 authenticated encryption
  - **Security**: Non-deterministic encryption with random salts for each operation
  - **Format**: SSH authorized key format for public keys, PKCS#8 for private keys

### Encryption Functions
- `EncryptLargeString()` - Auto-detects and supports both RSA and ed25519 public keys
- `DecryptLargeString()` - RSA decryption with chunked support  
- `DecryptLargeStringWithEd25519()` - Ed25519 HKDF-based decryption
- `GenerateKeyPair()` - RSA key pair generation
- `GenerateEd25519KeyPair()` - Ed25519 key pair generation

### Cryptor Integration
- `GenerateEd25519KeyPairWithProfile()` - Creates ed25519 keys with profile configuration
- `WithGeneratedEd25519Keys()` - Option for ed25519 key generation in cryptor
- Automatic key type detection in decryption process
- Full backward compatibility with existing RSA workflows

## Key Learnings
- Always verify actual struct definitions before documenting resource properties
- Use real-world examples from the aiwayz-sc-config project for accurate documentation
- Template and resource separation allows flexible deployment patterns across environments
- Resource references enable reusable templates with environment-specific configurations

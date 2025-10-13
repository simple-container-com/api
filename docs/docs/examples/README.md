# Real-World Examples

This directory contains production-tested Simple Container configurations based on real-world deployments. All company names, domains, and sensitive information have been anonymized while preserving the actual configuration patterns and best practices.

## Directory Structure

### Static Websites (`static-websites/`)
- **documentation-site**: MkDocs documentation deployment (based on Simple Container docs)
- **landing-page**: Main website with SPA configuration
- **admin-dashboard**: Admin UI deployment patterns
- **customer-portal**: Customer-facing UI deployment
- **media-store**: Media-specific static hosting

### ECS Deployments (`ecs-deployments/`)
- **backend-service**: Node.js backend with MongoDB integration
- **vector-database**: High-performance vector database with NLB
- **blockchain-service**: Blockchain integration with cross-service dependencies
- **blog-platform**: Multi-service deployment with reverse proxy
- **meteor-app**: Meteor.js application deployment

### Lambda Functions (`lambda-functions/`)
- **ai-gateway**: AWS Bedrock integration with specific IAM roles
- **storage-service**: Scheduled cleanup with cron expressions
- **billing-system**: Multi-environment Lambda with long timeouts
- **scheduler**: High-frequency scheduling (every minute)
- **cost-analytics**: AWS cost analysis with comprehensive IAM permissions

### GKE Autopilot (`gke-autopilot/`)
- **comprehensive-setup**: Complete GCP setup with all resources
- **template-based**: GKE template with resource references

### Kubernetes Native (`kubernetes-native/`)
- **streaming-platform**: Hardcoded infrastructure IPs with N8N integration
- **ai-development**: High-resource code execution environment
- **zero-downtime**: Advanced deployment configurations

### Advanced Configurations (`advanced-configs/`)
- **mixed-environments**: Different deployment types per environment
- **high-resource**: 32GB memory, 16 CPU configurations
- **ai-integration**: AI-powered development tools
- **blockchain-testnet**: Testnet integration patterns

### Parent Stacks (`parent-stacks/`)
- **aws-multi-region**: Multi-region AWS setup with comprehensive resources
- **gcp-comprehensive**: Complete GCP setup with all service types
- **hybrid-cloud**: Mixed cloud provider configurations

### CI/CD with GitHub Actions (`cicd-github-actions/`)
- **basic-setup**: Simple staging/production pipeline with automatic deployment
- **multi-stack**: Complex deployment managing multiple related stacks
- **preview-deployments**: PR-based preview environments with cleanup automation
- **advanced-notifications**: Multi-channel notifications with custom templates

### Kubernetes Affinity (`kubernetes-affinity/`)
- **multi-tier-node-isolation**: Real-world node pool isolation for multi-tier architecture
- **high-availability**: Zone anti-affinity and pod distribution patterns
- **performance-optimization**: Resource-specific scheduling and optimization

## Usage

Each example directory contains:
- `client.yaml` - Service deployment configuration
- `server.yaml` - Parent stack with resource definitions (when applicable)
- `README.md` - Specific documentation for the example

All examples use anonymized domains like `example.com`, `mycompany.com`, etc., and generic resource names that can be easily adapted to your use case.

## Key Patterns Demonstrated

- **Resource References**: `${resource:database.uri}`, `${secret:api-key}` patterns
- **Multi-Environment**: Staging/production configurations with YAML anchors
- **Security**: Cloudflare-only ingress, proper IAM roles
- **Scaling**: Auto-scaling configurations with various CPU thresholds
- **Advanced Features**: Response streaming, scheduled jobs, cross-service dependencies
- **Database Integration**: MongoDB, PostgreSQL, MySQL resource patterns
- **Email Services**: SMTP integration patterns
- **AI/ML Integration**: Bedrock, LLM proxy configurations
- **Blockchain**: Smart contract integration patterns
- **Kubernetes Affinity**: Node pool isolation, pod scheduling, performance optimization
- **Enterprise Scheduling**: Multi-tier architectures with workload separation

# Deployment Guides

This section provides step-by-step guides for deploying applications using Simple Container across different platforms and scenarios.

## Platform-Specific Guides

### Container Orchestration

- **[ECS Fargate](parent-ecs-fargate.md)** - Deploy containerized applications on AWS ECS Fargate
- **[GKE Autopilot](parent-gcp-gke-autopilot.md)** - Deploy on Google Kubernetes Engine Autopilot
- **[Pure Kubernetes](parent-pure-kubernetes.md)** - Deploy on any Kubernetes cluster

### Operational Guides

- **[CI/CD with GitHub Actions](cicd-github-actions.md)** - Complete guide to automated deployment pipelines with GitHub Actions
- **[DNS Management](dns-management.md)** - Complete guide to domain and DNS configuration with Cloudflare
- **[Secrets Management](secrets-management.md)** - Comprehensive guide to handling secrets and credentials
- **[Migration Guide](migration.md)** - Migrate existing applications to Simple Container
- **[Service Deployment Steps](service-steps-to-deploy.md)** - General deployment workflow

## Choosing the Right Platform

### AWS ECS Fargate
**Best for:** Teams already using AWS, serverless container deployments, auto-scaling workloads

- Fully managed container orchestration
- Pay-per-use pricing model
- Integrated with AWS services (RDS, S3, etc.)

### GKE Autopilot
**Best for:** Google Cloud users, teams wanting managed Kubernetes without complexity

- Fully managed Kubernetes experience
- Automatic node provisioning and scaling
- Built-in security and compliance features

### Pure Kubernetes
**Best for:** Multi-cloud deployments, existing Kubernetes expertise, maximum control

- Works with any Kubernetes cluster (EKS, AKS, on-premises)
- Full control over cluster configuration
- Portable across cloud providers

## Common Deployment Patterns

### Microservices Architecture

1. **Parent Stack Setup** - Configure shared infrastructure (databases, networking)
2. **Service Templates** - Define reusable service configurations
3. **Environment Management** - Deploy across dev/staging/prod environments
4. **Secret Management** - Secure handling of credentials and configuration

### Multi-Tenant Applications

1. **Resource Isolation** - Namespace-based tenant separation
2. **Shared Resources** - Common databases and services
3. **Scaling Strategies** - Per-tenant and shared resource scaling
4. **Monitoring & Logging** - Tenant-aware observability

## Next Steps

After completing a deployment guide:

1. Explore **[Examples](../examples/README.md)** for real-world configurations
2. Review **[Advanced Topics](../advanced/scaling-advantages.md)** for optimization strategies
3. Check **[Reference Documentation](../reference/supported-resources.md)** for complete API details

## Need Help?

- Review **[Core Concepts](../concepts/main-concepts.md)** for fundamental understanding
- Check **[Template Placeholders](../concepts/template-placeholders.md)** for configuration syntax
- Contact [support@simple-container.com](mailto:support@simple-container.com) for assistance

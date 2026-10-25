# Deployment Guides

This section provides step-by-step guides for deploying applications using Simple Container across different platforms and scenarios.

## Platform-Specific Guides

### Container Orchestration

- **[ECS Fargate](parent-ecs-fargate.md)** - Deploy containerized applications on AWS ECS Fargate
- **[GKE Autopilot](parent-gcp-gke-autopilot.md)** - Deploy on Google Kubernetes Engine Autopilot
- **[Pure Kubernetes](parent-pure-kubernetes.md)** - Deploy on any Kubernetes cluster

### Russian Clouds

- **[Yandex Cloud](parent-yandex-cloud.md)** - Serverless Containers, Container Registry, Lockbox secrets, and an API Gateway edge for a `.ru` domain
- **[Cloud.ru](parent-cloudru.md)** - Evolution Container Apps (serverless), machines SC creates itself, and single-node k3s on them

### Operational Guides

- **[Resource Adoption](resource-adoption.md)** - Import existing cloud infrastructure without downtime or modifications
- **[CI/CD with GitHub Actions](cicd-github-actions.md)** - Complete guide to automated deployment pipelines with GitHub Actions
- **[DNS Management](dns-management.md)** - Domain and DNS configuration across the Cloudflare, Yandex Cloud DNS and Cloud.ru Cloud DNS registrars
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

### Yandex Cloud / Cloud.ru
**Best for:** services that have to be hosted in Russia

- No managed Kubernetes is used or needed: serverless containers where they exist, a
  single machine or single-node k3s where they do not
- `vm` + `vm-compose` is the path for any cloud SC has no driver for — the machine can be
  one you created by hand (`provider: byo`)
- Read the cloud's guide before the first deploy: both have failure modes that present
  as success (a green Yandex deploy with zero revisions; a green Cloud.ru deploy on a
  port nothing opened)

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
2. Review **[Best Practices](../advanced/best-practices.md)** for production-readiness tips
3. Check **[Reference Documentation](../reference/supported-resources.md)** for complete API details

## Need Help?

- Review **[Core Concepts](../concepts/main-concepts.md)** for fundamental understanding
- Check **[Template Placeholders](../concepts/template-placeholders.md)** for configuration syntax
- Contact [support@simple-container.com](mailto:support@simple-container.com) for assistance
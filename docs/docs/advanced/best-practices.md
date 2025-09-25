# Best Practices

This guide covers best practices for using Simple Container effectively in production environments.

## Configuration Management

### Template Organization

- **Use descriptive template names** that clearly indicate their purpose
- **Group related templates** in logical sections within your configuration
- **Maintain consistent naming conventions** across environments

```yaml
templates:
  # Good: Clear, descriptive names
  api-backend-service:
    type: aws-ecs-fargate
  user-database:
    type: aws-rds-postgres
  
  # Avoid: Generic or unclear names
  service1:
    type: aws-ecs-fargate
```

### Environment Management

- **Use environment-specific resource naming** to avoid conflicts
- **Leverage environment variables** for configuration that changes between environments
- **Implement proper secret management** for sensitive data

```yaml
resources:
  resources:
    prod:
      api-db:
        name: "myapp-prod-database"
        instanceClass: "db.r5.large"
    staging:
      api-db:
        name: "myapp-staging-database" 
        instanceClass: "db.t3.medium"
```

## Security Best Practices

### Secret Management

- **Never commit secrets to version control**
- **Use Simple Container's secret management** for all sensitive data
- **Always add secrets.yaml to SC managed secrets** using `sc secrets add .sc/stacks/<parent>/secrets.yaml`
- **secrets.yaml contains ONLY exact literal values** - NO placeholders or variables are processed
- **Never use environment variable placeholders** in secrets.yaml (e.g., `${env:AWS_ACCESS_KEY}` will NOT work)
- **Rotate secrets regularly** and update configurations accordingly
- **Use least-privilege access** for all cloud resources

#### Proper secrets.yaml Workflow

1. **Create secrets.yaml with exact values:**
   ```yaml
   # .sc/stacks/devops/secrets.yaml
   schemaVersion: 1.0
   auth:
     aws-account:
       type: aws-token
       config:
         account: "123456789012"
         accessKey: "AKIAIOSFODNN7EXAMPLE"  # Exact literal value
         secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
         region: us-east-1
   values:
     DATABASE_PASSWORD: "mySecurePassword123"
     API_KEY: "sk-1234567890abcdef"
   ```

2. **Add to Simple Container's managed secrets:**
   ```bash
   sc secrets add .sc/stacks/devops/secrets.yaml
   ```

3. **Never commit the raw secrets.yaml file to version control** - only the encrypted version should be committed

### Network Security

- **Configure security groups** to allow only necessary traffic
- **Use private subnets** for databases and internal services
- **Implement proper CORS policies** for web applications
- **Enable HTTPS/TLS** for all public-facing services

## Scaling and Performance

### Resource Allocation

- **Start with conservative resource allocations** and scale up based on monitoring
- **Use auto-scaling policies** for variable workloads
- **Monitor resource utilization** and adjust configurations accordingly

### Database Optimization

- **Use read replicas** for read-heavy workloads
- **Implement connection pooling** for database connections
- **Configure appropriate backup retention** policies
- **Monitor database performance** metrics

## Deployment Strategies

### Blue-Green Deployments

- **Use separate environments** for blue-green deployments
- **Implement health checks** to verify deployment success
- **Plan rollback strategies** for failed deployments

### Gradual Rollouts

- **Deploy to staging first** before production
- **Use feature flags** for gradual feature rollouts
- **Monitor application metrics** during deployments

## Monitoring and Observability

### Logging

- **Implement structured logging** across all services
- **Use consistent log formats** for easier parsing
- **Set up log aggregation** for centralized monitoring
- **Configure appropriate log retention** policies

### Metrics and Alerting

- **Define key performance indicators** (KPIs) for your applications
- **Set up proactive alerting** for critical issues
- **Monitor both infrastructure and application metrics**
- **Create dashboards** for operational visibility

## Cost Optimization

### Resource Right-Sizing

- **Regularly review resource utilization** and adjust instance sizes
- **Use spot instances** where appropriate for cost savings
- **Implement auto-scaling** to avoid over-provisioning
- **Clean up unused resources** regularly

### Storage Optimization

- **Use appropriate storage classes** for different data access patterns
- **Implement lifecycle policies** for data archival
- **Monitor storage costs** and optimize accordingly

## Development Workflow

### Version Control

- **Use semantic versioning** for your configurations
- **Maintain separate branches** for different environments
- **Implement code review processes** for configuration changes
- **Tag releases** for easy rollback capabilities

### Testing

- **Test configurations** in staging environments before production
- **Implement automated testing** for critical deployment paths
- **Validate resource configurations** before deployment
- **Use infrastructure testing tools** where appropriate

## Troubleshooting

### Common Issues

- **Resource naming conflicts** - Use environment-specific prefixes
- **Permission issues** - Verify IAM roles and policies
- **Network connectivity** - Check security groups and routing
- **Secret access** - Ensure proper secret configuration

### Debugging Tools

- **Use Simple Container's status commands** to check deployment state
- **Enable verbose logging** for detailed troubleshooting
- **Check cloud provider logs** for infrastructure issues
- **Monitor application logs** for service-specific problems

## Team Collaboration

### Documentation

- **Document your architecture decisions** and configuration choices
- **Maintain up-to-date deployment guides** for your team
- **Create runbooks** for common operational tasks
- **Share knowledge** through internal documentation

### Access Management

- **Implement role-based access control** for different team members
- **Use separate credentials** for different environments
- **Regularly audit access permissions** and remove unused accounts
- **Implement approval processes** for production changes

## Compliance and Governance

### Audit Trail

- **Maintain logs** of all configuration changes
- **Implement approval workflows** for sensitive changes
- **Use version control** for all configuration files
- **Regular compliance reviews** of your infrastructure

### Data Protection

- **Implement data encryption** at rest and in transit
- **Configure proper backup strategies** for critical data
- **Ensure compliance** with relevant regulations (GDPR, HIPAA, etc.)
- **Regular security assessments** of your infrastructure

## Performance Optimization

### Application Performance

- **Use CDNs** for static content delivery
- **Implement caching strategies** at multiple layers
- **Optimize database queries** and indexing
- **Monitor application response times** and optimize bottlenecks

### Infrastructure Performance

- **Choose appropriate instance types** for your workloads
- **Use placement groups** for high-performance computing
- **Optimize network configurations** for low latency
- **Monitor infrastructure metrics** and adjust accordingly

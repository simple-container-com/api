# CI/CD with GitHub Actions Examples

This directory contains practical examples for setting up continuous integration and deployment (CI/CD) pipelines using Simple Container's GitHub Actions integration.

## Examples Overview

### [Basic Setup](basic-setup/)
A simple staging/production pipeline setup with automatic deployment to staging and manual approval for production.

**Features:**
- Automatic staging deployment on main branch push
- Manual production deployment with approval
- Slack/Discord notifications
- Basic secret management

**Best for:** Small teams, simple applications, getting started with CI/CD

### [Multi-Stack Deployment](multi-stack/)
Complex deployment pipeline managing multiple related stacks (infrastructure, databases, applications).

**Features:**
- Infrastructure-first deployment order
- Dependency management between stacks
- Cross-stack resource sharing
- Environment-specific configurations

**Best for:** Microservices architecture, complex applications with multiple components

### [Preview Deployments](preview-deployments/)
PR-based preview environments for testing changes before merging to main.

**Features:**
- Automatic preview deployment on PR creation
- Preview environment cleanup on PR close
- Temporary domain assignment
- Resource cleanup automation

**Best for:** Teams that want to test changes in isolation, QA processes

### [Advanced Notifications](advanced-notifications/)
Comprehensive notification setup with multiple channels and custom messaging.

**Features:**
- Multi-channel notifications (Slack, Discord, Telegram)
- Custom notification templates
- Status-specific messaging
- Team mentions and escalation

**Best for:** Large teams, production environments, compliance requirements

## Quick Start

1. **Choose an example** that matches your needs
2. **Copy the configuration** to your project
3. **Update the parameters** (organization name, stack names, etc.)
4. **Configure GitHub secrets** as specified in each example
5. **Generate workflows** using `sc cicd generate`

## Common Configuration

All examples use similar base configuration patterns:

### Server Configuration (`server.yaml`)
```yaml
schemaVersion: 1.0
cicd:
  type: github-actions
  config:
    organization: "your-org"
    environments:
      staging: { type: staging, auto-deploy: true }
      production: { type: production, protection: true }
    notifications:
      slack:
        webhook-url: "${secret:slack-webhook-url}"
        enabled: true
```

### Secrets Configuration (`secrets.yaml`)
```yaml
schemaVersion: 1.0
auth:
  aws:
    type: aws-token
    config:
      accessKey: "${secret:aws-access-key}"
      secretAccessKey: "${secret:aws-secret-key}"

values:
  aws-access-key: your-aws-access-key-here
  aws-secret-key: your-aws-secret-key-here
  slack-webhook-url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
  discord-webhook-url: "https://discord.com/api/webhooks/YOUR/WEBHOOK/URL"
  telegram-bot-token: your-telegram-bot-token-here
  telegram-chat-id: your-telegram-chat-id-here
```

### GitHub Secrets Setup

**Only ONE GitHub secret required:**
- `SC_CONFIG` - Simple Container configuration with SSH key pair to decrypt repository secrets

**All notification webhooks are configured in your secrets.yaml file and managed by Simple Container's secrets system.**

## Usage Patterns

### Generate Workflows
```bash
# Generate workflows from your configuration
sc cicd generate --stack myorg/infrastructure --output .github/workflows/
```

### Validate Configuration
```bash
# Validate CI/CD setup
sc cicd validate --stack myorg/infrastructure --show-diff
```

### Preview Changes
```bash
# Preview generated workflows
sc cicd preview --stack myorg/infrastructure --show-content
```

## Best Practices

1. **Start with Basic Setup** - Begin with the simple example and add complexity as needed
2. **Environment Protection** - Always configure production environment protection in GitHub
3. **Secret Management** - Use environment-specific secrets and rotate regularly
4. **Testing Strategy** - Use preview deployments for testing changes
5. **Monitoring** - Set up notifications and health checks for deployments

## Integration with Simple Container Features

### Parent Stacks
All examples work with Simple Container's parent stack pattern:
```yaml
# In client.yaml
parent: myorg/infrastructure
parentEnv: staging
```

### Resource Management
Examples show how to manage shared resources:
```yaml
# In server.yaml
resources:
  database:
    type: aws-rds-postgres
    config:
      instance-class: db.t3.micro
```

### Secret Integration
Examples demonstrate proper secret handling:
```yaml
# In client.yaml
config:
  secrets:
    DATABASE_URL: ${resource:database.uri}
    API_KEY: ${secret:api-key}
```

## Troubleshooting

### Common Issues

**Workflow not triggering:**
- Check branch protection rules
- Verify workflow file syntax
- Ensure proper event triggers configured

**Authentication errors:**
- Verify GitHub secrets are properly set
- Check cloud provider credential validity
- Confirm Simple Container configuration

**Deployment failures:**
- Review workflow logs in GitHub Actions
- Validate server.yaml configuration locally
- Check resource availability and quotas

### Getting Help

1. Review the **[CI/CD Guide](../../guides/cicd-github-actions.md)** for comprehensive documentation
2. Check **[Troubleshooting section](../../guides/cicd-github-actions.md#troubleshooting)** in the main guide
3. Examine workflow logs in GitHub Actions tab
4. Test configuration locally with `sc cicd validate --stack <stack-name>`

## Contributing

To add a new example:

1. Create a new directory with a descriptive name
2. Include complete server.yaml and secrets.yaml examples
3. Add a README.md explaining the use case and setup
4. Update this main README.md to list the new example

## Next Steps

After setting up CI/CD:
- Explore **[Advanced Deployment Patterns](../../advanced/deployment-patterns.md)**
- Review **[Secrets Management](../../guides/secrets-management.md)**
- Set up **[DNS Management](../../guides/dns-management.md)** for custom domains

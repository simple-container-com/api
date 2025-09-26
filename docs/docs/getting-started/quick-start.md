# Quick Start Guide

This guide will help you deploy your first application with Simple Container in under 15 minutes.

## Prerequisites

- Simple Container CLI installed ([Installation Guide](installation.md))
- Access to a cloud provider (AWS, GCP, or Kubernetes cluster)
- Basic familiarity with YAML configuration

## Step 1: Initialize Your Project

Create a new directory for your project and initialize Simple Container:

```bash
mkdir my-first-app
cd my-first-app
sc init
```

This creates a `.sc/` directory with basic configuration files.

## Step 2: Choose Your Deployment Type

Simple Container supports several deployment patterns. For this quick start, we'll deploy a simple static website.

Create the parent stack configuration file `.sc/stacks/infrastructure/server.yaml`:

```yaml
# .sc/stacks/infrastructure/server.yaml
schemaVersion: 1.0
templates:
  static-site:
    type: aws-static-website
    config:
      domain: "${secret:DOMAIN_NAME}"

resources:
  resources:
    prod:
      main-bucket:
        type: s3-bucket
        config:
          name: "my-app-${env:ENVIRONMENT}-bucket"
          allowOnlyHttps: true
```

Create the client stack configuration file `.sc/stacks/myapp/client.yaml`:

```yaml
# .sc/stacks/myapp/client.yaml
schemaVersion: 1.0
stacks:
  prod:
    type: static
    parent: infrastructure
    template: static-site
    config:
      bundleDir: "./dist"
      bucketName: "my-app-prod-bucket"
      location: "us-east-1"
      domain: "myapp.com"
      baseDnsZone: "myapp.com"
      indexDocument: "index.html"
      errorDocument: "index.html"
      provisionWwwDomain: true
```

## Step 3: Set Up Secrets

Create and configure your secrets.yaml file with exact values:

```bash
# Create the secrets file
mkdir -p .sc/stacks/prod
```

Create `.sc/stacks/prod/secrets.yaml` with your actual credentials:

```yaml
# .sc/stacks/prod/secrets.yaml
schemaVersion: 1.0
auth:
  aws:
    type: aws-token
    config:
      account: "123456789012"
      accessKey: "AKIAIOSFODNN7EXAMPLE"
      secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
      region: us-east-1

values:
  DOMAIN_NAME: "myapp.example.com"
```

Add the secrets file to Simple Container's managed secrets:

```bash
sc secrets add .sc/stacks/prod/secrets.yaml
```

## Step 4: Deploy

Deploy your application:

```bash
sc deploy -s my-first-app -e prod
```

Simple Container will:

1. Create the S3 bucket
2. Set up CloudFront distribution
3. Configure DNS (if using Route53)
4. Deploy your static files

## Step 5: Verify Deployment

Your website should now be live at your configured domain!

You can verify the deployment by:
- Visiting your domain in a web browser
- Checking your AWS S3 bucket for the deployed files
- Verifying CloudFront distribution is active in AWS Console

## Next Steps

Now that you have your first deployment running, explore:

- **[Core Concepts](../concepts/main-concepts.md)** - Understand templates, resources, and environments
- **[Deployment Guides](../guides/index.md)** - Learn about ECS, GKE, and Kubernetes deployments
- **[Examples](../examples/README.md)** - See real-world configuration examples
- **[Secrets Management](../guides/secrets-management.md)** - Advanced secret handling

## Common Issues

**Domain not resolving?**

- DNS propagation can take up to 24 hours
- Check your DNS settings with your domain provider
- Verify the domain is correctly configured in your secrets.yaml file

**Deployment failed?**

- Verify your secrets.yaml file contains correct AWS credentials
- Check that `sc secrets add .sc/stacks/prod/secrets.yaml` was run successfully
- Ensure your domain is properly configured

**Need help?**

- Join our community or contact [support@simple-container.com](mailto:support@simple-container.com)

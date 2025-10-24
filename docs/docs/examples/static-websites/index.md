# Static Website Examples

This section contains examples of deploying static websites using Simple Container.

## Available Examples

### Documentation Site
Deploy a MkDocs documentation site with Simple Container.

**Use Case:** Technical documentation, API docs, project documentation

**Configuration:**
```yaml
# .sc/stacks/docs-site/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: static
    parent: myorg/infrastructure
    config:
      bundleDir: ${git:root}/site
      domain: "docs.mycompany.com"
      indexDocument: index.html
      errorDocument: index.html
```

**Enhanced Example with Dynamic Placeholders:**
```yaml
# .sc/stacks/docs-site/client.yaml - Using new placeholders safely
schemaVersion: 1.0
stacks:
  # Dynamic stack name based on git branch for branch-specific deployments
  "${git:branch}":
    type: static
    parent: myorg/infrastructure
    config:
      bundleDir: ${git:root}/site
      # Dynamic domain based on branch: main.docs.mycompany.com, staging.docs.mycompany.com
      domain: "${git:branch}.docs.mycompany.com"
      indexDocument: index.html
      errorDocument: index.html
      # Note: Avoid version field here as it causes CloudFront recreation
      # Version tracking can be done in build process or deployment metadata
```

**Features:**

- Automated MkDocs build and deployment
- CloudFront CDN distribution
- Custom domain with SSL/TLS
- Automatic cache invalidation

### Landing Page
Deploy a company landing page or marketing site.

**Use Case:** Company websites, product landing pages, marketing campaigns

**Configuration:**
```yaml
# .sc/stacks/landing/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: static
    parent: myorg/infrastructure
    config:
      bundleDir: ${git:root}/dist
      domain: "www.mycompany.com"
      indexDocument: index.html
      errorDocument: index.html  # SPA routing support
```

**Features:**

- SPA (Single Page Application) routing
- Custom domain with SSL/TLS
- Global CDN distribution
- SEO optimization support

### Admin Dashboard
Deploy an admin dashboard or internal tool.

**Use Case:** Internal admin interfaces, dashboards, management tools

**Configuration:**
```yaml
# .sc/stacks/admin/client.yaml
schemaVersion: 1.0
stacks:
  staging:
    type: static
    parent: myorg/infrastructure
    config:
      bundleDir: ${git:root}/build
      domain: "admin-staging.mycompany.com"
      indexDocument: index.html
      errorDocument: index.html
  production:
    type: static
    parent: myorg/infrastructure
    config:
      bundleDir: ${git:root}/build
      domain: "admin.mycompany.com"
      indexDocument: index.html
      errorDocument: index.html
```

**Features:**

- Multi-environment deployment (staging/production)
- Basic authentication for security
- Internal tool deployment
- Secure access controls

### Customer Portal
Deploy a customer-facing portal or self-service interface.

**Use Case:** Customer portals, self-service interfaces, user dashboards

**Configuration:**
```yaml
# .sc/stacks/portal/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: static
    parent: myorg/infrastructure
    config:
      bundleDir: ${git:root}/build
      domain: "portal.mycompany.com"
      indexDocument: index.html
      errorDocument: index.html
```

**Features:**

- Customer-facing interface
- Security headers configuration
- SPA routing support
- Custom domain and SSL

## Common Patterns

### Multi-Environment Setup
```yaml
stacks:
  staging:
    type: aws-static-website
    parent: myorg/infrastructure
    parentEnv: staging
    config:
      domain: "staging.mysite.com"
  production:
    type: aws-static-website
    parent: myorg/infrastructure
    parentEnv: production
    config:
      domain: "mysite.com"
```

### Custom Build Process
```yaml
# docker-compose.yaml
version: '3.8'
services:
  build:
    build: .
    volumes:
      - ./dist:/app/dist
    command: npm run build
```

## Deployment Commands

**Deploy to staging:**
```bash
sc deploy -s mysite -e staging
```

**Deploy to production:**
```bash
sc deploy -s mysite -e production
```

## Best Practices

- **Use environment-specific domains** for staging vs production
- **Enable SPA routing** for single-page applications
- **Configure security headers** for enhanced security
- **Set up proper caching** through CloudFront configuration
- **Use basic auth** for internal tools and admin interfaces

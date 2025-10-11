# DNS Management with Simple Container

This comprehensive guide covers how to configure and manage DNS records and domain routing using Simple Container's integrated DNS management capabilities. Simple Container provides seamless domain management through Cloudflare integration with full DNS record control and security features.

## Overview

Simple Container's DNS management provides:

- **Cloudflare Integration** for reliable DNS hosting and CDN capabilities
- **Declarative Configuration** for version-controlled domain management
- **Multi-Environment Support** with environment-specific domains
- **Security Features** including Cloudflare-only ingress protection
- **Email Infrastructure** with SPF, DKIM, and DMARC record support
- **Template Placeholders** for dynamic domain generation
- **Load Balancer Integration** with automatic DNS record creation

## DNS Architecture

Simple Container uses a resource-based approach to DNS management:

1. **Parent Stack** defines DNS resources and capabilities
2. **Child Applications** reference domains in their client.yaml configuration
3. **Environment-Specific** domains are automatically managed
4. **Security Policies** are enforced at the DNS level

## Prerequisites

Before setting up DNS management, ensure you have:

- Cloudflare account with API access
- Domain registered and using Cloudflare nameservers
- Simple Container CLI installed
- Parent stack with DNS resource configured

## Setting Up DNS Resources

### 1. Parent Stack Configuration

Define DNS management resources in your parent stack's `server.yaml`:

```yaml
# server.yaml - Parent Stack
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "mycompany.com"
      dnsRecords:
        - name: "api"
          type: "CNAME"
          value: "api-lb.aws.mycompany.com"
        - name: "www"
          type: "CNAME"
          value: "mycompany.com"
        - name: "@"
          type: "A"
          value: "203.0.113.10"
        - name: "@"
          type: "TXT"
          value: "v=spf1 include:_spf.google.com ~all"
```

### 2. Authentication Setup

Configure Cloudflare credentials in your `secrets.yaml`:

```yaml
# secrets.yaml
auth:
  cloudflare:
    credentials: "${secret:cloudflare-api-token}"

values:
  cloudflare-api-token: "your-cloudflare-api-token"
  cloudflare-account-id: "your-cloudflare-account-id"
```

## Application Domain Configuration

### 1. Basic Domain Assignment

Configure domains in your application's `client.yaml`:

```yaml
# client.yaml
stacks:
  staging:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      domain: staging-api.mycompany.com
      dockerComposeFile: docker-compose.yaml
      runs: [web-app]
      
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      domain: api.mycompany.com
      dockerComposeFile: docker-compose.yaml
      runs: [web-app]
```

### 2. Dynamic Domain Generation

Use template placeholders for automatic domain generation:

```yaml
# Environment-based domains
stacks:
  staging:
    config:
      domain: "${stack:name}-api.mycompany.com"  # staging-api.mycompany.com
      
  production:
    config:
      domain: "api.mycompany.com"  # Production root domain
```

### 3. Environment Variable Domains

Support runtime domain configuration:

```yaml
# client.yaml
stacks:
  staging:
    config:
      domain: "${env:STAGING_DOMAIN:staging.mycompany.com}"
      env:
        BASE_URL: "https://${env:STAGING_DOMAIN:staging.mycompany.com}"
```

## Advanced DNS Features

### 1. Email Infrastructure

Configure email-related DNS records:

```yaml
# server.yaml - Email infrastructure
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "mycompany.com"
      dnsRecords:
        # SPF Record
        - name: "@"
          type: "TXT"
          value: "v=spf1 include:_spf.google.com include:_spf.hubspot.com ~all"
        
        # DKIM Record for Google Workspace
        - name: "google._domainkey"
          type: "TXT"
          value: "v=DKIM1; k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC..."
        
        # DMARC Policy
        - name: "_dmarc"
          type: "TXT"
          value: "v=DMARC1; p=quarantine; rua=mailto:dmarc@mycompany.com"
        
        # MX Records
        - name: "@"
          type: "MX"
          value: "1 smtp.google.com"
        - name: "@"
          type: "MX"
          value: "5 alt1.smtp.google.com"
```

### 2. Security Configuration

Implement security-focused DNS configurations:

```yaml
# Cloudflare-only ingress protection
stacks:
  production:
    config:
      domain: api.mycompany.com
      cloudExtras:
        securityGroup:
          ingress:
            allowOnlyCloudflare: true  # Only allow Cloudflare IPs
```

### 3. Multi-Region DNS

Configure DNS for multi-region deployments:

```yaml
# server.yaml - Multi-region setup
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "mycompany.com"
      dnsRecords:
        # US Region
        - name: "us-api"
          type: "CNAME"
          value: "us-east-1-lb.aws.mycompany.com"
        
        # EU Region
        - name: "eu-api"
          type: "CNAME"
          value: "eu-west-1-lb.aws.mycompany.com"
        
        # Geographic routing (requires Cloudflare enterprise)
        - name: "api"
          type: "CNAME"
          value: "us-api.mycompany.com"
          geoPolicy: "US"
        - name: "api"
          type: "CNAME"
          value: "eu-api.mycompany.com"
          geoPolicy: "EU"
```

## DNS Record Types

Simple Container supports all common DNS record types:

### A Records
```yaml
dnsRecords:
  - name: "@"
    type: "A"
    value: "203.0.113.10"
  - name: "www"
    type: "A"
    value: "203.0.113.10"
```

### CNAME Records
```yaml
dnsRecords:
  - name: "api"
    type: "CNAME"
    value: "api-lb.us-east-1.elb.amazonaws.com"
  - name: "www"
    type: "CNAME"
    value: "mycompany.com"
```

### TXT Records
```yaml
dnsRecords:
  - name: "@"
    type: "TXT"
    value: "v=spf1 include:_spf.google.com ~all"
  - name: "_verification"
    type: "TXT"
    value: "google-site-verification=abc123..."
```

### MX Records
```yaml
dnsRecords:
  - name: "@"
    type: "MX"
    value: "1 smtp.google.com"
  - name: "@"
    type: "MX"
    value: "10 alt1.smtp.google.com"
```

## Multi-Tenant DNS

### Customer-Specific Domains

Configure per-customer domains:

```yaml
# client.yaml - Multi-tenant application
stacks:
  customer-a:
    type: cloud-compose
    parent: myorg/saas-infrastructure
    config:
      domain: customera.myapp.com
      uses: [mongodb-cluster-us]
      runs: [app]
      secrets:
        CUSTOMER_SETTINGS: "${env:CUSTOMER_A_SETTINGS}"

  customer-b:
    type: cloud-compose
    parent: myorg/saas-infrastructure
    config:
      domain: customerb.myapp.com
      uses: [mongodb-cluster-eu]
      runs: [app]
      secrets:
        CUSTOMER_SETTINGS: "${env:CUSTOMER_B_SETTINGS}"
```

### Wildcard Domains

Support subdomain-based multi-tenancy:

```yaml
# server.yaml - Wildcard configuration
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "myapp.com"
      dnsRecords:
        - name: "*"
          type: "CNAME"
          value: "tenant-lb.us-east-1.elb.amazonaws.com"
```

## Deployment Integration

### Automatic DNS Updates

DNS records are automatically managed during deployment:

```bash
# Deploy with DNS configuration
sc deploy -s myapp -e production

# DNS records are automatically:
# 1. Created for new domains
# 2. Updated for changed load balancers
# 3. Verified for SSL certificate validation
```

### SSL Certificate Integration

Simple Container automatically handles SSL certificates with DNS validation:

```yaml
# Automatic SSL with DNS validation
stacks:
  production:
    config:
      domain: api.mycompany.com  # SSL cert auto-requested via DNS validation
      sslPolicy: "automatic"     # Default: automatic certificate management
```

## Monitoring and Troubleshooting

### DNS Propagation

Check DNS propagation after deployment:

```bash
# Check DNS propagation
dig api.mycompany.com @8.8.8.8
nslookup api.mycompany.com
```

### Cloudflare Dashboard Integration

Monitor DNS through Cloudflare dashboard:

1. **Analytics**: DNS query analytics and performance metrics
2. **Security**: DDoS protection and security events
3. **Cache**: CDN cache hit ratios and purge controls
4. **SSL/TLS**: Certificate status and security settings

### Common Issues

#### DNS Not Resolving
```bash
# Check if nameservers are correctly set
dig NS mycompany.com

# Verify Cloudflare integration
sc stack get-config -s infrastructure --explain
```

#### SSL Certificate Issues
```bash
# Check certificate status
openssl s_client -connect api.mycompany.com:443 -servername api.mycompany.com

# Verify DNS validation records
dig _acme-challenge.api.mycompany.com TXT
```

## Best Practices

### 1. Environment Naming

Use consistent environment-based domain patterns:

```yaml
# Good - Clear environment prefixes
staging: staging-api.mycompany.com
production: api.mycompany.com

# Avoid - Unclear naming
staging: api-stg.mycompany.com
production: api-prod.mycompany.com  # Should be root domain
```

### 2. Security Configuration

Always enable security features for production:

```yaml
# Production security setup
stacks:
  production:
    config:
      domain: api.mycompany.com
      cloudExtras:
        securityGroup:
          ingress:
            allowOnlyCloudflare: true
        proxy:
          enabled: true      # Enable Cloudflare proxy
          sslMode: "strict"  # Strict SSL/TLS
```

### 3. Template Placeholder Usage

Use placeholders for maintainable configurations:

```yaml
# Good - Dynamic and maintainable
domain: "${stack:name}.mycompany.com"
baseUrl: "https://${stack:name}.mycompany.com"

# Avoid - Hardcoded and error-prone
domain: "staging.mycompany.com"  # Must be changed per environment
```

### 4. Email Infrastructure Planning

Plan email infrastructure early:

```yaml
# Include email setup in initial DNS configuration
dnsRecords:
  # Email delivery
  - name: "@"
    type: "MX"
    value: "1 smtp.google.com"
  
  # Email authentication
  - name: "@"
    type: "TXT"
    value: "v=spf1 include:_spf.google.com ~all"
  
  # Email security
  - name: "_dmarc"
    type: "TXT"
    value: "v=DMARC1; p=quarantine; rua=mailto:dmarc@mycompany.com"
```

## Real-World Examples

### E-commerce Platform
```yaml
# Multi-service e-commerce DNS
dnsRecords:
  - name: "@"
    type: "A"
    value: "203.0.113.10"        # Main site
  - name: "api"
    type: "CNAME"
    value: "api-lb.aws.com"      # API service
  - name: "admin"
    type: "CNAME"
    value: "admin-lb.aws.com"    # Admin panel
  - name: "cdn"
    type: "CNAME"
    value: "d123.cloudfront.net" # CDN assets
```

### SaaS Application
```yaml
# Multi-tenant SaaS DNS
dnsRecords:
  - name: "*"
    type: "CNAME"
    value: "saas-lb.aws.com"     # Tenant subdomains
  - name: "app"
    type: "CNAME"
    value: "app-lb.aws.com"      # Main application
  - name: "docs"
    type: "CNAME"
    value: "docs-s3.aws.com"     # Documentation site
```

## Next Steps

After setting up DNS management:

1. **Configure SSL certificates** for secure HTTPS connections
2. **Set up monitoring** for DNS resolution and performance
3. **Implement CDN caching** strategies for optimal performance
4. **Plan disaster recovery** with backup DNS providers
5. **Review security settings** regularly for Cloudflare protection

## Need Help?

- Review **[Parent Stack Guides](parent-ecs-fargate.md)** for infrastructure setup
- Check **[Secrets Management](secrets-management.md)** for credential handling
- Explore **[Examples](../examples/README.md)** for real-world DNS configurations
- Contact [support@simple-container.com](mailto:support@simple-container.com) for assistance

## Related Resources

- **[Cloudflare DNS API Documentation](https://developers.cloudflare.com/api/operations/dns-records-for-a-zone-list-dns-records)**
- **[Template Placeholders](../concepts/template-placeholders.md)** for dynamic configuration
- **[Multi-Region Deployment Examples](../examples/parent-stacks/aws-multi-region/README.md)**
- **[Security Best Practices](../advanced/best-practices.md)**

# Meteor.js Application Example

This example shows how to deploy a Meteor.js application with MongoDB integration, media storage, and Cloudflare security.

## Configuration

- **Type**: ECS cloud-compose deployment
- **Size**: 1024 CPU, 2048MB memory
- **Framework**: Meteor.js application
- **Database**: MongoDB with URI references
- **Storage**: Media storage integration
- **Security**: Cloudflare-only ingress protection

## Key Features

- **Meteor.js Deployment**: Complete Meteor application configuration
- **Multi-Resource Usage**: MongoDB + S3 media storage
- **Security**: Cloudflare-only ingress protection
- **Environment-Specific Secrets**: Different Meteor settings per environment
- **Resource References**: `${resource:mongodb.uri}` pattern
- **Root Domain Production**: Production uses root domain

## Environments

- **Staging**: `staging.example.com`
- **Production**: `example.com` (root domain)

## Meteor Configuration

- **ROOT_URL**: Environment-specific root URL
- **PORT**: 3000 (standard Meteor port)
- **NODE_ENV**: production (for both environments)
- **METEOR_SETTINGS**: Environment-specific JSON configuration
- **MAIL_URL**: SMTP configuration for Meteor email

## Security Configuration

```yaml
cloudExtras:
  securityGroup:
    ingress:
      allowOnlyCloudflare: true
```

This ensures only Cloudflare can access the application directly.

## Usage

1. Ensure your parent stack provides MongoDB and media storage resources
2. Configure Meteor settings JSON for each environment
3. Set up SMTP mail URL for email functionality
4. Deploy to staging for testing
5. Promote to production (root domain)

## Parent Stack Requirements

This example requires a parent stack that provides:
- `mongodb` resource with URI access
- `media-storage` resource (S3 bucket)
- ECS deployment capabilities
- Domain management for root domain
- Cloudflare integration for security

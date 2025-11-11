# AWS Multi-Region Parent Stack Example

This example shows how to create a comprehensive AWS parent stack with multi-region deployment, extensive DNS configuration, and production-grade resource management.

## Configuration

- **Provisioner**: Pulumi with S3 state storage and AWS KMS secrets
- **Multi-Region**: EU (staging) and US (production) deployments
- **Templates**: ECS Fargate, Static Website, Lambda for both regions
- **DNS**: Comprehensive Cloudflare integration with SPF, DKIM records

## Key Features

- **Multi-Region AWS Setup**: Different regions for staging (EU) vs production (US)
- **Extensive DNS Configuration**: SPF records for Google and HubSpot email, DKIM configuration
- **Production-Grade MongoDB**: Different instance sizes and backup policies per environment
- **Template Reuse**: Shared templates with region-specific configurations
- **Email Service Integration**: Complete email infrastructure setup

## Regional Configuration

### Staging (Europe)
- **Region**: EU Central (aws-eu credentials)
- **Template**: `stack-per-app-eu`
- **MongoDB**: M10 instance in EU_CENTRAL_1
- **Backup**: 4h frequency, 24h retention

### Production (US)
- **Region**: US East (aws-us credentials)  
- **Template**: `stack-per-app-us`
- **MongoDB**: M30 instance in US_EAST_1
- **Backup**: 1h frequency, 168h (1 week) retention

## DNS Configuration

### Email Infrastructure
- **SPF Record**: `v=spf1 include:_spf.google.com ~all`
- **HubSpot Integration**: `include:143683367.spf06.hubspotemail.net`
- **DKIM**: Domain key authentication for email security

### Domain Management
- **Zone**: `example.com`
- **Cloudflare**: Complete DNS management with proxy control
- **Email Security**: SPF and DKIM for deliverability

## Templates Provided

- **ECS Fargate**: `stack-per-app-eu`, `stack-per-app-us`
- **Static Website**: `static-eu` for European deployments
- **Lambda Functions**: `lambda-eu` for serverless deployments

## Resources Provided

- **Media Storage**: S3 bucket for file storage
- **MongoDB Atlas**: Production-grade database with regional deployment
- **DNS Management**: Complete Cloudflare integration

## Usage

1. Configure AWS credentials for both EU and US regions
2. Set up Cloudflare API token for DNS management
3. Configure MongoDB Atlas credentials and organization
4. Deploy the parent stack first
5. Use the provided templates for service deployments

## Authentication Requirements

- **AWS EU**: Service account with EU region permissions
- **AWS US**: Service account with US region permissions
- **Cloudflare**: API token for DNS management
- **MongoDB Atlas**: Public/private key pair with organization access

## Scaling Considerations

- **Regional Isolation**: EU staging, US production for compliance
- **Database Scaling**: M10 staging → M30 production
- **Backup Strategy**: More frequent backups and longer retention in production
- **Cost Optimization**: Different instance sizes per environment

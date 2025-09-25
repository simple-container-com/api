# Blog Platform Example

This example shows how to deploy a blog platform with multi-service deployment (Caddy reverse proxy + blog application), MySQL database integration, and Gmail SMTP configuration.

## Configuration

- **Type**: ECS cloud-compose deployment
- **Size**: 1024 CPU, 2048MB memory
- **Services**: Caddy reverse proxy + Blog application
- **Database**: MySQL with complete resource references
- **Email**: Gmail SMTP integration for notifications

## Key Features

- **Multi-Service Deployment**: Caddy + Blog application in one stack
- **MySQL Integration**: Complete MySQL resource references with multiple environment variables
- **Gmail SMTP**: Full Gmail configuration for blog notifications and user emails
- **Custom Reverse Proxy**: Caddy configuration for routing and SSL termination
- **Multi-Environment**: Staging and production domains

## Environments

- **Staging**: `staging-blog.example.com`
- **Production**: `blog.example.com`

## Database Configuration

Uses comprehensive MySQL resource references:
- `${resource:mysql.user}` - Database username
- `${resource:mysql.password}` - Database password
- `${resource:mysql.host}` - Database host
- `${resource:mysql.port}` - Database port
- `${resource:mysql.database}` - Database name

## Email Configuration

Gmail SMTP setup for blog notifications:
- **Service**: Gmail SMTP
- **Security**: SSL/TLS enabled
- **Port**: 465 (secure)
- **Authentication**: Gmail account credentials

## Usage

1. Ensure your parent stack provides MySQL resource
2. Configure Gmail credentials for SMTP
3. Prepare Caddy configuration and Dockerfile
4. Deploy to staging for testing
5. Promote to production when ready

## Parent Stack Requirements

This example requires a parent stack that provides:
- `mysql` resource with full connection details
- ECS deployment capabilities
- Domain management
- Secrets management for Gmail credentials

## Files Included

- `client.yaml` - Main deployment configuration
- `docker-compose.yaml` - Multi-service container definitions
- `Caddyfile` - Reverse proxy configuration
- `caddy.Dockerfile` - Custom Caddy container build

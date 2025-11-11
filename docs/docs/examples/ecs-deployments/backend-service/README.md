# Backend Service Example

This example shows how to deploy a Node.js backend service with MongoDB integration, GraphQL API connections, and external service integrations.

## Configuration

- **Type**: ECS cloud-compose deployment
- **Size**: 1024 CPU, 2048MB memory
- **Scaling**: Min 2, Max 3 instances with 45% CPU threshold
- **Database**: MongoDB with resource URI references
- **External APIs**: GraphQL API integration, Brevo email, Telegram bot

## Key Features

- **Lower CPU Scaling Threshold**: 45% (optimized for financial/API services)
- **GraphQL API Integration**: External GraphQL endpoint connections
- **Resource URI References**: `${resource:mongodb.uri}` pattern
- **Multi-Environment**: Staging and production with different secrets
- **External Service Integration**: Email service, Telegram bot, admin systems
- **High Availability**: Minimum 2 replicas for zero downtime

## Environments

- **Staging**: `staging-api.example.com`
- **Production**: `api.example.com`

## Resource Usage

- **MongoDB**: `${resource:mongodb.uri}` from parent stack
- **Secrets**: Environment-specific API keys and credentials
- **External APIs**: GraphQL endpoints for account management

## Usage

1. Ensure your parent stack provides MongoDB resource
2. Configure secrets for each environment
3. Deploy to staging first for testing
4. Promote to production when ready

## Parent Stack Requirements

This example requires a parent stack that provides:
- `mongodb` resource with URI access
- ECS deployment capabilities
- Domain management
- Secrets management for API keys

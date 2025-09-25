# Customer Portal Example

This example shows how to deploy a customer-facing UI with multi-environment configuration.

## Configuration

- **Type**: Static website deployment
- **Template**: Uses `static-site` template from parent stack
- **Bundle Directory**: `${git:root}/build` - React/Vue build output
- **Multi-Environment**: Staging and production domains
- **SPA Configuration**: Same file for index and error documents

## Key Features

- Customer-facing UI deployment
- Multi-environment configuration with YAML anchors
- SPA configuration for client-side routing
- Customer-friendly domain naming

## Environments

- **Staging**: `staging-app.example.com`
- **Production**: `app.example.com`

## Usage

1. Build your customer portal: `npm run build`
2. Deploy to staging first for testing
3. Promote to production when ready

## Parent Stack Requirements

This example requires a parent stack that provides:
- `static-site` template
- Static website deployment capabilities
- Domain management for both environments

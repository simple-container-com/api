# Admin Dashboard Example

This example shows how to deploy an admin UI with multi-environment configuration.

## Configuration

- **Type**: Static website deployment
- **Template**: Uses `static-site` template from parent stack
- **Bundle Directory**: `${git:root}/build` - React/Vue build output
- **Multi-Environment**: Staging and production domains
- **SPA Configuration**: Same file for index and error documents

## Key Features

- Admin UI deployment
- Multi-environment configuration with YAML anchors
- SPA configuration for client-side routing
- Environment-specific domains

## Environments

- **Staging**: `staging-admin.example.com`
- **Production**: `admin.example.com`

## Usage

1. Build your admin dashboard: `npm run build`
2. Deploy to staging first for testing
3. Promote to production when ready

## Parent Stack Requirements

This example requires a parent stack that provides:
- `static-site` template
- Static website deployment capabilities
- Domain management for both environments

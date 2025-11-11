# Streaming Platform Example

This example shows how to deploy a streaming platform with hardcoded infrastructure IPs, N8N workflow automation, and zero-downtime deployment configuration.

## Configuration

- **Type**: ECS cloud-compose deployment
- **Parent**: `mycompany`
- **Database**: PostgreSQL with hardcoded private IP
- **Workflow**: N8N integration with disabled modules
- **Zero-Downtime**: Advanced deployment configurations

## Key Features

- **Hardcoded Infrastructure IPs**: Different cluster IPs per environment
  - Staging: `10.0.1.100`
  - Production: `10.0.2.100`
- **Hardcoded Database IP**: `10.120.0.3` for shared PostgreSQL instance
- **N8N Workflow Integration**: Automation with disabled modules (`insights,external-secrets`)
- **Zero-Downtime Deployment**: `minAvailable: 0`, `maxSurge: 0` configurations
- **Multi-Domain**: Different base DNS zones per environment

## Infrastructure Configuration

### Hardcoded IPs
- **Cluster IPs**: External cluster references for existing infrastructure
- **Database IP**: Private PostgreSQL instance IP
- **Environment-Specific**: Different IPs for staging vs production

### Zero-Downtime Deployment
```yaml
cloudExtras:
  disruptionBudget:
    minAvailable: 0      # Allow complete shutdown during updates
  rollingUpdate:
    maxSurge: 0          # No additional pods during updates
```

## N8N Integration

- **Workflow Automation**: N8N for data processing workflows
- **Disabled Modules**: `insights,external-secrets` for security
- **Analytics Integration**: Connected to analytics service

## Environments

- **Staging**: `streams.example.com` (staging cluster)
- **Production**: `streams.example.com` (production cluster with different base DNS)

## Use Cases

- **Media Streaming**: Video/audio streaming platform
- **Data Streaming**: Real-time data processing
- **Workflow Automation**: N8N-based data pipelines
- **Analytics Integration**: Stream analytics and monitoring

## Usage

1. Ensure external Kubernetes clusters are available at specified IPs
2. Configure PostgreSQL database at the hardcoded IP
3. Set up N8N workflow automation
4. Configure analytics service integration
5. Deploy with zero-downtime configuration

## Parent Stack Requirements

This example requires a parent stack that provides:
- ECS deployment capabilities with external cluster support
- Domain management with base DNS zone configuration
- Secrets management for database and service credentials
- Support for hardcoded infrastructure references

## Infrastructure Considerations

- **External Dependencies**: Relies on existing cluster and database infrastructure
- **IP Management**: Hardcoded IPs require careful network planning
- **Zero-Downtime**: Configured for controlled updates with no service interruption
- **Workflow Integration**: N8N requires proper module configuration

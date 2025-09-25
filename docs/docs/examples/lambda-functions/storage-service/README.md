# Storage Service Example

This example shows how to deploy a storage service with scheduled cleanup jobs using cron expressions, multiple resource usage (S3 + MongoDB), and automated cleanup operations.

## Configuration

- **Type**: Lambda single-image deployment
- **Template**: Uses `lambda-eu` template from parent stack
- **Timeout**: 120 seconds, 512MB memory
- **Resources**: Object storage (S3) + MongoDB integration
- **Scheduling**: Automated hourly cleanup jobs

## Key Features

- **Scheduled Jobs**: Cron-based cleanup automation with `cron(0 * * * ? *)`
- **Multiple Resource Usage**: Both S3 storage and MongoDB
- **Automated API Calls**: Scheduled cleanup with Bearer token authentication
- **Response Streaming**: `RESPONSE_STREAM` for real-time processing feedback
- **Function URL**: Direct HTTP access for manual operations

## Scheduled Cleanup

The service automatically runs cleanup every hour:
```yaml
lambdaSchedules:
  - name: cleanup
    expression: "cron(0 * * * ? *)" # every hour
    request: # Automated POST request to /api/cleanup
```

## Environments

- **Staging**: `staging-storage-service.example.com`

## Resource Usage

- **Object Storage**: S3 bucket for file storage
- **MongoDB**: Metadata and indexing storage
- **Proxy Service**: External storage proxy integration

## Authentication

- **Bearer Token**: API key authentication for scheduled jobs
- **Headers**: `Authorization: Bearer ${secret:staging-storage-service-api-key}`

## Usage

1. Ensure your parent stack provides object storage and MongoDB resources
2. Configure API keys for authentication
3. Set up storage proxy URL if using external storage
4. Deploy and monitor cleanup job execution
5. Check logs for automated cleanup operations

## Parent Stack Requirements

This example requires a parent stack that provides:
- `lambda-eu` template with scheduling support
- `object-storage` resource (S3 bucket)
- `mongodb` resource for metadata
- Lambda deployment with cron scheduling
- Secrets management for API keys

## Monitoring

- **Request Debug**: Enabled in staging for troubleshooting
- **Cleanup Logs**: Monitor hourly cleanup job execution
- **Storage Metrics**: Track storage usage and cleanup effectiveness

# Scheduler Example

This example shows how to deploy a high-frequency scheduler service with every-minute execution, automated reporting system, and Bearer token authentication.

## Configuration

- **Type**: Lambda single-image deployment
- **Template**: Uses `lambda-eu` template from parent stack
- **Timeout**: 300 seconds, 512MB memory
- **Database**: MongoDB integration for task storage
- **Frequency**: Every minute execution (`cron(* * * * ? *)`)

## Key Features

- **High-Frequency Scheduling**: Every minute execution for time-critical tasks
- **Automated Reporting**: JSON body with `{"report":true}` for task reporting
- **Bearer Token Authentication**: API key authentication for scheduled jobs
- **Function URL**: Direct HTTP access for manual task execution
- **MongoDB Integration**: Task storage and execution tracking

## Scheduling Configuration

The service runs every minute:
```yaml
lambdaSchedules:
  - name: every-minute
    expression: "cron(* * * * ? *)" # every minute
    request: # Automated POST request to /api/execute
```

## Environments

- **Staging**: `scheduler.example.com`

## Use Cases

- **Task Queue Processing**: Process pending tasks every minute
- **Health Checks**: Monitor system health at high frequency
- **Data Synchronization**: Keep data in sync across systems
- **Alert Processing**: Handle time-sensitive alerts quickly
- **Batch Job Coordination**: Coordinate distributed batch operations

## Authentication

- **Bearer Token**: API key authentication for all requests
- **Headers**: `Authorization: Bearer ${secret:staging-scheduler-api-key}`

## Usage

1. Ensure your parent stack provides MongoDB resource for task storage
2. Configure API keys for authentication
3. Deploy and monitor high-frequency execution
4. Check logs for every-minute task processing
5. Scale timeout and memory based on task complexity

## Parent Stack Requirements

This example requires a parent stack that provides:
- `lambda-eu` template with high-frequency scheduling support
- `mongodb` resource for task storage and tracking
- Lambda deployment with cron scheduling (every minute)
- Secrets management for API keys

## Monitoring Considerations

- **High Frequency**: Monitor Lambda costs due to every-minute execution
- **Execution Time**: Track task execution time to optimize timeout
- **Error Handling**: Implement robust error handling for failed tasks
- **Rate Limits**: Consider downstream service rate limits

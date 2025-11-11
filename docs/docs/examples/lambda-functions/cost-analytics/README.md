# Cost Analytics Example

This example shows how to deploy a comprehensive AWS cost analysis service with extensive IAM permissions, daily CRM synchronization, and high-resource Lambda configuration for data processing.

## Configuration

- **Type**: Lambda single-image deployment
- **Template**: Uses `lambda-eu` template from parent stack
- **Timeout**: 600 seconds (10 minutes) for complex data processing
- **Memory**: 1024MB for handling large cost datasets
- **Database**: MongoDB for cost data storage and analysis
- **Scheduling**: Daily CRM synchronization at midnight UTC

## Key Features

- **Comprehensive AWS Cost Explorer Roles**: 20+ IAM permissions for complete cost analysis
- **Daily CRM Synchronization**: Automated sync with CRM systems for cost reporting
- **High-Resource Configuration**: 10-minute timeout and 1GB memory for data processing
- **Response Streaming**: Real-time processing feedback for long-running operations
- **Multi-Service Integration**: Cost Explorer, Budgets, CloudWatch, CloudWatch Logs

## AWS IAM Permissions

### Cost Explorer Permissions
- `ce:GetCostAndUsage` - Retrieve cost and usage data
- `ce:GetAnomalies` - Detect cost anomalies
- `ce:GetAnomalyMonitors` - Monitor cost patterns
- `ce:GetPreferences` - Access cost preferences
- `ce:DescribeReport` - Generate cost reports

### Budget Permissions
- `budgets:ViewBudget` - Access budget information

### CloudWatch Permissions
- `cloudwatch:GetMetricData` - Retrieve metrics data
- `cloudwatch:ListMetrics` - List available metrics

### CloudWatch Logs Permissions
- Complete log analysis permissions for cost-related log processing
- Query execution and result retrieval for log-based cost analysis

## Scheduled Operations

Daily CRM synchronization at midnight UTC:
```yaml
lambdaSchedules:
  - name: crm-sync
    expression: "cron(0 0 * * ? *)" # every day at 00:00 UTC
```

## Environments

- **Staging**: `costs.example.com`

## Use Cases

- **Cost Reporting**: Generate comprehensive AWS cost reports
- **Anomaly Detection**: Identify unusual spending patterns
- **Budget Monitoring**: Track budget utilization and alerts
- **CRM Integration**: Sync cost data with customer relationship management
- **Log Analysis**: Analyze logs for cost optimization opportunities

## Usage

1. Ensure your AWS account has Cost Explorer API access enabled
2. Configure the extensive IAM roles for cost analysis
3. Set up CRM API integration credentials
4. Deploy and monitor daily synchronization
5. Use the high timeout for complex cost calculations

## Parent Stack Requirements

This example requires a parent stack that provides:
- `lambda-eu` template with comprehensive IAM role support
- `mongodb` resource for cost data storage
- Lambda deployment with extended timeout capabilities
- Secrets management for CRM integration
- Cost Explorer API access configuration

## Performance Considerations

- **High Memory**: 1GB for processing large cost datasets
- **Extended Timeout**: 10 minutes for complex calculations
- **Daily Processing**: Optimized for once-daily heavy processing
- **Streaming Response**: Real-time feedback for long operations

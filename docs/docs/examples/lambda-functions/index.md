# Lambda Functions Examples

This section contains examples of deploying AWS Lambda functions using Simple Container.

## Available Examples

### AI Gateway
Deploy an AWS Bedrock integration service with specific IAM roles.

**Use Case:** AI/ML API gateway, content generation, natural language processing

**Configuration:**
```yaml
# .sc/stacks/ai-gateway/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: single-image
    template: lambda-us
    parent: myorg/infrastructure
    config:
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 300
      maxMemory: 2048
      env:
        BEDROCK_REGION: us-east-1
        MODEL_ID: anthropic.claude-v2
```

**Dockerfile:**
```dockerfile
FROM public.ecr.aws/lambda/python:3.11
COPY requirements.txt ${LAMBDA_TASK_ROOT}
RUN pip install -r requirements.txt
COPY lambda_function.py ${LAMBDA_TASK_ROOT}
CMD ["lambda_function.lambda_handler"]
```

**Features:**
- AWS Bedrock integration
- Specific IAM roles for AI services
- Custom model configuration
- Secure API endpoints
- Cost-optimized serverless execution

### Storage Service
Deploy scheduled cleanup with cron expressions.

**Use Case:** Data cleanup, file management, automated maintenance tasks

**Configuration:**
```yaml
# .sc/stacks/storage-cleanup/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: single-image
    template: lambda-us
    parent: myorg/infrastructure
    config:
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 900
      maxMemory: 1024
      cloudExtras:
        lambdaInvokeMode: RESPONSE_STREAM
        lambdaRoutingType: function-url
        lambdaSchedules:
          - name: daily-cleanup
            expression: "cron(0 2 * * ? *)"  # Daily at 2 AM
            request: |-
              {
                "requestId": "daily-cleanup",
                "httpMethod": "POST",
                "path": "/api/cleanup",
                "body": "{\"action\":\"cleanup\"}",
                "headers": {
                  "Authorization": "Bearer ${secret:CLEANUP_API_KEY}"
                }
              }
      env:
        ENV: production
        HOME: /tmp
      secrets:
        S3_BUCKET: "${resource:storage-bucket.name}"
        RETENTION_DAYS: "30"
        CLEANUP_API_KEY: "${secret:CLEANUP_API_KEY}"
```

**Features:**
- Scheduled execution with cron expressions
- S3 bucket cleanup automation
- Configurable retention policies
- Error handling and logging
- Cost-effective maintenance automation

### Scheduler
Deploy high-frequency scheduling (every minute).

**Use Case:** Real-time monitoring, frequent data processing, system health checks

**Configuration:**
```yaml
# .sc/stacks/scheduler/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: single-image
    template: lambda-us
    parent: myorg/infrastructure
    config:
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 60
      maxMemory: 512
      cloudExtras:
        lambdaInvokeMode: RESPONSE_STREAM
        lambdaRoutingType: function-url
        lambdaSchedules:
          - name: health-check
            expression: "rate(1 minute)"
            request: |-
              {
                "requestId": "health-check",
                "httpMethod": "POST",
                "path": "/api/health-check",
                "body": "{\"check\":\"all\"}",
                "headers": {
                  "Authorization": "Bearer ${secret:SCHEDULER_API_KEY}"
                }
              }
      env:
        ENV: production
        HOME: /tmp
        MONITORING_ENDPOINT: "https://api.mycompany.com/health"
      secrets:
        ALERT_WEBHOOK: "${secret:SLACK_WEBHOOK_URL}"
        SCHEDULER_API_KEY: "${secret:SCHEDULER_API_KEY}"
```

**Features:**
- High-frequency execution (every minute)
- Real-time monitoring capabilities
- Webhook integration for alerts
- Fast execution and response
- Minimal cold start optimization

### Cost Analytics
Deploy AWS cost analysis with comprehensive IAM permissions.

**Use Case:** Cost monitoring, billing analysis, resource optimization

**Configuration:**
```yaml
# .sc/stacks/cost-analytics/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: single-image
    template: lambda-us
    parent: myorg/infrastructure
    config:
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 600
      maxMemory: 1024
      cloudExtras:
        lambdaInvokeMode: RESPONSE_STREAM
        lambdaRoutingType: function-url
        awsRoles:
          - ce:GetCostAndUsage
          - ce:GetUsageReport
          - ce:DescribeCostCategoryDefinition
          - budgets:ViewBudget
        lambdaSchedules:
          - name: daily-cost-report
            expression: "cron(0 8 * * ? *)"
            request: |-
              {
                "requestId": "daily-cost-report",
                "httpMethod": "POST",
                "path": "/api/generate-report",
                "body": "{\"reportType\":\"daily\"}",
                "headers": {
                  "Authorization": "Bearer ${secret:COST_ANALYTICS_API_KEY}"
                }
              }
      env:
        ENV: production
        HOME: /tmp
        COST_EXPLORER_REGION: us-east-1
        REPORT_S3_BUCKET: "${resource:reports-bucket.name}"
        NOTIFICATION_EMAIL: "${secret:ADMIN_EMAIL}"
```

**IAM Permissions Required:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetUsageReport",
        "ce:DescribeCostCategoryDefinition"
      ],
      "Resource": "*"
    }
  ]
}
```

**Features:**
- AWS Cost Explorer integration
- Comprehensive IAM permissions
- Automated cost reporting
- S3 report storage
- Email notifications

### Billing System
Deploy multi-environment billing with YAML anchors.

**Use Case:** Subscription billing, payment processing, invoice generation

**Configuration:**
```yaml
# .sc/stacks/billing/client.yaml
schemaVersion: 1.0

# YAML anchors for reusable configuration
x-billing-config: &billing-config
  image:
    dockerfile: ${git:root}/Dockerfile
  timeout: 300
  env:
    DATABASE_URL: "${resource:postgres-db.connectionString}"
    STRIPE_WEBHOOK_SECRET: "${secret:STRIPE_WEBHOOK_SECRET}"

stacks:
  staging:
    type: single-image
    template: lambda-us
    parent: myorg/infrastructure
    parentEnv: staging
    config:
      <<: *billing-config
      env:
        <<: *billing-config.env
        STRIPE_API_KEY: "${secret:STRIPE_TEST_KEY}"
        ENVIRONMENT: staging
  
  production:
    type: single-image
    template: lambda-us
    parent: myorg/infrastructure
    parentEnv: production
    config:
      <<: *billing-config
      env:
        <<: *billing-config.env
        STRIPE_API_KEY: "${secret:STRIPE_LIVE_KEY}"
        ENVIRONMENT: production
```

**Features:**
- Multi-environment deployment
- YAML anchors for configuration reuse
- Stripe payment integration
- Database connectivity
- Webhook handling
- Environment-specific API keys


## Deployment Commands

Deploy to staging:
```bash
sc deploy -s mylambda -e staging
```

Deploy to production:
```bash
sc deploy -s mylambda -e production
```

## Best Practices

- **Use appropriate timeout values** based on function complexity
- **Implement proper error handling** and retry logic
- **Configure dead letter queues** for failed executions
- **Use environment variables** for configuration
- **Optimize cold start performance** with proper runtime selection
- **Monitor function metrics** and set up appropriate alarms
- **Use IAM roles with least privilege** principle
- **Implement structured logging** for better observability
- **Consider memory allocation** for optimal cost/performance ratio

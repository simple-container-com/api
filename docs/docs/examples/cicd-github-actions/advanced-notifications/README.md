# Advanced Notifications Example

This example demonstrates comprehensive notification setup with multiple channels, custom templates, and team mentions for CI/CD deployments.

## Overview

This setup provides:
- **Multi-channel notifications** (Slack, Discord, Telegram)
- **Status-specific messaging** with custom templates
- **Team mentions and escalations** based on environment
- **Rich notification content** with deployment details and actions

## Configuration

### server.yaml

```yaml
schemaVersion: 1.0

cicd:
  type: github-actions
  config:
    organization: "my-company"
    
    environments:
      staging:
        type: staging
        auto-deploy: true
        variables:
          NOTIFICATION_LEVEL: "standard"
      production:
        type: production
        protection: true
        variables:
          NOTIFICATION_LEVEL: "critical"
    
    # Comprehensive notifications
    notifications:
      slack: "${secret:slack-webhook-general}"
      discord: "${secret:discord-webhook-main}"
      telegram-chat-id: "${secret:telegram-main-chat}"
      telegram-token: "${secret:telegram-bot-token}"
      
      # Team-specific channels
      channels:
        dev-team-slack: "${secret:slack-webhook-dev-team}"
        devops-slack: "${secret:slack-webhook-devops}"
        security-slack: "${secret:slack-webhook-security}"
        management-email: "${secret:management-email-list}"
    
    # Custom templates
    templates:
      success-detailed:
        title: "✅ Deployment Successful"
        color: "good"
        fields:
          - name: "Environment"
            value: "${env:ENVIRONMENT}"
          - name: "Version"
            value: "${env:GIT_SHA}"
          - name: "Duration"
            value: "${deployment:duration}"
        actions:
          - name: "View App"
            url: "${deployment:app-url}"
          - name: "Monitoring"
            url: "${monitoring:dashboard-url}"
            
      failure-critical:
        title: "🚨 CRITICAL: Production Deployment Failed"
        color: "danger"
        urgency: "high"
        fields:
          - name: "Error Type"
            value: "${error:type}"
          - name: "Failed Step"  
            value: "${deployment:failed-step}"
          - name: "Impact"
            value: "${deployment:impact-assessment}"
        actions:
          - name: "Emergency Response"
            url: "${incident:response-url}"
          - name: "Rollback"
            url: "${deployment:rollback-url}"
```

## GitHub Actions Workflow

### Enhanced Deployment with Notifications

```yaml
# .github/workflows/deploy-with-notifications.yml
name: Deploy with Advanced Notifications
on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      environment:
        type: choice
        options: ['staging', 'production']

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment || 'staging' }}
    steps:
      - name: Deploy Application with Notifications
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: notification-app
          environment: ${{ github.event.inputs.environment || 'staging' }}
          sc-config: ${{ secrets.SC_CONFIG }}
          # Built-in notifications automatically configured via SC secrets
```

**Note**: The `deploy-client-stack` action includes built-in notification support that automatically sends notifications to the configured channels (Slack, Discord, Telegram) on deployment success or failure. No separate notification steps are required.

## Advanced Notification Templates

The examples above use Simple Container's self-contained GitHub Action. Here are the notification templates that would be sent:

## Multi-Environment Configuration

For production environments, you can configure additional notification channels in your secrets:
```

## Notification Templates

### Slack Success Template
```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "✅ Deployment Successful"
      }
    },
    {
      "type": "section",
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Environment:*\n${env:ENVIRONMENT}"
        },
        {
          "type": "mrkdwn", 
          "text": "*Version:*\n`${env:GIT_SHA:0:8}`"
        }
      ]
    },
    {
      "type": "actions",
      "elements": [
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "View App"},
          "url": "${deployment:app-url}",
          "style": "primary"
        },
        {
          "type": "button", 
          "text": {"type": "plain_text", "text": "View Logs"},
          "url": "${deployment:logs-url}"
        }
      ]
    }
  ]
}
```

### Slack Failure Template
```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "🚨 Deployment Failed"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "<!channel> Deployment to *${env:ENVIRONMENT}* has failed."
      }
    },
    {
      "type": "section",
      "fields": [
        {
          "type": "mrkdwn",
          "text": "*Environment:*\n${env:ENVIRONMENT}"
        },
        {
          "type": "mrkdwn",
          "text": "*Error:*\n${error:message}"
        }
      ]
    },
    {
      "type": "actions",
      "elements": [
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "🔍 View Logs"},
          "url": "${deployment:logs-url}",
          "style": "danger"
        },
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "🔄 Retry"},
          "url": "${deployment:retry-url}",
          "style": "primary"
        }
      ]
    }
  ]
}
```

## Setup Instructions

### 1. GitHub Secrets

Configure notification webhooks:
- `SLACK_WEBHOOK_GENERAL` - Main Slack channel
- `SLACK_WEBHOOK_DEV_TEAM` - Dev team channel  
- `SLACK_WEBHOOK_DEVOPS` - DevOps team channel
- `DISCORD_WEBHOOK_MAIN` - Main Discord channel
- `TELEGRAM_BOT_TOKEN` - Telegram bot token
- `TELEGRAM_MAIN_CHAT` - Telegram chat ID
- `DEVOPS_EMAIL_LIST` - DevOps email list
- `MANAGEMENT_EMAIL_LIST` - Management emails

### 2. Webhook Setup

**Slack:**
1. Create Slack app and enable webhooks
2. Generate webhook URLs for different channels
3. Configure permissions for mentions

**Discord:**
1. Create webhook in Discord channel settings
2. Copy webhook URL to GitHub secrets
3. Test webhook with sample message

**Telegram:**
1. Create bot via @BotFather
2. Get bot token and add to secrets
3. Get chat ID and configure permissions

## Advanced Features

### Environment-Specific Routing
- **Staging failures** → Dev team Slack only
- **Production failures** → All channels + email + escalation
- **Security issues** → Security team + management
- **Performance warnings** → Dev team + monitoring alerts

### Time-Based Escalation
- Immediate notification to primary channels
- 15-minute delay before management escalation
- 1-hour delay before executive escalation
- Auto-escalation for unresolved critical issues

### Rich Content
- Deployment URLs and quick actions
- Error details and suggested fixes
- Performance metrics and health checks
- Integration with monitoring dashboards

## Next Steps

After setting up advanced notifications:
- **[Basic Setup](../basic-setup/)** - Simple notification patterns
- **[Multi-Stack Deployment](../multi-stack/)** - Complex deployment notifications  
- **[Preview Deployments](../preview-deployments/)** - PR-based notifications

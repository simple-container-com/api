---
title: AWS Setup
description: AWS-specific setup guide for Simple Container
platform: platform
product: simple-container
category: skills
subcategory: cloud-provider
date: '2026-03-29'
---

# AWS Setup Skill

This skill guides you through setting up AWS credentials and resources for Simple Container. Follow these steps to configure AWS authentication and create required resources.

## Prerequisites

- AWS account with appropriate permissions
- AWS CLI installed (`aws --version`)
- SC CLI installed (see [Installation](../installation.md))

## Steps

### Step 1: Install and Configure AWS CLI

If you haven't already, install the AWS CLI:

```bash
# Install AWS CLI (Linux/macOS)
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip
sudo ./aws/install

# Verify installation
aws --version
```

Configure AWS CLI with your credentials:

```bash
# Configure AWS CLI
aws configure

# Enter when prompted:
# AWS Access Key ID: [Your Access Key ID]
# AWS Secret Access Key: [ Your Secret Access Key]
# Default region name: us-east-1
# Default output format: json
```

### Step 2: Obtain AWS Credentials

#### Option A: Using AWS Console

1. Go to [AWS Console](https://console.aws.amazon.com/)
2. Navigate to IAM > Users > [Your User]
3. Click "Security credentials" tab
4. Click "Create access key"
5. Note down the Access Key ID and Secret Access Key

#### Option B: Using AWS CLI

```bash
# If you already have an IAM user, create an access key
aws iam create-access-key --user-name your-iam-username

# Get account ID
aws sts get-caller-identity --query 'Account'

# Get current region
aws configure get region
```

### Step 3: Verify Credentials

```bash
# Test AWS credentials
aws sts get-caller-identity

# Expected output:
# {
#     "UserId": "AIDAI...",
#     "Account": "123456789012",
#     "Arn": "arn:aws:iam::123456789012:user/username"
# }
```

### Step 4: Set Required Permissions

Your IAM user needs these permissions for SC:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecr:*",
        "ecs:*",
        "s3:*",
        "rds:*",
        "elasticache:*",
        "lambda:*",
        "iam:*",
        "route53:*",
        "acm:*",
        "cloudfront:*"
      ],
      "Resource": "*"
    }
  ]
}
```

Attach the policy:

```bash
# Create policy
aws iam create-policy --policy-name SCPermissions \
  --policy-document file://policy.json

# Attach to user
aws iam attach-user-policy --user-name your-username \
  --policy-arn arn:aws:iam::123456789012:policy/SCPermissions
```

### Step 5: Create ECR Repository

Create an ECR repository for your container images:

```bash
# Create ECR repository
aws ecr create-repository --repository-name myproject/api

# Get registry URL
aws ecr describe-repositories --repository-names myproject/api

# Output includes: 123456789012.dkr.ecr.us-east-1.amazonaws.com/myproject/api
```

### Step 6: Configure Environment Variables

For SC to use your AWS credentials, set these environment variables:

```bash
export AWS_ACCESS_KEY_ID="AKIAIOSFODNN7EXAMPLE"
export AWS_SECRET_ACCESS_KEY="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
export AWS_ACCOUNT_ID="123456789012"
export AWS_REGION="us-east-1"
```

### Step 7: Verify Setup

Verify your AWS setup works with SC:

```bash
# Check AWS configuration
sc config list

# Test authentication
sc auth validate --provider aws
```

## Environment Variables for AWS

| Variable | Description | Required |
|----------|-------------|----------|
| `AWS_ACCESS_KEY_ID` | AWS access key | Yes |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | Yes |
| `AWS_ACCOUNT_ID` | AWS account ID | Yes |
| `AWS_REGION` | Default AWS region | Yes |
| `AWS_SESSION_TOKEN` | Session token (for temp credentials) | No |

## Example: Full AWS server.yaml

```yaml
schemaVersion: 1.0

project: myproject
name: devops

provider:
  name: aws
  region: ${AWS_REGION}
  accountId: ${AWS_ACCOUNT_ID}

auth:
  - name: aws-main
    provider: aws
    config:
      accessKeyId: ${AWS_ACCESS_KEY_ID}
      secretAccessKey: ${AWS_SECRET_ACCESS_KEY}

resources:
  - name: postgres-main
    type: aws:rds:postgres
    config:
      instanceClass: db.t3.micro
      allocatedStorage: 20

  - name: s3-assets
    type: aws:s3:bucket
    config:
      publicAccess: false
```

## Common Issues

### "The security token included in the request is invalid"

Your credentials have expired. Refresh them:
```bash
aws configure
```

### "ECR image repository not found"

Create the repository:
```bash
aws ecr create-repository --repository-name your-repo
```

### "ECS task execution role not found"

Create the role:
```bash
aws iam create-role --role-name ecsTaskExecutionRole \
  --assume-role-policy-document file://(echo '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}')
```

## Next Steps

After AWS setup:

1. [DevOps Setup](../devops-setup.md) - Create server.yaml with AWS resources
2. [Service Setup](../service-setup.md) - Configure your service deployment
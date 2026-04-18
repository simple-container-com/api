# PR: Add consistent AWS tags to all resources

## Title

fix: add `simple-container.com/*` tags to all AWS resources

## Body

## Summary

- Add standard `simple-container.com/stack` and `simple-container.com/env` tags to **every** AWS resource that supports tagging — previously only a subset of ECS Fargate resources were tagged
- Thread `StackParams`-derived tags through all resource provisioning functions (secrets, ECR, Lambda, RDS, S3, static egress, ECS tasks, alerts)
- Skip resources where the AWS API does not support tags (e.g. `RolePolicyAttachment`, `SecretVersion`, `BucketPolicy`, `LifecyclePolicy`)

## Motivation

Tags were missing on Secrets Manager secrets, ECR repositories, and many other AWS resources (RDS instances, Lambda functions, S3 buckets, VPC networking, etc.). This made it difficult to:
- Track costs per stack/environment in AWS Cost Explorer
- Filter resources in the AWS console
- Implement automated governance and cleanup policies

## Changes by file

| File | What changed |
|------|-------------|
| `secrets.go` | Added `tags` param to `createSecret()`, applied to `SecretArgs.Tags` |
| `ecr_repository.go` | Build tags from `deployParams`, applied to `RepositoryArgs.Tags` |
| `ecs_fargate.go` | Merged stack tags into `FargateService.Tags` (alongside `deployTime`), pass tags to `attachAutoScalingPolicy` and `createEcsAlerts`, pass `StackParams` to static egress |
| `aws_lambda.go` | Build tags from `deployParams`, applied to IAM role, IAM policy, LogGroup, Lambda function, API Gateway, Stage, EventRule; pass tags to `createSecret` and `provisionScheduleForLambda`; pass `StackParams` to static egress |
| `alerts.go` | Added `tags` field to `alertCfg`, applied to IAM role, IAM policy, Lambda, MetricAlarm; added `tags` param to `createSNSTopicForAlerts`, applied to SNS Topic |
| `rds_postgres.go` | Build tags, applied to SecurityGroup, SubnetGroup, RDS Instance |
| `rds_mysql.go` | Build tags, applied to SecurityGroup, SubnetGroup, RDS Instance |
| `static_website.go` | Added `Tags` to `StaticSiteInput`, applied to both S3 buckets |
| `bucket.go` | Added `Tags` to `S3BucketInput`, applied to S3 bucket and IAM user |
| `static_egress.go` | Build tags in single-zone `provisionVpcWithStaticEgress`, applied to VPC, subnets, IGW, route tables, EIP, NAT gateway, security group |
| `exec_ecs_task.go` | Added `tags` to `ecsTaskConfig`, applied to IAM role, task definition, ECS cluster, security group, IAM policy |
| `compute_proc.go` | Pass tags via `ecsTaskConfig` for postgres and mysql init tasks |

## Resources intentionally NOT tagged (AWS does not support tags)

`SecretVersion`, `LifecyclePolicy`, `BackupPolicy`, `MountTarget`, `RolePolicyAttachment`, `BucketPublicAccessBlock`, `BucketOwnershipControls`, `BucketPolicy`, `BucketCorsConfigurationV2`, `Dashboard`, `EventTarget`, `Permission`, `FunctionUrl`, `Integration`, `Route`, `TopicSubscription`, `RouteTableAssociation`, `autoscaling.Policy`

## Test plan

- [x] `go build ./...` passes
- [x] `welder run fmt` passes (gofumpt + golangci-lint clean)
- [ ] Deploy a stack to AWS and verify tags appear on:
  - Secrets Manager secrets
  - ECR repositories
  - ECS Fargate service and cluster
  - RDS instances
  - Lambda functions
  - S3 buckets
  - VPC/subnet/IGW/NAT resources
  - CloudWatch alarms and log groups
  - SNS topics
  - API Gateway
- [ ] Verify existing deployments are not disrupted (tags are additive, no resource replacement expected)

# AWS Tags on All Resources

## Problem
AWS resources created during deployment (Secrets Manager secrets, ECR repositories, RDS instances, Lambda functions, S3 buckets, etc.) were missing the standard `simple-container.com/*` tags. Only a subset of resources in `ecs_fargate.go` had tags applied.

## Root Cause
The tagging utility (`BuildTagsFromStackParams().ToAWSTags()`) was only called in `createEcsFargateCluster()`, and the resulting tags were not passed to helper functions like `createSecret()`, `createEcrRegistry()`, or used in other resource provisioning files.

## Changes

### Core signature changes
- **`createSecret()`** (`secrets.go`): Added `tags sdk.StringMap` parameter, applied to `secretsmanager.SecretArgs.Tags`
- **`createSNSTopicForAlerts()`** (`alerts.go`): Added `tags sdk.StringMap` parameter, applied to `sns.TopicArgs.Tags`
- **`provisionScheduleForLambda()`** (`aws_lambda.go`): Added `tags sdk.StringMap` parameter, applied to `cloudwatch.EventRuleArgs.Tags`
- **`attachAutoScalingPolicy()`** (`ecs_fargate.go`): Added `tags sdk.StringMap` parameter, applied to `appautoscaling.TargetArgs.Tags`
- **`alertCfg`** struct (`alerts.go`): Added `tags sdk.StringMap` field
- **`ecsTaskConfig`** struct (`exec_ecs_task.go`): Added `tags sdk.StringMap` field
- **`StaticSiteInput`** struct (`static_website.go`): Added `Tags sdk.StringMap` field
- **`S3BucketInput`** struct (`bucket.go`): Added `Tags sdk.StringMap` field

### Files modified with tags added to resources

| File | Resources tagged |
|------|-----------------|
| `secrets.go` | `secretsmanager.NewSecret` |
| `ecr_repository.go` | `ecr.NewRepository` |
| `ecs_fargate.go` | `ecs.NewFargateService` (merged with existing deployTime tag), `appautoscaling.NewTarget`, `StaticEgressIPIn.StackParams` |
| `aws_lambda.go` | `iam.NewRole`, `iam.NewPolicy`, `cloudwatch.NewLogGroup`, `lambda.FunctionArgs`, `apigatewayv2.NewApi`, `apigatewayv2.NewStage`, `cloudwatch.NewEventRule`, `StaticEgressIPIn.StackParams` |
| `alerts.go` | `iam.NewRole`, `iam.NewPolicy`, `lambda.NewFunction`, `cloudwatch.MetricAlarmArgs`, `sns.NewTopic` |
| `rds_postgres.go` | `ec2.NewSecurityGroup`, `rds.NewSubnetGroup`, `rds.NewInstance` |
| `rds_mysql.go` | `ec2.NewSecurityGroup`, `rds.NewSubnetGroup`, `rds.NewInstance` |
| `static_website.go` | `s3.NewBucket` (main + www redirect) |
| `bucket.go` | `s3.NewBucket`, `iam.NewUser` |
| `static_egress.go` | `ec2.NewVpc`, `ec2.NewSubnet` (x2), `ec2.NewInternetGateway`, `ec2.NewRouteTable` (x2), `ec2.NewEip`, `ec2.NewNatGateway`, `ec2.NewSecurityGroup` |
| `exec_ecs_task.go` | `iam.NewRole`, `ecs.NewTaskDefinition`, `ecs.NewCluster`, `ec2.NewSecurityGroup`, `iam.NewPolicy` |
| `compute_proc.go` | Passes tags via `ecsTaskConfig` for both postgres and mysql init tasks |

### Resources NOT tagged (AWS API does not support tags on these)
- `secretsmanager.NewSecretVersion`
- `ecr.NewLifecyclePolicy`
- `efs.NewBackupPolicy`, `efs.NewMountTarget`
- `iam.NewRolePolicyAttachment`
- `appautoscaling.NewPolicy`
- `s3.NewBucketPublicAccessBlock`, `s3.NewBucketOwnershipControls`, `s3.NewBucketPolicy`, `s3.NewBucketCorsConfigurationV2`
- `cloudwatch.NewDashboard`, `cloudwatch.NewEventTarget`
- `lambda.NewPermission`, `lambda.NewFunctionUrl`
- `apigatewayv2.NewIntegration`, `apigatewayv2.NewRoute`
- `sns.NewTopicSubscription`
- `ec2.NewRouteTableAssociation`

## Tags applied
All tagged resources receive:
- `simple-container.com/stack` — stack name
- `simple-container.com/env` — environment name
- `simple-container.com/parent-stack` — (optional) parent stack
- `simple-container.com/client-stack` — (optional) client stack

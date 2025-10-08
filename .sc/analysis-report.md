# Simple Container Project Analysis Report

**Generated:** 2025-10-08 17:47:42 +03
**Analyzer Version:** 1.0
**Overall Confidence:** 68.3%

## Project Overview

- **Name:** .
- **Path:** .
- **Architecture:** standard-web-app
- **Primary Technology:** go gorilla-mux (95.0% confidence)

## Technology Stacks

### 1. go gorilla-mux

- **Confidence:** 95.0%
- **Runtime:** go
- **Version:** 1.24.0
- **Evidence:**
  - go.mod found
  - gorilla-mux framework detected
- **Additional Information:**
  - module: github.com/simple-container-com/api
  - mode: modules

### 2. yaml simple-container

- **Confidence:** 90.0%
- **Runtime:** simple-container
- **Version:** configured
- **Evidence:**
  - .sc directory found
  - welder.yaml found
  - simple-container reference in go.mod
  - SC CLI usage in branch.yaml
  - SC CLI usage in push.yaml
- **Additional Information:**
  - has_sc_directory: true
  - has_welder_config: true
  - maturity: full

### 3. yaml pulumi

- **Confidence:** 20.0%
- **Runtime:** pulumi
- **Version:** detected
- **Evidence:**
  - Pulumi SDK in go.mod
- **Additional Information:**

## Git Repository Analysis

- **Branch:** feature/ai-setup
- **Remote URL:** git@github.com:simple-container-com/api.git
- **Contributors:** 0
- **Has CI/CD:** false

## Detected Resources

### Databases

- **sqlite** (80.0% confidence)
  - Sources: pkg/assistant/analysis/recommendations.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Connection: sqlite
  - Recommended Resource: Consider upgrading to managed database for production
- **mongodb** (80.0% confidence)
  - Sources: cmd/schema-gen/main.go, docs/docs/examples/ecs-deployments/backend-service/client.yaml, .sc/secrets.yaml, docs/docs/examples/ecs-deployments/blockchain-service/client.yaml, docs/docs/examples/ecs-deployments/meteor-app/client.yaml, docs/docs/examples/ecs-deployments/meteor-app/docker-compose.yaml, docs/docs/examples/gke-autopilot/comprehensive-setup/server.yaml, docs/docs/examples/lambda-functions/billing-system/client.yaml, docs/docs/examples/lambda-functions/cost-analytics/client.yaml, docs/docs/examples/lambda-functions/scheduler/client.yaml, docs/docs/examples/lambda-functions/storage-service/client.yaml, docs/docs/examples/parent-stacks/aws-multi-region/server.yaml, docs/docs/examples/secrets/aws-mongodb-atlas/secrets.yaml, docs/docs/examples/secrets/gcp-auth-cloudflare-mongodb-discord-telegram/secrets.yaml, cmd/generate-embeddings/main.go, docs/schemas/index.json, docs/schemas/kubernetes/helmmongodboperator.json, docs/schemas/kubernetes/index.json, docs/schemas/mongodb/index.json, docs/schemas/mongodb/atlasconfig.json, pkg/api/git/testdata/repo/stacks/common/secrets.yaml, pkg/api/git/testdata/repo/stacks/refapp/secrets.yaml, pkg/api/secrets/testdata/repo/stacks/common/secrets.yaml, pkg/api/secrets/testdata/repo/stacks/refapp/secrets.yaml, pkg/api/tests/refapp.go, pkg/api/tests/testdata/stacks/common/secrets.yaml, pkg/api/tests/testdata/stacks/refapp/client.yaml, pkg/api/tests/testdata/stacks/refapp/docker-compose.yaml, pkg/api/tests/testdata/stacks/refapp-aws/client.yaml, pkg/api/tests/testdata/stacks/refapp-ansible-k3s/secrets.yaml, pkg/api/tests/testdata/stacks/refapp/server.yaml, pkg/api/tests/testdata/stacks/refapp-aws/docker-compose.yaml, pkg/api/tests/testdata/stacks/refapp-gke-autopilot/client.yaml, pkg/api/tests/testdata/stacks/refapp-gke-autopilot/docker-compose.yaml, pkg/api/tests/testdata/stacks/refapp-kubernetes/client.yaml, pkg/api/tests/testdata/stacks/refapp-kubernetes/docker-compose.yaml, pkg/assistant/analysis/analyzer_test.go, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/recommendations.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/interface.go, pkg/assistant/chat/commands.go, pkg/assistant/embeddings/docs/examples/ecs-deployments/backend-service/client.yaml, pkg/assistant/embeddings/docs/examples/ecs-deployments/blockchain-service/client.yaml, pkg/assistant/embeddings/docs/examples/ecs-deployments/meteor-app/client.yaml, pkg/assistant/embeddings/docs/examples/ecs-deployments/meteor-app/docker-compose.yaml, pkg/assistant/embeddings/docs/examples/gke-autopilot/comprehensive-setup/server.yaml, pkg/assistant/embeddings/docs/examples/lambda-functions/billing-system/client.yaml, pkg/assistant/embeddings/docs/examples/lambda-functions/cost-analytics/client.yaml, pkg/assistant/embeddings/docs/examples/lambda-functions/scheduler/client.yaml, pkg/assistant/embeddings/docs/examples/parent-stacks/aws-multi-region/server.yaml, pkg/assistant/embeddings/embeddings.go, pkg/assistant/embeddings/docs/examples/lambda-functions/storage-service/client.yaml, pkg/assistant/llm/prompts/system.go, pkg/assistant/mcp/protocol.go, pkg/assistant/mcp/schemas/index.json, pkg/assistant/mcp/schemas/kubernetes/helmmongodboperator.json, pkg/assistant/mcp/schemas/kubernetes/index.json, pkg/assistant/mcp/schemas/mongodb/index.json, pkg/assistant/mcp/schemas/mongodb/atlasconfig.json, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/mcp/server.go, pkg/assistant/modes/schemas/index.json, pkg/assistant/modes/schemas/kubernetes/helmmongodboperator.json, pkg/assistant/modes/schemas/kubernetes/index.json, pkg/assistant/modes/schemas/mongodb/index.json, pkg/assistant/modes/schemas/mongodb/atlasconfig.json, pkg/assistant/resources/matcher.go, pkg/clouds/k8s/init.go, pkg/clouds/k8s/postgres.go, pkg/clouds/mongodb/init.go, pkg/clouds/mongodb/mongodb.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/clouds/pulumi/e2e_compose_test.go, pkg/clouds/pulumi/init.go, pkg/clouds/pulumi/kubernetes/helm_operator_mongodb.go, pkg/clouds/pulumi/kubernetes/compute_proc_mongodb.go, pkg/clouds/pulumi/kubernetes/init.go, pkg/clouds/pulumi/kubernetes/init_mongo_user_job.go, pkg/clouds/pulumi/mongodb/compute_proc.go, pkg/clouds/pulumi/mongodb/init.go, pkg/clouds/pulumi/mongodb/provider.go, pkg/clouds/pulumi/mongodb/util.go, pkg/clouds/pulumi/mongodb/util_test.go, pkg/clouds/pulumi/mongodb/cluster.go, pkg/clouds/pulumi/testutil/secrets_test_util.go, pkg/provisioner/init.go, pkg/provisioner/placeholders/tests/placeholders_test.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Connection: mongodb
  - Recommended Resource: mongodb-atlas
- **mysql** (80.0% confidence)
  - Sources: docs/docs/examples/ecs-deployments/blog-platform/docker-compose.yaml, docs/schemas/aws/index.json, docs/docs/examples/ecs-deployments/blog-platform/client.yaml, cmd/generate-embeddings/main.go, docs/schemas/aws/mysqlconfig.json, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/recommendations.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/docs/examples/ecs-deployments/blog-platform/client.yaml, pkg/assistant/embeddings/docs/examples/ecs-deployments/blog-platform/docker-compose.yaml, pkg/assistant/embeddings/embeddings.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/mcp/schemas/aws/index.json, pkg/assistant/mcp/schemas/aws/mysqlconfig.json, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/modes/schemas/aws/index.json, pkg/assistant/modes/schemas/aws/mysqlconfig.json, pkg/assistant/mcp/server.go, pkg/assistant/resources/matcher.go, pkg/clouds/aws/init.go, pkg/clouds/aws/rds_mysql.go, pkg/clouds/pulumi/aws/compute_proc.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/clouds/pulumi/aws/init.go, pkg/clouds/pulumi/aws/rds_mysql.go, pkg/clouds/pulumi/gcp/init_pg_user_job.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Connection: mysql
  - Recommended Resource: aws-rds-mysql
- **redis** (80.0% confidence)
  - Sources: docs/docs/examples/gke-autopilot/comprehensive-setup/server.yaml, cmd/generate-embeddings/main.go, docs/schemas/gcp/redisconfig.json, docs/schemas/gcp/index.json, docs/schemas/kubernetes/helmredisoperator.json, docs/schemas/kubernetes/index.json, pkg/assistant/analysis/analyzer_test.go, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/recommendations.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/interface.go, pkg/assistant/chat/commands.go, pkg/assistant/embeddings/docs/examples/gke-autopilot/comprehensive-setup/server.yaml, pkg/assistant/embeddings/embeddings.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/mcp/protocol.go, pkg/assistant/mcp/schemas/gcp/index.json, pkg/assistant/mcp/schemas/gcp/redisconfig.json, pkg/assistant/mcp/schemas/kubernetes/helmredisoperator.json, pkg/assistant/mcp/schemas/kubernetes/index.json, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/mcp/server.go, pkg/assistant/modes/schemas/gcp/index.json, pkg/assistant/modes/schemas/gcp/redisconfig.json, pkg/assistant/modes/schemas/kubernetes/helmredisoperator.json, pkg/assistant/modes/schemas/kubernetes/index.json, pkg/assistant/resources/matcher.go, pkg/clouds/gcloud/init.go, pkg/clouds/gcloud/redis.go, pkg/clouds/k8s/init.go, pkg/clouds/k8s/postgres.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/clouds/pulumi/gcp/init.go, pkg/clouds/pulumi/gcp/redis.go, pkg/clouds/pulumi/kubernetes/compute_proc_redis.go, pkg/clouds/pulumi/kubernetes/helm_operator_redis.go, pkg/clouds/pulumi/kubernetes/init.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Connection: redis
  - Recommended Resource: gcp-redis or kubernetes-helm-redis-operator
- **postgresql** (90.0% confidence)
  - Sources: docs/docs/examples/kubernetes-native/streaming-platform/docker-compose.yaml, docs/schemas/aws/index.json, docs/docs/examples/kubernetes-native/streaming-platform/client.yaml, cmd/generate-embeddings/main.go, docs/schemas/aws/postgresconfig.json, docs/schemas/gcp/postgresgcpcloudsqlconfig.json, docs/schemas/gcp/index.json, docs/schemas/kubernetes/index.json, docs/schemas/kubernetes/helmpostgresoperator.json, pkg/api/tests/refapp.go, pkg/api/tests/testdata/stacks/refapp/server.yaml, pkg/assistant/analysis/analyzer_test.go, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/recommendations.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/interface.go, pkg/assistant/chat/commands.go, pkg/assistant/embeddings/docs/examples/kubernetes-native/streaming-platform/client.yaml, pkg/assistant/embeddings/docs/examples/kubernetes-native/streaming-platform/docker-compose.yaml, pkg/assistant/embeddings/embeddings.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/mcp/protocol.go, pkg/assistant/mcp/schemas/aws/index.json, pkg/assistant/mcp/schemas/aws/postgresconfig.json, pkg/assistant/mcp/schemas/gcp/postgresgcpcloudsqlconfig.json, pkg/assistant/mcp/schemas/gcp/index.json, pkg/assistant/mcp/schemas/kubernetes/helmpostgresoperator.json, pkg/assistant/mcp/schemas/kubernetes/index.json, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/modes/schemas/aws/index.json, pkg/assistant/modes/schemas/aws/postgresconfig.json, pkg/assistant/mcp/server.go, pkg/assistant/modes/schemas/gcp/postgresgcpcloudsqlconfig.json, pkg/assistant/modes/schemas/gcp/index.json, pkg/assistant/modes/schemas/kubernetes/helmpostgresoperator.json, pkg/assistant/modes/schemas/kubernetes/index.json, pkg/assistant/resources/matcher.go, pkg/clouds/aws/init.go, pkg/clouds/aws/rds_postgres.go, pkg/clouds/gcloud/init.go, pkg/clouds/gcloud/postgres.go, pkg/clouds/k8s/init.go, pkg/clouds/k8s/postgres.go, pkg/clouds/pulumi/aws/compute_proc.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/clouds/pulumi/aws/init.go, pkg/clouds/pulumi/aws/rds_mysql.go, pkg/clouds/pulumi/aws/rds_postgres.go, pkg/clouds/pulumi/db/constants.go, pkg/clouds/pulumi/gcp/cloudsql_proxy.go, pkg/clouds/pulumi/gcp/init.go, pkg/clouds/pulumi/gcp/init_pg_user_job.go, pkg/clouds/pulumi/gcp/postgres.go, pkg/clouds/pulumi/gcp/compute_proc.go, pkg/clouds/pulumi/kubernetes/compute_proc_postgres.go, pkg/clouds/pulumi/kubernetes/helm_operator_postgres.go, pkg/clouds/pulumi/kubernetes/init.go, pkg/clouds/pulumi/kubernetes/init_pg_user_job.go, pkg/clouds/pulumi/kubernetes/helpers.go, pkg/cmd/cmd_assistant/assistant.go, pkg/provisioner/placeholders/tests/placeholders_test.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Recommended Resource: aws-rds-postgres or gcp-cloudsql-postgres or kubernetes-helm-postgres-operator
- **elasticsearch** (80.0% confidence)
  - Sources: cmd/generate-embeddings/main.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/commands.go, pkg/assistant/embeddings/embeddings.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/clouds/pulumi/aws/exec_ecs_task.go, pkg/clouds/pulumi/aws/ecs_fargate.go, pkg/clouds/pulumi/aws/static_egress.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Recommended Resource: Consider managed Elasticsearch service

### External APIs

- **aws_ses** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: email
- **huggingface** (80.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: ai
- **openai** (90.0% confidence)
  - Sources: .env.example, .github/workflows/branch.yaml, .github/workflows/push.yaml, cmd/generate-embeddings/main.go, pkg/assistant/analysis/detector.go, pkg/assistant/chat/types.go, pkg/assistant/config/config.go, pkg/assistant/chat/interface.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/commands.go, pkg/assistant/llm/deepseek.go, pkg/assistant/embeddings/embeddings.go, pkg/assistant/llm/ollama.go, pkg/assistant/llm/openai.go, pkg/assistant/llm/provider_test.go, pkg/assistant/llm/provider.go, pkg/assistant/llm/yandex.go, pkg/assistant/modes/developer.go, pkg/cmd/cmd_assistant/assistant.go, welder.yaml, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: ai
- **google_maps** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: maps
- **supabase** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: backend_service
- **square** (80.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: payment
- **mapbox** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: maps
- **anthropic** (90.0% confidence)
  - Sources: docs/docs/examples/advanced-configs/high-resource/client.yaml, docs/docs/examples/advanced-configs/high-resource/docker-compose.yaml, docs/docs/examples/lambda-functions/ai-gateway/client.yaml, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/docs/examples/advanced-configs/high-resource/client.yaml, pkg/assistant/embeddings/docs/examples/advanced-configs/high-resource/docker-compose.yaml, pkg/assistant/embeddings/docs/examples/lambda-functions/ai-gateway/client.yaml, pkg/assistant/llm/anthropic.go, pkg/assistant/llm/provider_test.go, pkg/assistant/llm/provider.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: ai
- **auth0** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: authentication
- **twilio** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: communication
- **mixpanel** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: analytics
- **algolia** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: search
- **discord** (80.0% confidence)
  - Sources: docs/docs/examples/secrets/aws-mongodb-atlas/secrets.yaml, docs/docs/examples/secrets/gcp-auth-cloudflare-mongodb-discord-telegram/secrets.yaml, pkg/assistant/analysis/resource_detectors.go, pkg/clouds/aws/helpers/ch_cloudwatch_alert.go, pkg/clouds/pulumi/aws/alerts.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: communication
- **stripe** (90.0% confidence)
  - Sources: pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: payment
- **sendgrid** (90.0% confidence)
  - Sources: pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go
  - Purpose: email
- **slack** (80.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go, pkg/clouds/aws/helpers/ch_cloudwatch_alert.go, pkg/clouds/pulumi/aws/alerts.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: communication
- **google_analytics** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: analytics
- **firebase** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: backend_service
- **mailgun** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: email
- **amplitude** (80.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: analytics
- **paypal** (90.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
  - Purpose: payment

### Storage

- **cdn** (80.0% confidence)
  - Sources: cmd/generate-embeddings/main.go, pkg/assistant/analysis/recommendations.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/embeddings.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: static
- **static_assets** (50.0% confidence)
  - Sources: pkg/assistant/analysis/architecture_analysis.go, pkg/assistant/analysis/recommendations.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/commands.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/mcp/server.go, pkg/assistant/modes/developer.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: static
- **azure_blob** (100.0% confidence)
  - Sources: pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/modes/developer.go, pkg/assistant/resources/matcher.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: cloud_storage
- **file_upload** (70.0% confidence)
  - Sources: pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/commands.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/modes/devops.go, pkg/clouds/pulumi/aws/bucket.go, pkg/clouds/pulumi/aws/static_website.go, pkg/clouds/pulumi/gcp/bucket_uploader.go, pkg/clouds/pulumi/gcp/static_website.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: uploads
- **gcs** (100.0% confidence)
  - Sources: caddy.Dockerfile, .sc/secrets.yaml, docs/schemas/gcp/index.json, docs/schemas/gcp/statestorageconfig.json, docs/docs/examples/parent-stacks/aws-multi-region/server.yaml, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/docs/examples/parent-stacks/aws-multi-region/server.yaml, pkg/assistant/mcp/schemas/gcp/index.json, pkg/assistant/mcp/schemas/gcp/statestorageconfig.json, pkg/assistant/mcp/server.go, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/modes/schemas/gcp/index.json, pkg/assistant/modes/schemas/gcp/statestorageconfig.json, pkg/assistant/resources/matcher.go, pkg/clouds/gcloud/auth.go, pkg/clouds/pulumi/gcp/bucket_uploader.go, pkg/clouds/pulumi/gcp/provider.go, pkg/clouds/pulumi/gcp/static_website.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: cloud_storage
- **s3** (100.0% confidence)
  - Sources: .sc/secrets.yaml, cmd/schema-gen/main.go, cmd/generate-embeddings/main.go, docs/schemas/aws/index.json, docs/schemas/aws/s3bucket.json, docs/schemas/aws/statestorageconfig.json, docs/docs/examples/parent-stacks/aws-multi-region/server.yaml, pkg/api/secrets/alias_deduplication_test.go, pkg/api/secrets/testdata/repo/.sc/cfg.local-key-inline.yaml, pkg/api/secrets/util_test.go, pkg/api/tests/testdata/stacks/common/server.yaml, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/chat/interface.go, pkg/assistant/chat/commands.go, pkg/assistant/embeddings/docs/examples/parent-stacks/aws-multi-region/server.yaml, pkg/assistant/embeddings/embeddings.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/mcp/mcp_test.go, pkg/assistant/mcp/schemas/aws/index.json, pkg/assistant/mcp/schemas/aws/statestorageconfig.json, pkg/assistant/mcp/schemas/aws/s3bucket.json, pkg/assistant/mcp/server.go, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/modes/schemas/aws/index.json, pkg/assistant/modes/schemas/aws/s3bucket.json, pkg/assistant/modes/schemas/aws/statestorageconfig.json, pkg/assistant/resources/matcher.go, pkg/clouds/aws/auth.go, pkg/clouds/aws/bucket.go, pkg/clouds/aws/init.go, pkg/clouds/pulumi/aws/bucket.go, pkg/clouds/pulumi/aws/compute_proc.go, pkg/clouds/pulumi/aws/init.go, pkg/clouds/pulumi/e2e_compose_test.go, pkg/clouds/pulumi/aws/static_website.go, pkg/clouds/pulumi/e2e_helpers_test.go, pkg/clouds/pulumi/pulumi.go, pkg/provisioner/testdata/.sc/cfg.default.yaml, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
  - Purpose: cloud_storage

### Message Queues

- **azure_servicebus** (100.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
- **nats** (80.0% confidence)
  - Sources: pkg/assistant/analysis/resource_detectors.go
- **gcp_pubsub** (100.0% confidence)
  - Sources: cmd/schema-gen/main.go, docs/schemas/gcp/index.json, docs/schemas/gcp/pubsubconfig.json, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/mcp/schemas/gcp/index.json, pkg/assistant/mcp/schemas/gcp/pubsubconfig.json, pkg/assistant/modes/schemas/gcp/pubsubconfig.json, pkg/assistant/modes/schemas/gcp/index.json, pkg/clouds/pulumi/gcp/init.go, pkg/clouds/pulumi/gcp/pubsub.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
- **aws_sqs** (100.0% confidence)
  - Sources: .sc/secrets.yaml, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go
- **kafka** (85.0% confidence)
  - Sources: cmd/generate-embeddings/main.go, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/embeddings/embeddings.go, pkg/assistant/modes/developer.go
- **rabbitmq** (85.0% confidence)
  - Sources: docs/schemas/kubernetes/helmrabbitmqoperator.json, docs/schemas/kubernetes/index.json, pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/llm/prompts/system.go, pkg/assistant/mcp/schemas/kubernetes/helmrabbitmqoperator.json, pkg/assistant/mcp/schemas/kubernetes/index.json, pkg/assistant/mcp/server.go, pkg/assistant/modes/developer.go, pkg/assistant/modes/devops.go, pkg/assistant/modes/schemas/kubernetes/helmrabbitmqoperator.json, pkg/assistant/modes/schemas/kubernetes/index.json, pkg/clouds/k8s/init.go, pkg/clouds/k8s/postgres.go, pkg/clouds/pulumi/kubernetes/compute_proc_rabbitmq.go, pkg/clouds/pulumi/kubernetes/helm_operator_rabbitmq.go, pkg/clouds/pulumi/kubernetes/init.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json
- **redis_pubsub** (80.0% confidence)
  - Sources: pkg/assistant/analysis/detector.go, pkg/assistant/analysis/resource_detectors.go, pkg/assistant/modes/developer.go, pkg/assistant/embeddings/vectors/prebuilt_embeddings_local.json, pkg/assistant/embeddings/vectors/prebuilt_embeddings_openai.json

### Environment Variables

- **Total:** 5 environment variables detected
- **Sources:**
  - .env.example: 5 variables

### Detected Secrets

- **Total:** 7 potential secrets detected
- **Types:**
  - aws_key: 2 instances
  - database_url: 3 instances
  - api_key: 2 instances

## Recommendations

### Critical Priority

**Secrets Management**
- Potential secrets detected in code. Move sensitive data to secure secrets management
- Action: secure_secrets

### High Priority

**Go Multi-stage Dockerfile**
- Generate optimized multi-stage Dockerfile for Go application with minimal final image
- Action: generate_dockerfile

**Add Dockerfile**
- Generate optimized Dockerfile for containerized deployment
- Action: generate_dockerfile

**CI/CD Pipeline Setup**
- No CI/CD detected. Set up automated testing and deployment pipeline for better development workflow
- Action: setup_cicd

### Medium Priority

**Go Build Optimization**
- Configure Go build with proper flags for smaller binaries and faster startup
- Action: optimize_go_build

**Simple Container Advanced Features**
- Explore advanced Simple Container features like multi-environment deployments and resource optimization
- Action: explore_advanced_features

**Version Tagging Strategy**
- No version tags detected. Implement semantic versioning for better release management
- Action: setup_versioning

**Database Architecture Review**
- Multiple databases detected. Review data architecture for potential consolidation opportunities
- Action: review_database_architecture

**API Management Strategy**
- Many external APIs detected. Consider API gateway for better management and monitoring
- Action: implement_api_gateway

### Low Priority

**Simple Container Configuration Review**
- Review current Simple Container configuration for optimization opportunities
- Action: review_configuration

## Simple Container Setup Guide

Based on this analysis, here's how to get started with Simple Container:

1. **Initialize Simple Container**
   ```bash
   sc init
   ```

2. **Configure for go gorilla-mux**
   - Simple Container will automatically detect your technology stack
   - Review the generated configuration files

3. **Deploy**
   ```bash
   sc deploy
   ```

For more information, visit: https://simple-container.com/docs

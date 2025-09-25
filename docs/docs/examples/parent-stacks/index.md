# Parent Stack Examples

This section contains examples of parent stack configurations that provide shared infrastructure for client applications.

## Available Examples

### AWS Multi-Region Parent Stack
Complete parent stack with Cloudflare DNS configuration and multi-region AWS setup.

**Use Case:** Global applications, disaster recovery, multi-region deployment

**Configuration:**
```yaml
# .sc/stacks/devops/server.yaml
schemaVersion: 1.0

templates:
  ecs-fargate-us:
    type: aws-ecs-fargate
    config: &aws-us-config
      credentials: "${auth:aws-us}"
      account: "${auth:aws-us.projectId}"
      region: us-east-1
      
  ecs-fargate-eu:
    type: aws-ecs-fargate
    config: &aws-eu-config
      credentials: "${auth:aws-eu}"
      account: "${auth:aws-eu.projectId}"
      region: eu-west-1

resources:
  resources:
    production:
      # US East resources
      s3-storage-us:
        type: aws-s3-bucket
        config:
          <<: *aws-us-config
          name: "myapp-prod-storage-us"
          allowOnlyHttps: true
          corsConfig:
            allowedOrigins: ["https://myapp.com"]
            allowedMethods: ["GET", "POST", "PUT"]
            
      rds-postgres-us:
        type: aws-rds-postgres
        config:
          <<: *aws-us-config
          name: "myapp-prod-db-us"
          instanceClass: "db.r5.xlarge"
          allocateStorage: 100
          engineVersion: "14.9"
          username: "appuser"
          password: "${secret:DB_PASSWORD_US}"
          databaseName: "myapp"
          
      # EU West resources
      s3-storage-eu:
        type: aws-s3-bucket
        config:
          <<: *aws-eu-config
          name: "myapp-prod-storage-eu"
          allowOnlyHttps: true
          corsConfig:
            allowedOrigins: ["https://myapp.com"]
            allowedMethods: ["GET", "POST", "PUT"]
            
      rds-postgres-eu:
        type: aws-rds-postgres
        config:
          <<: *aws-eu-config
          name: "myapp-prod-db-eu"
          instanceClass: "db.r5.xlarge"
          allocateStorage: 100
          engineVersion: "14.9"
          username: "appuser"
          password: "${secret:DB_PASSWORD_EU}"
          databaseName: "myapp"
          
      # MongoDB Atlas with multi-region
      mongodb-atlas:
        type: mongodb-atlas
        config:
          publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
          privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
          orgId: "${secret:MONGODB_ATLAS_ORG_ID}"
          projectId: "${secret:MONGODB_ATLAS_PROJECT_ID}"
          instanceSize: M30
          region: US_EAST_1
          cloudProvider: AWS
          admins: ["admin@mycompany.com"]
          developers: ["dev@mycompany.com"]
          backup:
            every: 6h
            retention: 168h
          networkConfig:
            allowCidrs: ["10.0.0.0/8"]
            privateLinkEndpoint: true
          extraProviders:
            - name: AWS
              credentials: "${auth:aws-us}"
              
registrar:
  cloudflare:
    type: cloudflare-registrar
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "${secret:CLOUDFLARE_ACCOUNT_ID}"
      zoneName: "myapp.com"
      dnsRecords:
        - name: "@"
          type: A
          value: "203.0.113.10"
          proxied: true
        - name: "api"
          type: CNAME
          value: "us-east-1.elb.amazonaws.com"
          proxied: true
        - name: "api-eu"
          type: CNAME
          value: "eu-west-1.elb.amazonaws.com"
          proxied: true
```

**Features:**
- Multi-region AWS deployment (US East, EU West)
- Cloudflare DNS management with proxying
- MongoDB Atlas with cross-region networking
- S3 buckets with CORS configuration
- RDS PostgreSQL in multiple regions
- Private link endpoints for security
- Comprehensive backup strategies

### GCP Comprehensive Parent Stack
Complete GCP setup with GKE, databases, and Cloudflare domain management.

**Use Case:** GCP-native applications, Kubernetes workloads, global CDN

**Configuration:**
```yaml
# .sc/stacks/devops/server.yaml
schemaVersion: 1.0

templates:
  gke-autopilot:
    type: gcp-gke-autopilot
    gkeClusterResource: "main-gke-cluster"
    artifactRegistryResource: "main-registry"

resources:
  resources:
    production:
      main-gke-cluster:
        type: gcp-gke-autopilot-cluster
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: europe-west3
          gkeMinVersion: "1.27"
          
      main-registry:
        type: gcp-artifact-registry
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: europe-west3
          docker:
            immutableTags: true
            
      postgres-db:
        type: gcp-sql-postgres
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          region: europe-west3
          tier: db-standard-2
          diskSize: 100
          databaseVersion: POSTGRES_14
          
      redis-cache:
        type: gcp-redis
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: europe-west3
          tier: standard
          memorySizeGb: 4
          
      storage-bucket:
        type: gcp-bucket
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          name: "myapp-prod-storage"
          location: europe-west3
          
      pubsub-messaging:
        type: gcp-pubsub
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          topics:
            - name: "events"
              labels:
                environment: "production"
          subscriptions:
            - name: "event-processor"
              topic: "events"
              ackDeadlineSec: 60
              exactlyOnceDelivery: true

registrar:
  cloudflare:
    type: cloudflare-registrar
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "${secret:CLOUDFLARE_ACCOUNT_ID}"
      zoneName: "myapp.com"
      dnsRecords:
        - name: "@"
          type: A
          value: "203.0.113.20"
          proxied: true
        - name: "api"
          type: CNAME
          value: "gcp-lb.myapp.com"
          proxied: true
```

**Features:**
- GKE Autopilot for managed Kubernetes
- Artifact Registry for container images
- Cloud SQL PostgreSQL database
- Redis for caching and sessions
- Cloud Storage for file storage
- Pub/Sub for event-driven architecture
- Cloudflare integration for global CDN

### Hybrid Cloud Parent Stack
Multi-cloud parent stack configuration with AWS and GCP resources.

**Use Case:** Multi-cloud strategy, vendor diversification, best-of-breed services

**Configuration:**
```yaml
# .sc/stacks/devops/server.yaml
schemaVersion: 1.0

templates:
  aws-compute:
    type: aws-ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"
      region: us-east-1
      
  gcp-data:
    type: gcp-gke-autopilot
    gkeClusterResource: "data-cluster"
    artifactRegistryResource: "data-registry"

resources:
  resources:
    production:
      # AWS for compute workloads
      ecs-cluster:
        type: aws-ecs-cluster
        config:
          credentials: "${auth:aws}"
          account: "${auth:aws.projectId}"
          region: us-east-1
          name: "myapp-compute"
          
      s3-storage:
        type: aws-s3-bucket
        config:
          credentials: "${auth:aws}"
          account: "${auth:aws.projectId}"
          region: us-east-1
          name: "myapp-hybrid-storage"
          allowOnlyHttps: true
          
      # GCP for data services
      data-cluster:
        type: gcp-gke-autopilot-cluster
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: us-central1
          gkeMinVersion: "1.27"
          
      data-registry:
        type: gcp-artifact-registry
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: us-central1
          
      bigquery-analytics:
        type: gcp-bigquery
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          datasetId: "analytics"
          location: US
```

**Features:**
- AWS for compute-intensive workloads
- GCP for data analytics and processing
- Cross-cloud resource sharing
- Unified DNS management
- Cost optimization through best-of-breed services

## Common Parent Stack Patterns

### Environment Separation
```yaml
resources:
  resources:
    development:
      template: basic-setup
      scaling:
        minCapacity: 1
        maxCapacity: 3
    staging:
      template: production-like
      scaling:
        minCapacity: 2
        maxCapacity: 10
    production:
      template: high-availability
      scaling:
        minCapacity: 5
        maxCapacity: 100
```

### Shared Resource Configuration
```yaml
resources:
  resources:
    production:
      shared-database:
        type: aws-rds-postgres
        config:
          instanceClass: "db.r5.2xlarge"
          multiAZ: true
          backupRetention: 30
      shared-cache:
        type: aws-elasticache-redis
        config:
          nodeType: "cache.r6g.large"
          numCacheNodes: 3
```

## Deployment Commands

Provision parent stack:
```bash
sc provision -s devops
```

Update parent stack:
```bash
sc provision -s devops --update
```

## Best Practices

- **Use environment separation** for development, staging, and production
- **Implement proper backup strategies** for all data stores
- **Configure monitoring and alerting** for all shared resources
- **Use least-privilege IAM policies** for all service accounts
- **Implement network security** with proper VPC and firewall rules
- **Use infrastructure as code** for all resource definitions
- **Document resource dependencies** and relationships
- **Plan for disaster recovery** across regions or clouds
- **Monitor costs** and implement cost optimization strategies
- **Use consistent naming conventions** across all resources

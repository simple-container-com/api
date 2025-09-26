# GKE Autopilot Examples

This section contains examples of deploying applications to Google Kubernetes Engine Autopilot using Simple Container.

## Available Examples

### Comprehensive Setup
Complete GCP setup with GKE, Artifact Registry, Pub/Sub, Redis, and MongoDB Atlas.

**Use Case:** Full-stack applications, microservices architecture, production-ready GCP deployment

**Parent Stack Configuration:**
```yaml
# .sc/stacks/devops/server.yaml
schemaVersion: 1.0
templates:
  gke-autopilot-template:
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
            
      redis-cache:
        type: gcp-redis
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: europe-west3
          tier: standard
          memorySizeGb: 1
          
      pubsub-messaging:
        type: gcp-pubsub
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          topics:
            - name: "user-events"
              labels:
                environment: "production"
          subscriptions:
            - name: "user-events-processor"
              topic: "user-events"
              ackDeadlineSec: 60
              exactlyOnceDelivery: true
              
      mongodb-atlas:
        type: mongodb-atlas
        config:
          publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
          privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
          orgId: "${secret:MONGODB_ATLAS_ORG_ID}"
          projectId: "${secret:MONGODB_ATLAS_PROJECT_ID}"
          instanceSize: M30
          region: EUROPE_WEST_3
          cloudProvider: GCP
          admins: ["admin@mycompany.com"]
          developers: ["dev@mycompany.com"]
          backup:
            every: 6h
            retention: 168h
          networkConfig:
            allowCidrs: ["10.0.0.0/8"]
          extraProviders:
            - name: GCP
              credentials: "${auth:gcloud}"
```

**Client Stack Configuration:**
```yaml
# .sc/stacks/myapp/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [redis-cache, pubsub-messaging, mongodb-atlas]
      runs: [web-app, worker-service]
```

**Docker Compose:**
```yaml
# docker-compose.yaml
version: '3.8'
services:
  web-app:
    build: ./web
    ports:
      - "8080:8080"
    environment:
      REDIS_URL: ${REDIS_URL}
      MONGODB_URL: ${MONGODB_URL}
      PUBSUB_TOPIC: ${PUBSUB_TOPIC}
      
  worker-service:
    build: ./worker
    environment:
      REDIS_URL: ${REDIS_URL}
      MONGODB_URL: ${MONGODB_URL}
      PUBSUB_SUBSCRIPTION: ${PUBSUB_SUBSCRIPTION}
```

**Features:**

- Complete GCP setup with all major services
- GKE Autopilot for managed Kubernetes
- Artifact Registry for container images
- Redis for caching and session storage
- Pub/Sub for event-driven architecture
- MongoDB Atlas integration with GCP networking
- Multi-service deployment with resource sharing

### Multi-Region Deployment
Deploy applications across multiple GCP regions for high availability.

**Use Case:** Global applications, disaster recovery, low-latency worldwide access

**Configuration:**
```yaml
# .sc/stacks/global-app/client.yaml
schemaVersion: 1.0
stacks:
  us-production:
    type: cloud-compose
    parent: myorg/infrastructure-us
    config:
      uses: [gke-cluster-us, redis-us]
      runs: [api-service]
      
  eu-production:
    type: cloud-compose
    parent: myorg/infrastructure-eu
    config:
      uses: [gke-cluster-eu, redis-eu]
      runs: [api-service]
```

**Features:**

- Multi-region deployment
- Regional resource isolation
- Global load balancing
- Disaster recovery capabilities
- Reduced latency for global users

## Common Patterns

### Microservices Architecture
```yaml
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [gke-cluster, artifact-registry, mongodb-atlas]
      runs: [web-app, worker-service]
```

### Auto-Scaling Configuration
```yaml
# In docker-compose.yaml
services:
  web-app:
    deploy:
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "500m"
          memory: "512Mi"
      replicas: 3
      update_config:
        parallelism: 1
        delay: 10s
```


## Deployment Commands

**Deploy to staging:**
```bash
sc deploy -s myapp -e staging
```

**Deploy to production:**
```bash
sc deploy -s myapp -e production
```

## Best Practices

- **Use GKE Autopilot** for managed Kubernetes experience
- **Configure proper resource requests and limits** for optimal scheduling
- **Implement health checks** for all services
- **Use Artifact Registry** for secure container image storage
- **Configure network policies** for security isolation
- **Set up monitoring and logging** with Google Cloud Operations
- **Use Workload Identity** for secure GCP service access
- **Implement proper secret management** with Google Secret Manager
- **Configure auto-scaling** based on actual usage patterns
- **Use regional persistent disks** for data durability

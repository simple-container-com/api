# ECS Deployment Examples

This section contains examples of deploying containerized applications to AWS ECS Fargate using Simple Container.

## Available Examples

### Backend Service
Deploy a Node.js backend service with MongoDB integration.

**Use Case:** REST APIs, GraphQL services, microservices backends

**Configuration:**
```yaml
# .sc/stacks/backend/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      domain: api.mycompany.com
      size:
        cpu: 2048
        memory: 4096
      dockerComposeFile: ${git:root}/docker-compose.yaml
      uses: [mongodb-shared, redis-cache]
      runs: [backend-service]
      env:
        MONGODB_URL: "${resource:mongodb-shared.uri}"
        REDIS_HOST: "${resource:redis-cache.host}"
        REDIS_PORT: "${resource:redis-cache.port}"
        NODE_ENV: production
      alerts:
        slack:
          webhookUrl: ${secret:alerts-slack-webhook}
        maxMemory:
          threshold: 80
          alertName: backend-max-memory
          description: "Backend memory usage exceeds 80%"
        maxCPU:
          threshold: 70
          alertName: backend-max-cpu
          description: "Backend CPU usage exceeds 70%"
```

**Docker Compose:**
```yaml
# docker-compose.yaml (for local development)
version: '3.8'
services:
  backend-service:
    build: .
    ports:
      - "3000:3000"
    environment:
      MONGODB_URL: "mongodb://localhost:27017/backend_dev"
      REDIS_URL: "redis://localhost:6379"
      NODE_ENV: development
```

**Features:**

- MongoDB Atlas integration
- Redis caching layer
- Auto-scaling configuration
- Health checks and monitoring
- Secure environment variable injection

### Vector Database
Deploy a high-performance vector database service.

**Use Case:** AI/ML applications, similarity search, recommendation engines

**Configuration:**
```yaml
# .sc/stacks/vectordb/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      domain: vectordb.mycompany.com
      size:
        cpu: 4096
        memory: 8192
      dockerComposeFile: ${git:root}/docker-compose.yaml
      uses: [vector-storage, nlb-loadbalancer]
      runs: [vector-service]
      alerts:
        slack:
          webhookUrl: ${secret:alerts-slack-webhook}
        maxMemory:
          threshold: 85
          alertName: vectordb-max-memory
          description: "Vector DB memory usage exceeds 85%"
        maxCPU:
          threshold: 75
          alertName: vectordb-max-cpu
          description: "Vector DB CPU usage exceeds 75%"
```

**Features:**

- Network Load Balancer for high performance
- Auto-scaling based on CPU utilization
- Persistent vector storage
- High-throughput configuration
- GPU support for ML workloads

### Blockchain Service
Deploy blockchain integration services.

**Use Case:** Web3 applications, cryptocurrency services, smart contract interaction

**Configuration:**
```yaml
# .sc/stacks/blockchain/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [ethereum-node, postgres-db]
      domain: blockchain.mycompany.com
      size:
        cpu: 2048
        memory: 4096
      dockerComposeFile: ${git:root}/docker-compose.yaml
      runs: [blockchain-service]
      dependencies:
        - name: ethereum-shared
          owner: myproject/blockchain-infrastructure  
          resource: ethereum-node-cluster
```

**Features:**

- Ethereum node integration
- Cross-service dependencies
- PostgreSQL for transaction storage
- Secure API endpoints
- Real-time blockchain monitoring

### Blog Platform
Deploy a multi-service blog platform.

**Use Case:** Content management, publishing platforms, media sites

**Configuration:**
```yaml
# .sc/stacks/blog/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [postgres-db, redis-cache, s3-media]
      domain: blog.mycompany.com
      size:
        cpu: 1024
        memory: 2048
      dockerComposeFile: ${git:root}/docker-compose.yaml
      runs: [blog-api, blog-admin]
```

**Docker Compose:**
```yaml
version: '3.8'
services:
  blog-api:
    build: ./api
    ports:
      - "3000:3000"
    environment:
      DATABASE_URL: ${DATABASE_URL}
      REDIS_URL: ${REDIS_URL}
      S3_BUCKET: ${S3_BUCKET}
  
  blog-admin:
    build: ./admin
    ports:
      - "3001:3001"
    environment:
      API_URL: "https://blog.mycompany.com/api"
```

**Features:**

- Multi-service deployment
- Reverse proxy configuration
- Media storage with S3
- Admin interface separation
- Content delivery optimization

### Meteor.js Application
Deploy a Meteor.js full-stack application.

**Use Case:** Real-time applications, collaborative tools, full-stack JavaScript apps

**Configuration:**
```yaml
# .sc/stacks/meteor-app/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [mongodb-shared]
      domain: app.mycompany.com
      size:
        cpu: 1024
        memory: 2048
      dockerComposeFile: ${git:root}/docker-compose.yaml
      runs: [meteor-app]
```

**Features:**

- MongoDB integration
- Real-time data synchronization
- WebSocket support
- Meteor-specific optimizations
- Session affinity configuration

## Common Patterns

### Multi-Service Architecture
```yaml
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [postgres-db, redis-cache, s3-storage]
      runs: [api-service, worker-service, scheduler]
      dependencies:
        - name: billing
          owner: myproject/billing
          resource: mongo-cluster2
```


### Health Checks
```yaml
# docker-compose.yaml
services:
  api-service:
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

## Deployment Commands

**Deploy to staging:**
```bash
sc deploy -s myservice -e staging
```

**Deploy to production:**
```bash
sc deploy -s myservice -e production
```

## Best Practices

- **Use health checks** for all services to ensure proper deployment
- **Configure auto-scaling** based on actual usage patterns
- **Implement proper logging** with structured log formats
- **Use environment-specific configurations** for different environments
- **Set up monitoring and alerting** for production services
- **Implement graceful shutdown** handling in your applications
- **Use secrets management** for sensitive configuration values

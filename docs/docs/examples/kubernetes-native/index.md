# Kubernetes Native Examples

This section contains examples of deploying applications to native Kubernetes clusters using Simple Container.

## Available Examples

### Streaming Platform
Deploy a streaming platform with hardcoded IPs, N8N integration, and zero-downtime configurations.

**Use Case:** Media streaming, real-time data processing, workflow automation

**Configuration:**
```yaml
# .sc/stacks/streaming/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [redis-cache]
      runs: [streaming-server, n8n-workflow]
```

**Docker Compose:**
```yaml
# docker-compose.yaml
version: '3.8'
services:
  streaming-server:
    build: ./streaming
    ports:
      - "8080:8080"
    environment:
      RTMP_PORT: "1935"
      HLS_PORT: "8080"
      REDIS_URL: ${REDIS_URL}
    volumes:
      - streaming-data:/var/streaming
      
  n8n-workflow:
    image: n8nio/n8n:latest
    ports:
      - "5678:5678"
    environment:
      N8N_BASIC_AUTH_ACTIVE: "true"
      N8N_BASIC_AUTH_USER: "${secret:N8N_USERNAME}"
      N8N_BASIC_AUTH_PASSWORD: "${secret:N8N_PASSWORD}"
      WEBHOOK_URL: "https://streaming.mycompany.com/webhook"
    volumes:
      - n8n-data:/home/node/.n8n

volumes:
  streaming-data:
  n8n-data:
```


**Features:**

- Hardcoded IP addresses for consistent access
- N8N workflow automation integration
- Zero-downtime deployment strategy
- RTMP and HLS streaming support
- Redis integration for session management
- Custom registry authentication
- SSL/TLS termination with Caddy

### High-Resource Environment
Deploy applications requiring significant compute resources.

**Use Case:** AI/ML workloads, data processing, compute-intensive applications

**Configuration:**
```yaml
# .sc/stacks/ai-workload/client.yaml
schemaVersion: 1.0
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [gpu-resources, storage-volumes]
      runs: [ai-processor]
```

**Docker Compose:**
```yaml
# docker-compose.yaml
version: '3.8'
services:
  ai-processor:
    build: ./ai-processor
    ports:
      - "8080:8080"
    deploy:
      resources:
        requests:
          cpus: '8.0'
          memory: 32G
        limits:
          cpus: '16.0'
          memory: 64G
    environment:
      CUDA_VISIBLE_DEVICES: "0,1"
      MODEL_PATH: "/models"
      BATCH_SIZE: "32"
    volumes:
      - model-storage:/models
      - gpu-cache:/tmp/gpu-cache

volumes:
  model-storage:
    driver: local
    driver_opts:
      type: nfs
      o: addr=nfs.mycompany.com,rw
      device: ":/models"
  gpu-cache:
```


**Features:**

- High CPU and memory allocation (32GB/16CPU)
- GPU support for AI/ML workloads
- NFS storage for large model files
- Node selector for specific hardware
- Resource quotas and limits
- Optimized for compute-intensive tasks

## Common Patterns

### Zero-Downtime Deployment
```yaml
# Deployment strategy for zero downtime
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 50%
      maxUnavailable: 0
  template:
    spec:
      containers:
      - name: app
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "sleep 15"]
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

- **Use rolling updates** for zero-downtime deployments
- **Configure proper resource requests and limits** for optimal scheduling
- **Implement comprehensive health checks** (readiness and liveness probes)
- **Use network policies** for security isolation
- **Configure persistent storage** for stateful applications
- **Set up horizontal pod autoscaling** based on metrics
- **Use node selectors** for hardware-specific workloads
- **Implement proper logging and monitoring** with Prometheus and Grafana
- **Use secrets and configmaps** for configuration management
- **Configure RBAC** for proper access control

# Advanced Configuration Examples

This section contains examples of advanced Simple Container configurations for complex deployment scenarios.

## Available Examples

### High-Resource AI Development Environment
Deploy a 32GB/16CPU AI development environment with Kubernetes integration.

**Use Case:** Machine learning development, AI model training, high-performance computing

**Configuration:**
```yaml
# .sc/stacks/ai-dev/client.yaml
schemaVersion: 1.0
stacks:
  development:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [gpu-resources, storage-volumes]
      runs: [jupyter-lab, tensorboard, mlflow]
```

**Docker Compose:**
```yaml
# docker-compose.yaml
version: '3.8'
services:
  jupyter-lab:
    build: ./jupyter
    ports:
      - "8888:8888"
    deploy:
      resources:
        requests:
          cpus: '8.0'
          memory: 16G
        limits:
          cpus: '16.0'
          memory: 32G
    environment:
      JUPYTER_ENABLE_LAB: "yes"
      JUPYTER_TOKEN: "${secret:JUPYTER_TOKEN}"
      CUDA_VISIBLE_DEVICES: "all"
    volumes:
      - notebooks:/home/jovyan/work
      - datasets:/data
      - models:/models
      
  tensorboard:
    image: tensorflow/tensorflow:latest-gpu
    ports:
      - "6006:6006"
    command: tensorboard --logdir=/logs --host=0.0.0.0
    volumes:
      - training-logs:/logs
      
  mlflow:
    image: python:3.9
    ports:
      - "5000:5000"
    command: |
      bash -c "pip install mlflow && 
               mlflow server --host 0.0.0.0 --port 5000 --backend-store-uri sqlite:///mlflow.db"
    volumes:
      - mlflow-data:/mlflow

volumes:
  notebooks:
  datasets:
    driver: local
    driver_opts:
      type: nfs
      o: addr=nfs.mycompany.com,rw
      device: ":/datasets"
  models:
  training-logs:
  mlflow-data:
```


**Features:**
- 32GB RAM and 16 CPU cores allocation
- GPU support for AI/ML workloads
- Jupyter Lab for interactive development
- TensorBoard for experiment visualization
- MLflow for experiment tracking
- NFS storage for large datasets
- High-performance node selection

### Multi-Environment Complex Deployment
Deploy applications across multiple environments with different configurations.

**Use Case:** Enterprise applications, complex staging/production setups, A/B testing

**Configuration:**
```yaml
# .sc/stacks/complex-app/client.yaml
schemaVersion: 1.0

# YAML anchors for reusable configurations
x-common-config: &common-config
  type: cloud-compose
  parent: myorg/infrastructure

x-app-env: &app-env
  REDIS_URL: "${resource:redis-cache.connectionString}"
  DATABASE_URL: "${resource:postgres-db.connectionString}"
  LOG_LEVEL: info

stacks:
  development:
    <<: *common-config
    parentEnv: development
    config:
      uses: [redis-dev, postgres-dev, s3-dev]
      runs: [api-service, worker-service, admin-ui]
      scale:
        max: 3
        min: 1
        policy:
          cpu:
            max: 70
      env:
        <<: *app-env
        LOG_LEVEL: debug
        FEATURE_FLAGS: "all_enabled"
        
  staging:
    <<: *common-config
    parentEnv: staging
    config:
      uses: [redis-staging, postgres-staging, s3-staging]
      runs: [api-service, worker-service, admin-ui]
      scale:
        max: 8
        min: 2
        policy:
          cpu:
            max: 70
      env:
        <<: *app-env
        FEATURE_FLAGS: "staging_features"
        
  production:
    <<: *common-config
    parentEnv: production
    config:
      uses: [redis-prod, postgres-prod, s3-prod, monitoring]
      runs: [api-service, worker-service, admin-ui]
      scale:
        max: 50
        min: 5
        policy:
          cpu:
            max: 70
      env:
        <<: *app-env
        FEATURE_FLAGS: "production_stable"
      alerts:
        slack:
          webhookUrl: "${secret:SLACK_WEBHOOK_URL}"
        maxMemory:
          threshold: 85
          alertName: production-max-memory
          description: "Production memory usage exceeds 85% ${secret:alerts-bot-slack-cc}"
        maxCPU:
          threshold: 80
          alertName: production-max-cpu
          description: "Production CPU usage exceeds 80% ${secret:alerts-bot-slack-cc}"
```

**Features:**
- Multi-environment deployment (dev/staging/prod)
- YAML anchors for configuration reuse
- Environment-specific scaling policies
- Feature flag management per environment
- Monitoring and alerting integration
- Resource isolation per environment

### Hybrid Cloud Configuration
Deploy applications across multiple cloud providers.

**Use Case:** Multi-cloud strategy, vendor lock-in avoidance, geographic distribution

**Configuration:**
```yaml
# .sc/stacks/hybrid-app/client.yaml
schemaVersion: 1.0
stacks:
  aws-primary:
    type: cloud-compose
    parent: myorg/aws-infrastructure
    config:
      uses: [aws-rds, aws-redis, aws-s3]
      runs: [primary-api, data-processor]
      
  gcp-secondary:
    type: cloud-compose
    parent: myorg/gcp-infrastructure
    config:
      uses: [gcp-sql, gcp-redis, gcp-storage]
      runs: [secondary-api, backup-processor]
      
  azure-backup:
    type: cloud-compose
    parent: myorg/azure-infrastructure
    config:
      uses: [azure-sql, azure-storage]
      runs: [backup-api]
```

**Features:**
- Multi-cloud deployment (AWS, GCP, Azure)
- Geographic distribution
- Cloud-specific resource utilization
- Disaster recovery across providers
- Vendor lock-in avoidance

## Common Advanced Patterns



## Deployment Commands

Deploy development environment:
```bash
sc deploy -s myapp -e development
```

Deploy staging environment:
```bash
sc deploy -s myapp -e staging
```

Deploy production environment:
```bash
sc deploy -s myapp -e production
```

## Best Practices

- **Use YAML anchors** to reduce configuration duplication
- **Implement environment-specific scaling** policies
- **Configure proper resource limits** for high-resource workloads
- **Use node selectors** for hardware-specific requirements
- **Implement comprehensive monitoring** and alerting
- **Plan for disaster recovery** across multiple environments
- **Use feature flags** for controlled feature rollouts
- **Implement proper secret management** across environments
- **Configure network policies** for security isolation
- **Use persistent storage** for stateful applications

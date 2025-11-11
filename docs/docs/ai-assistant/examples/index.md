# AI Assistant Examples

Real-world examples and scenarios for using Simple Container AI Assistant in both Developer and DevOps modes.

## 📋 Example Categories

### **🧑‍💻 Developer Examples**
- **[Node.js Express API](nodejs-express-api.md)** - REST API with PostgreSQL and Redis
- **[Python Django App](python-django-app.md)** - Web application with database  
- **[Go Microservice](go-microservice.md)** - gRPC service with health checks
- **[React Frontend](react-frontend.md)** - Static site deployment
- **[Docker Multi-Stage](docker-multistage.md)** - Optimized container builds

### **🛠️ DevOps Examples**
- **[AWS Infrastructure](aws-infrastructure.md)** - Complete AWS setup with RDS and ECS
- **[GCP Setup](gcp-setup.md)** - GKE Autopilot with Cloud SQL
- **[Multi-Cloud](multi-cloud.md)** - AWS + GCP hybrid architecture
- **[Kubernetes Native](kubernetes-native.md)** - On-premises Kubernetes
- **[Security Hardening](security-hardening.md)** - Production security best practices

### **🔗 Integration Examples**
- **[IDE Integration](ide-integration.md)** - Windsurf, Cursor, VS Code setup
- **[CI/CD Pipeline](cicd-pipeline.md)** - GitHub Actions with AI Assistant
- **[Team Workflows](team-workflows.md)** - DevOps + Developer collaboration
- **[Custom MCP Clients](custom-mcp-clients.md)** - Building custom integrations

### **🎯 Use Case Examples**
- **[New Company Setup](new-company-setup.md)** - Complete organization onboarding
- **[Legacy Migration](legacy-migration.md)** - Migrating existing applications
- **[Scaling Challenges](scaling-challenges.md)** - Growing from startup to enterprise
- **[Compliance Requirements](compliance-requirements.md)** - SOC2, GDPR, HIPAA

## 🚀 Quick Start Examples

### **5-Minute Developer Setup**
```bash
# 1. Developer gets a new Node.js project
cd my-new-api
npm init -y
npm install express pg redis

# 2. AI Assistant analyzes and sets up everything
sc assistant dev setup

# 3. Start developing locally
docker-compose up -d
npm start

# 4. Deploy to staging
sc deploy -e staging
```

### **10-Minute DevOps Setup**  
```bash
# 1. DevOps creates infrastructure project
mkdir company-infrastructure
cd company-infrastructure
sc init

# 2. Interactive infrastructure wizard
sc assistant devops setup --interactive
# Selects: AWS, PostgreSQL, Redis, S3, ECS

# 3. Configure secrets and deploy
sc secrets add aws-access-key aws-secret-key
sc provision -s infrastructure -e staging,production

# 4. Share with development teams
echo "Infrastructure ready! Use parent: infrastructure"
```

## 🎭 Mode-Specific Examples

### **Developer Mode Scenarios**

#### **Scenario 1: Existing Project Migration**
```bash
# You have an existing Express.js app to containerize
cd legacy-express-app

# AI Assistant analyzes existing codebase
sc assistant dev analyze --detailed
# Output: Detects Express, PostgreSQL, identifies optimization opportunities

# Generate Simple Container configs
sc assistant dev setup --interactive
# Asks about: Database preferences, scaling requirements, deployment target

# Review and customize generated files
cat client.yaml docker-compose.yaml Dockerfile
```

#### **Scenario 2: Microservice Architecture** 
```bash
# You're building a microservices system
mkdir payment-service && cd payment-service
npm init -y
npm install fastify pg @fastify/postgres

# AI Assistant generates service-specific config
sc assistant dev setup --framework fastify
# Creates: Service-to-service communication, shared database config

# Deploy as part of larger system
sc deploy -e staging --parent microservices-infra
```

### **DevOps Mode Scenarios**

#### **Scenario 1: Multi-Environment Infrastructure**
```bash
# Setting up dev/staging/production environments
sc assistant devops setup --envs development,staging,production

# Different resource specs per environment:
# - Development: Local docker-compose
# - Staging: Small cloud instances
# - Production: High-availability setup

# Secrets management per environment
sc assistant devops secrets --generate prod-db-password --length 64
sc assistant devops secrets --auth aws-production --interactive
```

#### **Scenario 2: Cost-Optimized Setup**
```bash
# Budget-conscious startup infrastructure
sc assistant devops setup --cloud aws --optimize-cost

# AI suggests:
# - Reserved instances for predictable workloads  
# - Spot instances for development
# - Auto-scaling policies to minimize idle resources
# - S3 intelligent tiering for storage
```

## 🏢 Industry Examples

### **E-commerce Platform**
```yaml
# Generated server.yaml for e-commerce
resources:
  production:
    # Customer database
    user-db:
      type: aws-rds-postgres
      instanceClass: db.r5.xlarge
      multiAZ: true
      
    # Product catalog
    catalog-db:
      type: aws-rds-postgres  
      instanceClass: db.r5.large
      allocateStorage: 100
      databaseName: catalog
      engineVersion: "15.4"
      username: dbadmin
      password: "${secret:catalog-db-password}"
      
    # Session storage
    session-store:
      type: s3-bucket
      name: session-storage
      allowOnlyHttps: true
      
    # File uploads
    uploads-bucket:
      type: s3-bucket
      name: uploads-storage
      allowOnlyHttps: true
```

### **SaaS Application**
```yaml
# Generated client.yaml for multi-tenant SaaS
stacks:
  api-service:
    type: cloud-compose
    parent: saas-infrastructure
    config:
      uses: [tenant-db, cache-cluster, file-storage]
      scale:
        min: 3
        max: 50
      env:
        MULTI_TENANT: "true"
        DATABASE_URL: "${resource:tenant-db.url}"
        CACHE_URL: "${resource:cache-cluster.url}"
        TENANT_ISOLATION: "database"
```

## 🔄 Workflow Examples

### **Complete Team Workflow**

#### **Week 1: DevOps Infrastructure Setup**
```bash
# Day 1: DevOps team sets up foundation
sc assistant devops setup --cloud aws --interactive
# Configures: ECS Fargate, RDS, ECR, S3

# Day 2: Security and secrets configuration  
sc assistant devops secrets --init --cloud aws
sc assistant devops resources --add monitoring,alerting

# Day 3: Deploy and validate infrastructure
sc provision -s infrastructure

# Verify infrastructure is working
curl -f https://staging-db.yourcompany.com/health || echo "Infrastructure deployed successfully"

# Day 4-5: Production deployment and documentation  
sc provision -s infrastructure
# Create team documentation and onboarding guides
```

#### **Week 2: Developer Onboarding**
```bash
# Team A: API Development
cd user-service
sc assistant dev analyze  # Detects: Go + Gin + PostgreSQL
sc assistant dev setup    # Generates: client.yaml, Dockerfile, compose
sc deploy -s user-service -e staging      # Deploy to shared staging infrastructure

# Team B: Frontend Development  
cd admin-dashboard
sc assistant dev analyze  # Detects: React + TypeScript
sc assistant dev setup    # Generates: Static site config
sc deploy -s admin-dashboard -e staging      # Deploy static site

# Team C: Background Jobs
cd notification-worker
sc assistant dev analyze  # Detects: Python + Celery + Redis
sc assistant dev setup    # Generates: Worker configuration
sc deploy -s notification-worker -e staging      # Deploy worker service
```

### **Scaling Workflow Example**

#### **Phase 1: MVP (Month 1)**
```yaml
# Simple single-environment setup
resources:
  production:
    app-db:
      type: aws-rds-postgres
      name: app-database
      instanceClass: db.t3.micro
      allocateStorage: 20
      databaseName: myapp
      engineVersion: "15.4"
      username: dbadmin
      password: "${secret:db-password}"
    app-cache:
      type: s3-bucket
      name: app-cache-bucket
      allowOnlyHttps: true
```

#### **Phase 2: Growth (Month 6)**
```yaml
# Multi-environment with monitoring
resources:
  staging:
    app-db:
      type: aws-rds-postgres
      instanceClass: db.t3.small
  production:
    app-db:
      type: aws-rds-postgres
      name: production-database
      instanceClass: db.r5.large
      allocateStorage: 100
      databaseName: myapp
      engineVersion: "15.4"
      username: dbadmin
      password: "${secret:prod-db-password}"
    monitoring:
      type: s3-bucket
      name: monitoring-logs
      allowOnlyHttps: true
```

#### **Phase 3: Scale (Year 1)**
```yaml
# Multi-region with auto-scaling
resources:
  production-us:
    primary-db:
      type: aws-rds-postgres
      instanceClass: db.r5.2xlarge
      multiAZ: true
  production-eu:
    replica-db:
      type: aws-rds-postgres
      instanceClass: db.r5.xlarge
      readReplica: production-us.primary-db
```

## 💡 Best Practice Examples

### **Configuration Patterns**

#### **Environment Consistency**
```yaml
# Template for consistent naming across environments
resources:
  ${environment}:
    postgres-db:
      name: "myapp-${environment}-db"
      instanceClass: "${environment == 'production' ? 'db.r5.large' : 'db.t3.small'}"
```

#### **Secret Management**
```yaml
# Secure secret handling pattern
auth:
  aws:
    account: "${secret:aws-account-id}"
    accessKey: "${secret:aws-access-key-${environment}}"
    secretAccessKey: "${secret:aws-secret-key-${environment}}"
```

### **Development Patterns**

#### **Feature Branch Deployments**
```bash
# Create preview environment for feature branch
export BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)
sc assistant dev setup --env preview-${BRANCH_NAME}
sc deploy -e preview-${BRANCH_NAME}

# Cleanup when feature is merged
sc destroy -e preview-${BRANCH_NAME}
```

#### **Local Development Optimization**
```yaml
# Optimized docker-compose for development
services:
  app:
    build: .
    volumes:
      - .:/app:delegated  # Performance optimization for macOS
      - node_modules:/app/node_modules  # Avoid overwriting
    environment:
      - NODE_ENV=development
      - DEBUG=app:*
```

## 🔗 Related Resources

### **Documentation Links**
- **[Getting Started](../getting-started.md)** - Basic setup guide
- **[Developer Mode](../developer-mode.md)** - Complete developer workflows
- **[DevOps Mode](../devops-mode.md)** - Infrastructure management
- **[Commands Reference](../commands.md)** - Full command documentation

### **External Resources**
- **[Simple Container Main Documentation](https://docs.simple-container.com)**
- **[Cloud Provider Best Practices](https://docs.simple-container.com/best-practices)**
- **[Security Guidelines](https://docs.simple-container.com/security)**
- **[Performance Optimization](https://docs.simple-container.com/performance)**

### **Community Examples**
- **[GitHub Examples Repository](https://github.com/simple-container-com/examples)**
- **[Community Slack](https://slack.simple-container.com)** - Share your examples
- **[Stack Overflow](https://stackoverflow.com/questions/tagged/simple-container)** - Q&A

---

**Need a specific example?** [Search our documentation](../commands.md#search-commands):
```bash
sc assistant search "your specific use case"
```

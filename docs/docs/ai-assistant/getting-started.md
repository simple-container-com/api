# Getting Started with AI Assistant

Get up and running with Simple Container's AI Assistant in under 10 minutes. This guide covers both **Developer** and **DevOps** workflows.

## 🎯 Prerequisites

### System Requirements
- Simple Container CLI installed (`sc` command available)
- Docker installed (for local development)
- Cloud provider account (AWS, GCP, or Kubernetes cluster)

### Installation Check
```bash
# Verify Simple Container is installed
sc --version
# Should output: Simple Container v1.x.x

# Check if AI assistant is available
sc assistant --help
# Should show assistant subcommands

# Test documentation search
sc assistant search "getting started"
```

## 🎭 Choose Your Role

Simple Container AI Assistant has two distinct modes based on your team role:

<div style="display: flex; gap: 20px; margin: 20px 0;">

<div style="flex: 1; border: 2px solid #0066cc; border-radius: 8px; padding: 20px;">
<h3>👩‍💻 I'm a Developer</h3>
<p><strong>You build applications</strong></p>
<ul>
<li>Work with existing infrastructure</li>
<li>Need client.yaml and docker-compose files</li>
<li>Focus on application deployment</li>
<li>Use shared resources from DevOps team</li>
</ul>
<p><a href="#developer-setup">→ Developer Setup Guide</a></p>
</div>

<div style="flex: 1; border: 2px solid #cc6600; border-radius: 8px; padding: 20px;">
<h3>🔧 I'm DevOps/Infrastructure</h3>
<p><strong>You manage shared infrastructure</strong></p>
<ul>
<li>Set up cloud resources and databases</li>
<li>Need server.yaml and secrets.yaml files</li>
<li>Create templates for dev teams</li>
<li>Manage environments and security</li>
</ul>
<p><a href="#devops-setup">→ DevOps Setup Guide</a></p>
</div>

</div>

## 🔧 DevOps Setup

**DevOps teams should set up infrastructure FIRST** before developers can deploy applications.

### Step 1: Initialize Infrastructure Project
```bash
# Create infrastructure directory
mkdir mycompany-infrastructure
cd mycompany-infrastructure

# Initialize Simple Container
sc init
```

### Step 2: Run Infrastructure Wizard
```bash
# Start interactive setup wizard
sc assistant devops setup --interactive
```

The wizard will guide you through:

#### **Cloud Provider Selection**
```
🌐 Select your primary cloud provider:
1. AWS (Recommended for most teams)
2. GCP (Google Cloud Platform) 
3. Kubernetes (Cloud-agnostic)

Choice [1-3]: 1 ✅
```

#### **Environment Configuration**
```
📊 Configure environments:
✅ Development (docker-compose locally)
✅ Staging (cloud resources, cost-optimized)
✅ Production (cloud resources, high availability)

Additional environments (preview, testing)? (y/n): n
```

#### **Resource Selection**
```
🎯 Select shared resources:

Databases:
☑️ PostgreSQL (recommended for most apps)
☐ MongoDB Atlas (document database)
☐ MySQL (legacy compatibility)
☑️ Redis (caching and sessions)

Storage:
☑️ S3-compatible bucket (file uploads)
☐ CDN (static asset delivery)

Compute:
☑️ ECS Fargate (containerized applications)
☐ Lambda (serverless functions)

Continue? (y/n): y ✅
```

### Step 3: Review Generated Files
```bash
# Check generated infrastructure files
ls -la .sc/stacks/infrastructure/
# server.yaml    - Infrastructure resources and templates  
# secrets.yaml   - Authentication and sensitive configuration

# Review server configuration
cat .sc/stacks/infrastructure/server.yaml
```

### Step 4: Configure Secrets
```bash
# Add cloud provider credentials
sc secrets add aws-access-key
# Enter value: AKIA...

sc secrets add aws-secret-key  
# Enter value: secret-key...

sc secrets add staging-db-password
# Enter value: secure-staging-password

sc secrets add prod-db-password
# Enter value: ultra-secure-production-password
```

### Step 5: Deploy Infrastructure
```bash
# Deploy staging environment
sc provision -s infrastructure -e staging

# Deploy production environment
sc provision -s infrastructure -e production

# Verify deployment by checking configuration files
cat .sc/stacks/infrastructure/server.yaml | grep -A 10 "resources:"
```

### Step 6: Share with Development Teams
```bash
# Review resource information for developers
echo "Available resources for developers:"
cat .sc/stacks/infrastructure/server.yaml

# Share this information with your development teams:
# - Available resources (databases, cache, storage)
# - Parent stack name: "infrastructure"  
# - Environment names: "staging", "production"
```

## 👩‍💻 Developer Setup

**Developers use existing infrastructure** set up by the DevOps team.

### Step 1: Navigate to Your Application
```bash
# Go to your application directory
cd my-awesome-app

# Verify this is a valid project directory
ls -la
# Should see package.json, requirements.txt, go.mod, or similar
```

### Step 2: Analyze Your Project
```bash
# Let AI assistant analyze your project
sc assistant dev analyze

# Example output:
# 🔍 Analyzing project at: .
# 
# 🎯 Analysis Results:
#    Language: Node.js (v18.x)
#    Framework: Express.js  
#    Architecture: REST API
#    Dependencies: PostgreSQL, Redis detected
#    Confidence: 92%
#
# 📋 Recommendations:
#    ✅ Use PostgreSQL database resource
#    ✅ Use Redis cache resource  
#    ✅ Use ECS Fargate template
#    ✅ Configure health check endpoint
```

### Step 3: Generate Application Configuration
```bash
# Generate client.yaml and docker-compose.yaml
sc assistant dev setup

# Interactive mode for customization
sc assistant dev setup --interactive
```

### Step 4: Review Generated Files
```bash
# Check generated files
ls -la
# client.yaml           - Simple Container app configuration
# docker-compose.yaml   - Local development environment
# Dockerfile            - Container image definition (if needed)

# Review application configuration  
cat .sc/stacks/my-awesome-app/client.yaml
```

### Step 5: Set Up Local Development
```bash
# Start local development environment
docker-compose up -d

# Check services are running
docker-compose ps
# Should show your app + database + redis containers

# Run your application
npm start  # or python manage.py runserver, go run main.go
```

### Step 6: Deploy to Staging
```bash
# Deploy to staging environment  
sc deploy -e staging

# Verify deployment is working
curl https://staging-api.yourcompany.com/health

# Check application logs via Docker (if needed)
docker logs my-awesome-app_app_1
```

### Step 7: Deploy to Production
```bash
# Deploy to production environment
sc deploy -e production

# Scaling is configured in client.yaml config.scale section
# Edit .sc/stacks/my-awesome-app/client.yaml to update scaling

# Verify production deployment is working  
curl https://api.yourcompany.com/health
```

## 🔄 Complete Workflow Example

Here's how DevOps and Developer teams work together:

### 1. **DevOps Team** - Infrastructure Setup
```bash
# 1. DevOps creates infrastructure project
mkdir acme-infrastructure && cd acme-infrastructure
sc init

# 2. Set up shared resources  
sc assistant devops setup --cloud aws --interactive

# 3. Configure secrets and deploy
sc secrets add aws-access-key aws-secret-key
sc provision -s infrastructure -e staging
sc provision -s infrastructure -e production

# 4. Share resource info with developers
echo "Infrastructure ready! Developers can reference:"
echo "- Parent: infrastructure"  
echo "- Resources: postgres-db, redis-cache, uploads-bucket"
echo "- Environments: staging, production"
```

### 2. **Developer Team** - Application Setup
```bash
# 1. Developer works on their app
cd my-express-api
sc assistant dev analyze
# Detects: Node.js + Express + PostgreSQL + Redis

# 2. Generate application configs
sc assistant dev setup
# Creates: client.yaml, docker-compose.yaml, Dockerfile

# 3. Local development
docker-compose up -d  # Starts local db + redis
npm run dev           # Starts application

# 4. Deploy to staging
sc deploy -e staging
# Uses shared staging database and cache

# 5. Deploy to production  
sc deploy -e production
# Uses shared production database and cache
```

## 🎯 Common Scenarios

### **Scenario 1: New Company Setup**
```bash
# Step 1: DevOps sets up foundation
sc assistant devops setup --cloud aws --envs staging,production
sc provision -s infrastructure -e staging,production

# Step 2: Multiple dev teams deploy apps
cd team-a/api-service && sc assistant dev setup && sc deploy -e staging
cd team-b/web-app && sc assistant dev setup && sc deploy -e staging  
cd team-c/worker && sc assistant dev setup && sc deploy -e staging
```

### **Scenario 2: Adding New Environment**
```bash
# DevOps adds preview environment
sc assistant devops resources --env preview --copy-from staging
sc provision -s infrastructure -e preview

# Developers can now deploy to preview
sc deploy -e preview
```

### **Scenario 3: New Resource Required**
```bash
# DevOps adds MongoDB to existing infrastructure
sc assistant devops resources --add mongodb --env staging,production
sc provision -s infrastructure -e staging,production

# Developers update their apps to use MongoDB
sc assistant dev setup --update --add-resource mongodb
sc deploy -e staging
```

## 🔍 Validation Steps

### **DevOps Validation**
```bash
# Verify infrastructure deployment by checking files
ls -la .sc/stacks/infrastructure/

# Verify infrastructure configuration
cat .sc/stacks/infrastructure/server.yaml | grep -A 5 "resources:"

# Verify secrets are properly configured
sc secrets list
```

### **Developer Validation**  
```bash
# Verify local development works
docker-compose up -d
curl http://localhost:3000/health

# Verify staging deployment works
sc deploy -e staging  
curl https://staging-api.mycompany.com/health

# Verify production deployment works
sc deploy -e production
curl https://api.mycompany.com/health
```

## ❗ Common Issues

### **DevOps Issues**
```bash
# Issue: Cloud credentials not working
# Solution: Re-add credentials with correct permissions
sc secrets add aws-access-key --overwrite

# Issue: Resource naming conflicts  
# Solution: Use unique prefixes
sc assistant devops setup --prefix mycompany

# Issue: Environment isolation problems
# Solution: Verify separate resource names per environment
```

### **Developer Issues**
```bash
# Issue: Parent stack not found
# Solution: Ensure DevOps has deployed infrastructure first
ls -la .sc/stacks/  # Should show "infrastructure" directory

# Issue: Resource references not working
# Solution: Check resource names match server.yaml
sc assistant search "template placeholders"

# Issue: Local development not working
# Solution: Check docker-compose services are running
docker-compose ps
docker-compose logs
```

## 🚀 Next Steps

### **For DevOps Teams**
1. **[Advanced DevOps Configuration →](devops-mode.md)**
2. **[Multi-cloud Setup →](examples/multi-cloud.md)**
3. **[Monitoring and Alerting →](examples/monitoring.md)**
4. **[Security Best Practices →](examples/security.md)**

### **For Developer Teams**
1. **[Advanced Developer Workflows →](developer-mode.md)**
2. **[Framework-specific Guides →](examples/)**
3. **[Local Development Tips →](examples/local-dev.md)**
4. **[Debugging and Troubleshooting →](troubleshooting.md)**

### **For Both Teams**
1. **[MCP Integration with IDEs →](mcp-integration.md)**
2. **[CI/CD Pipeline Setup →](examples/cicd.md)**  
3. **[Performance Optimization →](examples/performance.md)**
4. **[Cost Management →](examples/cost-optimization.md)**

## 💬 Getting Help

### **Documentation Search**
```bash
# Search for specific topics
sc assistant search "database configuration"
sc assistant search "scaling applications"  
sc assistant search "troubleshooting deployment"
```

### **Interactive Help**
```bash
# Start MCP server for IDE integration
sc assistant mcp --port 9999

# Use interactive chat (Phase 3)
sc assistant chat
```

### **Community Support**
- **[Simple Container Documentation](https://docs.simple-container.com)**
- **[GitHub Issues](https://github.com/simple-container-com/api/issues)**
- **[Community Slack](https://slack.simple-container.com)**
- **[Stack Overflow Tag: simple-container](https://stackoverflow.com/questions/tagged/simple-container)**

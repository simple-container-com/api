# Developer Mode

Developer Mode is designed for **application teams** who need to set up their applications to work with existing Simple Container infrastructure managed by DevOps teams.

## 🎯 Overview

**Developer Mode Focus:**
- Generate `client.yaml` configurations for your applications
- Create optimized `docker-compose.yaml` for local development
- Generate `Dockerfile` based on detected tech stack
- Connect to shared resources provided by DevOps team
- Handle application-specific scaling and deployment settings

## 🚀 Quick Start

### Basic Project Setup
```bash
# Navigate to your project directory
cd my-awesome-app

# Auto-generate configurations based on project analysis
sc assistant dev setup

# Interactive mode for guided setup
sc assistant dev setup --interactive

# Specify target environment
sc assistant dev setup --env staging
```

### Analyze Your Project First
```bash
# Get detailed project analysis
sc assistant dev analyze

# Output example:
# 🔍 Analyzing project at: .
#
# 🎯 Analysis Results:
#    Language: Node.js (18.x)
#    Framework: Express.js
#    Architecture: REST API
#    Dependencies: PostgreSQL, Redis
#    Recommendations: 3 resources, 2 optimizations
```

## 🔍 Project Analysis Engine

Developer Mode automatically detects:

### **Supported Languages & Frameworks**

| Language    | Frameworks Detected                                | Configuration Files                              |
|-------------|----------------------------------------------------|--------------------------------------------------|
| **Node.js** | Express, Koa, Fastify, NestJS, Next.js, React, Vue | `package.json`, `package-lock.json`              |
| **Python**  | Django, Flask, FastAPI, Tornado                    | `requirements.txt`, `setup.py`, `pyproject.toml` |
| **Go**      | Gin, Echo, Fiber, Gorilla/Mux                      | `go.mod`, `go.sum`                               |
| **Java**    | Spring Boot, Quarkus, Micronaut                    | `pom.xml`, `build.gradle`                        |
| **PHP**     | Laravel, Symfony, CodeIgniter                      | `composer.json`                                  |
| **Ruby**    | Rails, Sinatra, Hanami                             | `Gemfile`, `Gemfile.lock`                        |

### **Database Detection**
- **PostgreSQL**: `pg`, `psycopg2`, `sequelize`, `typeorm`
- **MongoDB**: `mongoose`, `pymongo`, `mongo-go-driver`
- **MySQL**: `mysql2`, `pymysql`, `gorm`
- **Redis**: `redis`, `ioredis`, `go-redis`
- **SQLite**: `sqlite3`, Development databases

### **Architecture Patterns**
- **Microservice**: Multiple service directories, docker-compose with multiple services
- **Monolith**: Single large application with MVC structure
- **Static Site**: HTML/CSS/JS, static site generators (Gatsby, Next.js export)
- **Serverless**: Lambda functions, Vercel/Netlify configs

## 📁 Generated Files

### 1. **client.yaml** - Application Configuration
```yaml
schemaVersion: 1.0

stacks:
  my-awesome-app:
    type: cloud-compose
    parent: infrastructure  # References DevOps-managed resources
    parentEnv: staging
    config:
      dockerComposeFile: docker-compose.yaml  # REQUIRED: Reference to local compose file
      uses: [postgres-db, redis-cache]  # Shared resources from parent
      runs: [web-app, worker]  # Services from docker-compose
      domain: my-awesome-app.mycompany.com  # Optional: DNS domain (requires registrar)
      scale:
        min: 1
        max: 3
      env:
        NODE_ENV: production  # Non-sensitive environment variables only
        PORT: 3000
        # Database connections use auto-injected environment variables:
        # PostgreSQL: PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSWORD
        # Redis: REDIS_HOST, REDIS_PORT
      secrets:
        JWT_SECRET: "${secret:jwt-secret}"
        API_KEY: "${secret:third-party-api-key}"
        DATABASE_URL: "${secret:database-url}"  # Connection strings in secrets
        REDIS_URL: "${secret:redis-url}"        # Not in env section
```

### 2. **docker-compose.yaml** - Local Development
```yaml
version: '3.8'

services:
  web-app:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development
      - DATABASE_URL=postgresql://user:pass@postgres:5432/myapp
      - REDIS_URL=redis://redis:6379
    depends_on:
      - postgres
      - redis
    volumes:
      - .:/app
      - node_modules:/app/node_modules

  worker:
    build: .
    command: npm run worker
    environment:
      - NODE_ENV=development
      - DATABASE_URL=postgresql://user:pass@postgres:5432/myapp
      - REDIS_URL=redis://redis:6379
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: user
      POSTGRES_PASSWORD: pass
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  node_modules:
  postgres_data:
```

### 3. **Dockerfile** - Optimized Container Image
```dockerfile
# Multi-stage build for Node.js
FROM node:18-alpine AS dependencies
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production --silent

FROM node:18-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci --silent
COPY . .
RUN npm run build

FROM node:18-alpine AS runtime
WORKDIR /app
RUN addgroup -g 1001 -S nodejs && \
    adduser -S nextjs -u 1001
COPY --from=dependencies /app/node_modules ./node_modules
COPY --from=build --chown=nextjs:nodejs /app/dist ./dist
COPY --from=build --chown=nextjs:nodejs /app/package*.json ./

USER nextjs
EXPOSE 3000
CMD ["npm", "start"]
```

## 🎛️ Command Options

### **Setup Command Options**
```bash
# Basic setup
sc assistant dev setup

# Interactive mode with prompts
sc assistant dev setup --interactive

# Specify target environment
sc assistant dev setup --env production

# Skip certain file generation
sc assistant dev setup --skip-dockerfile --skip-compose

# Use specific parent stack
sc assistant dev setup --parent my-infrastructure

# Override detected language/framework
sc assistant dev setup --language python --framework django

# Generate for specific cloud provider
sc assistant dev setup --cloud gcp
```

### **Analyze Command Options**
```bash
# Basic analysis
sc assistant dev analyze

# Detailed output with recommendations
sc assistant dev analyze --detailed

# Export analysis to file
sc assistant dev analyze --output analysis.json

# Analyze specific directory
sc assistant dev analyze --path ./services/api
```

## 🔗 Connecting to Shared Resources

Developer Mode assumes DevOps has already set up shared infrastructure. Your `client.yaml` references these resources:

### **Resource Consumption Pattern**
```yaml
# In your client.yaml
config:
  uses: [postgres-db, redis-cache, s3-uploads]  # Managed by DevOps
  env:
    # Non-sensitive configuration only
    NODE_ENV: production
    PORT: 3000
    # Database connections use auto-injected environment variables:
    # PostgreSQL: PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSWORD
    # Redis: REDIS_HOST, REDIS_PORT
    # S3: S3_BUCKET, S3_REGION, S3_ACCESS_KEY, S3_SECRET_KEY
  secrets:
    # Sensitive connection strings go in secrets section
    DATABASE_URL: "${secret:database-url}"
    REDIS_URL: "${secret:redis-url}"
    S3_ACCESS_KEY: "${secret:s3-access-key}"
```

### **Parent Stack Reference**
```yaml
# References DevOps-managed server.yaml
stacks:
  my-app:
    parent: infrastructure  # Points to .sc/stacks/infrastructure/server.yaml
    parentEnv: staging      # Uses staging environment resources
```

## 🎯 Framework-Specific Optimizations

### **Node.js Projects**
- **Package Manager Detection**: npm, yarn, pnpm
- **Build Tools**: Webpack, Vite, Rollup, esbuild
- **Frameworks**: Express routing, Next.js static export, React SPA
- **Testing**: Jest, Mocha, Cypress configurations

### **Python Projects**
- **Dependency Management**: pip, pipenv, poetry, conda
- **WSGI/ASGI**: Gunicorn, Uvicorn, Hypercorn
- **Frameworks**: Django settings, Flask configs, FastAPI metadata
- **Virtual Environments**: Automatic detection and setup

### **Go Projects**
- **Module System**: go.mod parsing, vendor detection
- **Build Optimization**: Multi-stage Docker builds, CGO_ENABLED=0
- **Frameworks**: Gin middleware, Echo routing, Fiber configuration
- **Performance**: Scratch-based final images, static compilation

## 🚀 Deployment Strategies

### **Local Development**
```bash
# Start local environment
docker-compose up -d

# Run application
npm run dev  # or python manage.py runserver, go run main.go
```

### **Staging Deployment**
```bash
# Deploy to staging (uses shared staging resources)
sc deploy -e staging

# Application scaling is configured in client.yaml config.scale section
# Edit client.yaml to update scaling configuration
```

### **Production Deployment**
```bash
# Deploy to production (uses shared production resources)
sc deploy -e production

# Verify deployment is working
curl https://my-app.yourcompany.com/health
```

## 💡 Best Practices

### **Configuration Management**
- ✅ **Use Environment Variables**: Never hardcode secrets or config
- ✅ **Reference Shared Resources**: Use `uses: [resource-name]` array for resource consumption
- ✅ **Environment Separation**: Different configs for dev/staging/prod
- ✅ **Secret Management**: Use `${secret:name}` for sensitive data (connection strings, API keys)
- ✅ **Security Separation**: env: for non-sensitive config, secrets: for sensitive data

### **Docker Optimization**
- ✅ **Multi-stage Builds**: Separate build and runtime environments
- ✅ **Layer Caching**: Order instructions for optimal caching
- ✅ **Non-root Users**: Run containers as non-privileged users
- ✅ **Health Checks**: Add proper health check endpoints

### **Local Development**
- ✅ **Volume Mounts**: Enable hot reloading with volume mounts
- ✅ **Service Dependencies**: Use `depends_on` for service ordering
- ✅ **Port Mapping**: Consistent port mapping across environments
- ✅ **Environment Parity**: Match production environment closely

## 🔍 Troubleshooting

### **Analysis Issues**
```bash
# No technology detected
sc assistant dev analyze --verbose
# Check if package.json, requirements.txt, go.mod, etc. exist

# Wrong framework detected
sc assistant dev setup --framework express --skip-analysis

# Complex monorepo structure
sc assistant dev analyze --path ./services/api
```

### **Generated Configuration Issues**
```bash
# Parent stack not found
# Ensure DevOps has set up infrastructure first
sc assistant devops setup  # Run this first

# Resource references not working
# Check that resource names match server.yaml
sc assistant search "resource placeholders"
```

### **Docker Issues**
```bash
# Build failures
# Check generated Dockerfile syntax
docker build -t test .

# Multi-platform builds
sc assistant dev setup --platform linux/amd64,linux/arm64
```

## 📋 Examples by Framework

### **Express.js API**
- [Express REST API with PostgreSQL](examples/express-api.md)
- [Express GraphQL with MongoDB](examples/express-graphql.md)
- [Express Microservices](examples/express-microservices.md)

### **Django Application**
- [Django REST API with PostgreSQL](examples/django-api.md)
- [Django Monolith with Redis](examples/django-monolith.md)
- [Django with Celery Workers](examples/django-celery.md)

### **Go Applications**
- [Gin REST API](examples/go-gin-api.md)
- [Go Microservices with gRPC](examples/go-microservices.md)
- [Go CLI Tools](examples/go-cli.md)

## 🔗 Next Steps

1. **[Set up your first project →](getting-started.md#developer-setup)**
2. **[Learn about resource consumption →](../concepts/template-placeholders.md)**
3. **[Deploy to staging →](../guides/deployment.md)**
4. **[Scale your application →](../advanced/scaling-advantages.md)**

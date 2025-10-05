# Node.js Express API Example

Complete example of using Simple Container AI Assistant to set up a Node.js Express API with PostgreSQL database and Redis cache.

## 🎯 Scenario

You're building a REST API for a task management application with:
- **Node.js** with Express.js framework
- **PostgreSQL** database for task storage
- **Redis** for session management and caching
- **JWT** authentication
- **Local development** with Docker Compose
- **Cloud deployment** to AWS ECS Fargate

## 📂 Project Structure

```
task-management-api/
├── src/
│   ├── controllers/
│   │   ├── authController.js
│   │   └── taskController.js
│   ├── models/
│   │   ├── User.js
│   │   └── Task.js
│   ├── middleware/
│   │   └── auth.js
│   ├── routes/
│   │   ├── auth.js
│   │   └── tasks.js
│   └── app.js
├── package.json
├── server.js
└── README.md
```

## 🚀 Step-by-Step Setup

### Step 1: Initialize Project

```bash
# Create project directory
mkdir task-management-api
cd task-management-api

# Initialize Node.js project
npm init -y

# Install dependencies
npm install express pg redis jsonwebtoken bcrypt cors helmet
npm install --save-dev nodemon jest supertest

# Create basic project structure
mkdir -p src/{controllers,models,middleware,routes}
```

### Step 2: Create Basic Express App

```javascript
// package.json
{
  "name": "task-management-api",
  "version": "1.0.0",
  "description": "Task management REST API",
  "main": "server.js",
  "scripts": {
    "start": "node server.js",
    "dev": "nodemon server.js",
    "test": "jest"
  },
  "dependencies": {
    "express": "^4.18.0",
    "pg": "^8.8.0",
    "redis": "^4.3.0",
    "jsonwebtoken": "^8.5.1",
    "bcrypt": "^5.1.0",
    "cors": "^2.8.5",
    "helmet": "^6.0.1"
  },
  "devDependencies": {
    "nodemon": "^2.0.20",
    "jest": "^28.1.3",
    "supertest": "^6.2.4"
  },
  "engines": {
    "node": ">=18.0.0"
  }
}
```

```javascript
// server.js
const express = require('express');
const cors = require('cors');
const helmet = require('helmet');

const app = express();

// Middleware
app.use(helmet());
app.use(cors());
app.use(express.json());

// Health check
app.get('/health', (req, res) => {
  res.json({ 
    status: 'healthy', 
    timestamp: new Date().toISOString(),
    version: process.env.npm_package_version 
  });
});

// Routes
app.use('/api/auth', require('./src/routes/auth'));
app.use('/api/tasks', require('./src/routes/tasks'));

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
  console.log(`Server running on port ${PORT}`);
});

module.exports = app;
```

### Step 3: AI Assistant Analysis

```bash
# Let AI Assistant analyze the project
sc assistant dev analyze --detailed

# Expected output:
# 🔍 Project Analysis Results
# 
# 📊 Technology Stack:
#    Language:     Node.js (>=18.0.0)
#    Framework:    Express.js
#    Architecture: REST API
#    Confidence:   96%
# 
# 📦 Dependencies Detected:
#    ✅ express      ^4.18.0   (Web framework)
#    ✅ pg           ^8.8.0    (PostgreSQL client)
#    ✅ redis        ^4.3.0    (Redis client)
#    ✅ jsonwebtoken ^8.5.1    (JWT authentication)
#    ✅ bcrypt       ^5.1.0    (Password hashing)
# 
# 🎯 Recommendations:
#    🔹 Add PostgreSQL database resource
#    🔹 Add Redis cache resource
#    🔹 Configure health check endpoint ✅
#    🔹 Add request logging middleware
#    🔹 Set up JWT secret management
```

### Step 4: Generate Configuration Files

```bash
# Generate Simple Container configuration
sc assistant dev setup --interactive

# Interactive prompts:
# ? Target environment: staging
# ? Use detected PostgreSQL dependency? Yes
# ? Use detected Redis dependency? Yes
# ? Generate Dockerfile? Yes
# ? Generate docker-compose.yaml for local development? Yes
# ? Configure health checks? Yes
```

### Step 5: Review Generated Files

#### **Generated client.yaml**
```yaml
schemaVersion: 1.0

stacks:
  task-management-api:
    type: cloud-compose
    parent: infrastructure
    parentEnv: staging
    config:
      # Reference shared resources from DevOps team
      uses: [postgres-db, redis-cache]
      
      # Services from docker-compose.yaml
      runs: [api]
      
      # Scaling configuration
      scale:
        min: 2
        max: 10
      
      # Environment variables
      env:
        NODE_ENV: production
        PORT: 3000
        DATABASE_URL: "${resource:postgres-db.url}"
        REDIS_URL: "${resource:redis-cache.url}"
        JWT_SECRET: "${secret:jwt-secret}"
        
      # Health check configuration
      healthCheck:
        path: "/health"
        port: 3000
        initialDelaySeconds: 30
        periodSeconds: 10
        
      # Secrets
      secrets:
        JWT_SECRET: "${secret:jwt-secret}"
        DATABASE_PASSWORD: "${secret:staging-db-password}"
```

#### **Generated docker-compose.yaml**
```yaml
version: '3.8'

services:
  api:
    build: .
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=development
      - PORT=3000
      - DATABASE_URL=postgresql://taskuser:taskpass@postgres:5432/taskdb
      - REDIS_URL=redis://redis:6379
      - JWT_SECRET=dev-jwt-secret-key-123
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    volumes:
      - .:/app:delegated
      - node_modules:/app/node_modules
    command: npm run dev

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: taskdb
      POSTGRES_USER: taskuser
      POSTGRES_PASSWORD: taskpass
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./sql/init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U taskuser -d taskdb"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  node_modules:
  postgres_data:
  redis_data:
```

#### **Generated Dockerfile**
```dockerfile
# Multi-stage build for Node.js
FROM node:18-alpine AS dependencies

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create app directory
WORKDIR /app

# Copy package files
COPY package*.json ./

# Install production dependencies
RUN npm ci --only=production --silent && npm cache clean --force

# Development stage
FROM node:18-alpine AS development
WORKDIR /app
COPY package*.json ./
RUN npm ci --silent
COPY . .
CMD ["npm", "run", "dev"]

# Build stage
FROM dependencies AS build
COPY . .
# If you have a build step, uncomment:
# RUN npm run build

# Production stage
FROM node:18-alpine AS production

# Install dumb-init
RUN apk add --no-cache dumb-init

# Create non-root user
RUN addgroup -g 1001 -S nodejs && \
    adduser -S nodeuser -u 1001

WORKDIR /app

# Copy dependencies
COPY --from=dependencies --chown=nodeuser:nodejs /app/node_modules ./node_modules

# Copy application code
COPY --chown=nodeuser:nodejs . .

# Switch to non-root user
USER nodeuser

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/health', (res) => { process.exit(res.statusCode === 200 ? 0 : 1) })"

# Start the application
ENTRYPOINT ["dumb-init", "--"]
CMD ["npm", "start"]
```

### Step 6: Database Schema Setup

```sql
-- sql/init.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tasks table
CREATE TABLE tasks (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  title VARCHAR(255) NOT NULL,
  description TEXT,
  status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed')),
  priority VARCHAR(10) DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high')),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  due_date TIMESTAMP,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for better performance
CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_due_date ON tasks(due_date);
```

### Step 7: Local Development

```bash
# Start local development environment
docker-compose up -d

# Check services are running
docker-compose ps

# Expected output:
# NAME                    SERVICE     STATUS      PORTS
# task-api-api-1         api         running     0.0.0.0:3000->3000/tcp
# task-api-postgres-1    postgres    running     0.0.0.0:5432->5432/tcp  
# task-api-redis-1       redis       running     0.0.0.0:6379->6379/tcp

# Test the application
curl http://localhost:3000/health

# Expected response:
# {"status":"healthy","timestamp":"2024-10-05T13:19:02.123Z","version":"1.0.0"}

# View logs
docker-compose logs api

# Stop services
docker-compose down
```

### Step 8: Deploy to Staging

```bash
# Ensure secrets are configured
sc secrets list
# Should show: jwt-secret, staging-db-password

# If missing, add secrets
sc secrets add jwt-secret
# Enter value: your-super-secret-jwt-key-here

# Deploy to staging
sc deploy -e staging

# Verify staging deployment is working
curl https://staging-api.yourcompany.com/health

# Check application logs via Docker (if needed for troubleshooting)
docker logs task-management-api_api_1

# Test staging deployment
curl https://staging-api.yourcompany.com/health
```

### Step 9: Production Deployment

```bash
# Add production secrets
sc secrets add prod-jwt-secret
sc secrets add prod-db-password

# Deploy to production
sc deploy -e production

# Verify production deployment is working
curl https://api.yourcompany.com/health

# Check application logs via Docker (if needed for troubleshooting)
docker logs task-management-api_web-app_1
```

## 🧪 Testing the API

### Local Testing
```bash
# Register a new user
curl -X POST http://localhost:3000/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "John Doe",
    "email": "john@example.com", 
    "password": "securepassword123"
  }'

# Login
curl -X POST http://localhost:3000/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john@example.com",
    "password": "securepassword123"
  }'

# Create a task (use JWT token from login response)
curl -X POST http://localhost:3000/api/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "title": "Complete project setup",
    "description": "Set up Simple Container configuration",
    "priority": "high",
    "due_date": "2024-10-10T18:00:00Z"
  }'

# Get tasks
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  http://localhost:3000/api/tasks
```

## 📊 Monitoring and Observability

### Application Metrics
```javascript
// src/middleware/metrics.js
const express = require('express');
const router = express.Router();

let requestCount = 0;
let requestDuration = [];

router.use((req, res, next) => {
  const start = Date.now();
  requestCount++;
  
  res.on('finish', () => {
    const duration = Date.now() - start;
    requestDuration.push(duration);
    
    // Keep only last 1000 requests for memory efficiency
    if (requestDuration.length > 1000) {
      requestDuration = requestDuration.slice(-1000);
    }
  });
  
  next();
});

// Metrics endpoint
router.get('/metrics', (req, res) => {
  const avgDuration = requestDuration.length > 0 
    ? requestDuration.reduce((a, b) => a + b) / requestDuration.length 
    : 0;
    
  res.json({
    requests_total: requestCount,
    avg_response_time_ms: Math.round(avgDuration),
    active_connections: process._getActiveHandles().length,
    memory_usage: process.memoryUsage(),
    uptime_seconds: process.uptime()
  });
});

module.exports = router;
```

### Health Checks
```javascript
// src/middleware/health.js
const { Client } = require('pg');
const redis = require('redis');

async function healthCheck(req, res) {
  const checks = {
    database: false,
    redis: false,
    memory: false
  };

  try {
    // Check PostgreSQL connection
    const client = new Client({
      connectionString: process.env.DATABASE_URL
    });
    await client.connect();
    await client.query('SELECT 1');
    await client.end();
    checks.database = true;
  } catch (error) {
    console.error('Database health check failed:', error.message);
  }

  try {
    // Check Redis connection
    const redisClient = redis.createClient({
      url: process.env.REDIS_URL
    });
    await redisClient.connect();
    await redisClient.ping();
    await redisClient.quit();
    checks.redis = true;
  } catch (error) {
    console.error('Redis health check failed:', error.message);
  }

  // Check memory usage
  const memUsage = process.memoryUsage();
  checks.memory = memUsage.heapUsed < (512 * 1024 * 1024); // Less than 512MB

  const isHealthy = Object.values(checks).every(check => check === true);

  res.status(isHealthy ? 200 : 503).json({
    status: isHealthy ? 'healthy' : 'unhealthy',
    timestamp: new Date().toISOString(),
    version: process.env.npm_package_version,
    checks
  });
}

module.exports = { healthCheck };
```

## 🔐 Security Best Practices

### Environment Variables
```bash
# .env.example
NODE_ENV=development
PORT=3000
DATABASE_URL=postgresql://user:password@localhost:5432/dbname
REDIS_URL=redis://localhost:6379
JWT_SECRET=your-super-secret-jwt-key
JWT_EXPIRATION=24h
BCRYPT_ROUNDS=12
CORS_ORIGIN=http://localhost:3000
RATE_LIMIT_WINDOW_MS=900000
RATE_LIMIT_MAX_REQUESTS=100
```

### Rate Limiting
```javascript
// src/middleware/rateLimiter.js
const rateLimit = require('express-rate-limit');

const createRateLimiter = (windowMs = 15 * 60 * 1000, max = 100) => {
  return rateLimit({
    windowMs,
    max,
    message: {
      error: 'Too many requests from this IP',
      retryAfter: Math.ceil(windowMs / 1000)
    },
    standardHeaders: true,
    legacyHeaders: false
  });
};

module.exports = {
  globalLimiter: createRateLimiter(),
  authLimiter: createRateLimiter(15 * 60 * 1000, 5), // 5 attempts per 15 min
  apiLimiter: createRateLimiter(15 * 60 * 1000, 1000) // 1000 requests per 15 min
};
```

## 🚀 Performance Optimization

### Connection Pooling
```javascript
// src/config/database.js
const { Pool } = require('pg');

const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  max: 20, // Maximum number of connections
  idleTimeoutMillis: 30000,
  connectionTimeoutMillis: 2000,
  ssl: process.env.NODE_ENV === 'production' ? { rejectUnauthorized: false } : false
});

module.exports = pool;
```

### Redis Caching
```javascript
// src/config/redis.js
const redis = require('redis');

const client = redis.createClient({
  url: process.env.REDIS_URL,
  retry_strategy: (options) => {
    if (options.error && options.error.code === 'ECONNREFUSED') {
      return new Error('Redis server connection refused');
    }
    if (options.total_retry_time > 1000 * 60 * 60) {
      return new Error('Redis retry time exhausted');
    }
    if (options.attempt > 10) {
      return undefined;
    }
    return Math.min(options.attempt * 100, 3000);
  }
});

client.on('error', (err) => console.error('Redis Client Error', err));
client.on('connect', () => console.log('Redis Client Connected'));

module.exports = client;
```

## 📈 Scaling Considerations

### Auto-scaling Configuration
```yaml
# In client.yaml, configure auto-scaling
config:
  scale:
    min: 2
    max: 20
    targetCPU: 70
    targetMemory: 80
    scaleUpCooldown: 300   # 5 minutes
    scaleDownCooldown: 600 # 10 minutes
```

### Load Testing
```bash
# Install artillery for load testing
npm install -g artillery

# Create load test config
cat > load-test.yml << EOF
config:
  target: 'http://localhost:3000'
  phases:
    - duration: 60
      arrivalRate: 10
    - duration: 120  
      arrivalRate: 50
scenarios:
  - name: "API workflow"
    flow:
      - get:
          url: "/health"
      - post:
          url: "/api/auth/register"
          json:
            name: "Test User"
            email: "test{{ $randomNumber() }}@example.com"
            password: "testpass123"
EOF

# Run load test
artillery run load-test.yml
```

## 🔗 Next Steps

1. **Add Authentication Middleware**
2. **Implement Comprehensive Logging**
3. **Set up API Documentation with Swagger**
4. **Add Integration Tests**
5. **Configure CI/CD Pipeline**
6. **Set up Monitoring and Alerting**

## 📚 Related Examples

- **[Python Django App](python-django-app.md)** - Similar pattern with Django
- **[Go Microservice](go-microservice.md)** - Microservice with gRPC
- **[React Frontend](react-frontend.md)** - Frontend for this API
- **[CI/CD Pipeline](cicd-pipeline.md)** - Automated deployment

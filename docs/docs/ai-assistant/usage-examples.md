# AI Assistant Usage Examples

This guide provides comprehensive examples of how to use the Simple Container AI Assistant across all its modes and features.

## 🚀 Quick Start

The AI Assistant provides three main interfaces:

1. **Interactive Chat** - Natural language conversations
2. **Command-line Tools** - Direct CLI commands  
3. **IDE Integration** - Via MCP protocol (Windsurf, Cursor, etc.)

## 💬 Interactive Chat Mode

### Basic Chat Session

Start an interactive chat session:

```bash
sc assistant chat
```

**Example conversation:**

```
🤖 Simple Container AI Assistant
Type 'help' for available commands or ask any question about Simple Container.

You: How do I create a client.yaml file for a Go application?

AI: I'll help you create a client.yaml file for your Go application. Here's a typical structure:

```yaml
schemaVersion: 1.0

stacks:
  production:
    type: cloud-compose
    parent: infrastructure
    config:
      runs: [app]
      scale:
        min: 1
        max: 3
      env:
        GO_ENV: production
        PORT: 8080
      secrets:
        DATABASE_URL: "${secret:db-url}"
```

This configuration:
- Uses cloud-compose deployment type
- References an "infrastructure" parent stack
- Scales between 1-3 instances
- Sets environment variables for Go
- Uses secrets for sensitive data

You: Can you show me Docker Compose setup too?

AI: Here's a docker-compose.yaml for local development:

```yaml
version: '3.8'

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - GO_ENV=development
      - PORT=8080
    volumes:
      - .:/app
    depends_on:
      - db
      
  db:
    image: postgres:13
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: user
      POSTGRES_PASSWORD: password
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

You: /help

Available Commands:
- /search <query>     - Search documentation  
- /analyze           - Analyze current project
- /setup             - Interactive project setup
- /help              - Show this help
- exit               - Leave chat

You: /search postgres configuration

🔍 Found 3 relevant documents:

1. **PostgreSQL RDS Setup** (similarity: 0.94)
   Guide for configuring AWS RDS PostgreSQL with Simple Container...

2. **Database Connection Patterns** (similarity: 0.91)  
   Best practices for connecting applications to databases...

3. **Secrets Management** (similarity: 0.88)
   How to securely store database credentials...

You: /analyze

📂 Analyzing current project...

Detected:
- Language: Go
- Framework: gorilla/mux  
- Architecture: REST API
- Confidence: 95%

Recommendations:
- Create Go-optimized Dockerfile
- Use cloud-compose deployment
- Set up PostgreSQL database
- Configure environment variables

You: exit

👋 Thanks for using Simple Container AI Assistant!
```

### Chat with OpenAI Integration

For enhanced responses, configure your OpenAI API key:

```bash
# Option 1: Environment variable
export OPENAI_API_KEY=sk-your-key-here
sc assistant chat

# Option 2: Command line flag
sc assistant chat --openai-key sk-your-key-here

# Option 3: Interactive prompt (secure)
sc assistant chat
# AI will prompt for key if not found
```

## 🔍 Command-Line Search

### Documentation Search

Search the documentation directly from command line:

```bash
# Basic search
sc assistant search "client.yaml example"

# Limited results
sc assistant search "docker compose" --limit 3

# Verbose mode (shows debug info)
sc assistant search "kubernetes deployment" --verbose

# Filter by document type
sc assistant search "postgres" --type guides
```

**Example output:**

```
🔍 Searching documentation for: client.yaml example

Found 2 relevant documents:

1. Getting Started with Simple Container
   Path: getting-started/index.md
   Type: documentation
   Similarity: 0.954
   Preview: Welcome to Simple Container! This section will help you get up and running...

2. Quick Start Guide  
   Path: getting-started/quick-start.md
   Type: documentation
   Similarity: 0.953
   Preview: This guide will help you deploy your first application...
```

## 👩‍💻 Developer Mode

### Project Setup and Analysis

```bash
# Analyze current project and generate files
sc assistant dev setup

# Analyze specific directory
sc assistant dev analyze /path/to/project

# Setup with specific parameters
sc assistant dev setup --env production --parent infrastructure --language go

# Skip project analysis (faster)
sc assistant dev setup --skip-analysis --language python --framework flask
```

**Example Developer Mode Session:**

```bash
$ sc assistant dev setup

🚀 Simple Container Developer Mode - Project Setup
📂 Project path: /home/user/my-api

🔍 Analyzing project...
   Language:     Go
   Framework:    gorilla/mux
   Version:      1.21
   Architecture: REST API
   Confidence:   95%

🎯 Recommendations:
   🔹 Create Go Dockerfile (high)
   🔹 Set up PostgreSQL database (medium)
   🔹 Configure environment variables (medium)
   🔹 Add health check endpoints (low)

📝 Generating configuration files...
   📄 Generating client.yaml... ✓
   📄 Generating docker-compose.yaml... ✓
   📄 Generating Dockerfile... ✓

📁 Generated files:
   • .sc/stacks/client.yaml     - Simple Container configuration
   • docker-compose.yaml        - Local development environment  
   • Dockerfile                 - Container image definition

🚀 Next steps:
   1. Start local development: docker-compose up -d
   2. Deploy to production:     sc deploy -e production
   3. Set up secrets:          sc secrets add .sc/stacks/secrets.yaml

💡 Recommendations:
   • Configure PostgreSQL database in parent stack
   • Set up environment-specific configurations
   • Add health check endpoint to your Go application
```

### Generated Files Examples

**Generated client.yaml:**

```yaml
schemaVersion: 1.0

stacks:
  production:
    type: cloud-compose
    parent: infrastructure
    parentEnv: production
    config:
      runs: [app]
      scale:
        min: 2
        max: 10
      env:
        GO_ENV: production
        PORT: 8080
        LOG_LEVEL: info
      secrets:
        DATABASE_URL: "${secret:db-url}"
        JWT_SECRET: "${secret:jwt-secret}"
      dependencies:
        - postgres
```

**Generated Dockerfile:**

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Runtime stage  
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8080
CMD ["./main"]
```

**Generated docker-compose.yaml:**

```yaml
version: '3.8'

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - GO_ENV=development
      - PORT=8080
      - DATABASE_URL=postgres://user:password@db:5432/myapp
    volumes:
      - .:/app
    depends_on:
      - db
      
  db:
    image: postgres:13-alpine
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: user
      POSTGRES_PASSWORD: password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

volumes:
  postgres_data:
```

## 🛠️ DevOps Mode

### Infrastructure Management

```bash
# Interactive infrastructure setup
sc assistant devops setup

# Resource management
sc assistant devops resources --list
sc assistant devops resources --add postgres --env production
sc assistant devops resources --remove redis --env staging
sc assistant devops resources --update s3-bucket --env production

# Secrets management
sc assistant devops secrets --list
sc assistant devops secrets --add database-url
sc assistant devops secrets --rotate jwt-secret
```

**Example DevOps Mode Session:**

```bash
$ sc assistant devops setup

🛠️  Simple Container DevOps Mode - Infrastructure Setup
📂 Project path: /home/user/my-infrastructure

🏗️  Infrastructure Configuration:
   Provider:     AWS
   Region:       us-east-1
   Environment:  production
   
📋 What would you like to set up?
   1. 🗄️  Database (PostgreSQL/MySQL)
   2. 🪣 Storage (S3 Bucket)
   3. 🔐 Secrets (KMS Key)
   4. 🌐 Load Balancer (ALB)
   5. 📊 Monitoring (CloudWatch)
   6. 🔧 All of the above
   
Select option [1-6]: 6

📝 Generating infrastructure configuration...
   📄 Generating server.yaml... ✓
   📄 Generating secrets.yaml... ✓  
   📄 Generating cfg.default.yaml... ✓

📁 Generated files:
   • .sc/stacks/infrastructure/server.yaml  - Infrastructure definition
   • .sc/stacks/infrastructure/secrets.yaml - Authentication & secrets
   • cfg.default.yaml                       - Default configuration

🚀 Next steps:
   1. Configure AWS credentials: aws configure
   2. Add secrets to Simple Container: sc secrets add .sc/stacks/infrastructure/secrets.yaml
   3. Deploy infrastructure: sc provision -s infrastructure -e production
   4. Verify deployment: sc deploy -e production --dry-run
```

## 🔌 IDE Integration (MCP Protocol)

### Windsurf IDE Setup

1. **Start MCP Server:**

```bash
sc assistant mcp --port 9999
```

2. **Configure Windsurf** (`~/.config/windsurf/mcp.json`):

```json
{
  "mcpServers": {
    "simple-container": {
      "command": "/usr/local/bin/sc",
      "args": ["assistant", "mcp", "--port", "9999"],
      "env": {},
      "capabilities": {
        "resources": {
          "documentation_search": {
            "description": "Search Simple Container documentation"
          },
          "project_analysis": {
            "description": "Analyze project structure and recommend configurations"
          },
          "resource_discovery": {
            "description": "List available Simple Container resources"
          }
        }
      }
    }
  }
}
```

3. **Use in Windsurf:**

- **@simple-container** "How do I deploy a Go app?"
- **@simple-container** "Search for PostgreSQL examples"
- **@simple-container** "What resources are available for AWS?"
- **@simple-container** "Analyze this project structure"

### MCP Direct API Usage

```bash
# Health check
curl http://localhost:9999/health

# Search documentation
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "search_documentation",
    "params": {
      "query": "docker compose setup",
      "limit": 3
    }
  }'

# Analyze project
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "analyze_project",
    "params": {
      "path": "/path/to/project"
    }
  }'

# Get supported resources
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "get_supported_resources",
    "params": {}
  }'
```

## 🎯 Advanced Usage Patterns

### Automation Scripts

**Auto-setup new projects:**

```bash
#!/bin/bash
# setup-new-project.sh

PROJECT_NAME=$1
LANGUAGE=$2

echo "🚀 Setting up new $LANGUAGE project: $PROJECT_NAME"

mkdir $PROJECT_NAME
cd $PROJECT_NAME

# Initialize git
git init

# Generate Simple Container configuration
sc assistant dev setup \
  --language $LANGUAGE \
  --env production \
  --parent infrastructure \
  --skip-analysis

# Generate README from AI Assistant
sc assistant chat --non-interactive << EOF
Generate a README.md for a $LANGUAGE project named $PROJECT_NAME with Simple Container deployment instructions.
EOF

echo "✅ Project $PROJECT_NAME setup complete!"
echo "📂 Files generated:"
ls -la
```

**Documentation search automation:**

```bash
#!/bin/bash
# search-docs.sh

QUERY="$1"
OUTPUT_FILE="search-results.md"

echo "# Documentation Search Results" > $OUTPUT_FILE
echo "Query: \"$QUERY\"" >> $OUTPUT_FILE
echo "Date: $(date)" >> $OUTPUT_FILE
echo "" >> $OUTPUT_FILE

sc assistant search "$QUERY" --limit 5 >> $OUTPUT_FILE

echo "📄 Results saved to $OUTPUT_FILE"
```

### CI/CD Integration

**GitHub Actions workflow:**

```yaml
# .github/workflows/simple-container.yml
name: Simple Container Deployment

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install Simple Container
        run: |
          curl -sSL https://get.simple-container.com | bash
          
      - name: Analyze Project
        run: |
          sc assistant dev analyze --format json > analysis.json
          cat analysis.json
          
      - name: Validate Configuration
        run: |
          sc validate -f .sc/stacks/client.yaml
          
      - name: Deploy (Production only)
        if: github.ref == 'refs/heads/main'
        run: |
          sc deploy -e production
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
```

## 🔧 Configuration Options

### Verbose Mode

Enable detailed logging for troubleshooting:

```bash
# All commands support --verbose flag
sc assistant search "postgres" --verbose
sc assistant dev setup --verbose
sc assistant chat --verbose
sc assistant mcp --verbose
```

### Custom Configuration

**Environment Variables:**

```bash
# OpenAI API key
export OPENAI_API_KEY=sk-your-key-here

# Custom MCP server settings
export SC_MCP_HOST=0.0.0.0
export SC_MCP_PORT=8888

# Verbose logging by default
export SC_VERBOSE=true
```

**Configuration Files:**

```yaml
# ~/.sc/config.yaml
ai_assistant:
  llm:
    provider: openai
    model: gpt-4
    temperature: 0.7
    max_tokens: 2048
  
  mcp:
    host: localhost
    port: 9999
    
  search:
    default_limit: 10
    similarity_threshold: 0.7
    
  dev_mode:
    auto_analyze: true
    skip_confirmation: false
    default_parent: infrastructure
```

## 🚨 Troubleshooting

### Common Issues

**"No documents loaded" error:**

```bash
# Check embedded documentation
sc assistant search "test" --verbose

# Should show: "Initialized documentation database with 30 documents"
```

**MCP server connection issues:**

```bash
# Check server status
curl http://localhost:9999/health

# Check if port is in use
lsof -i :9999

# Restart server with different port
sc assistant mcp --port 9998
```

**OpenAI API issues:**

```bash
# Test API key
sc assistant chat --openai-key sk-test-key

# Use fallback templates
sc assistant dev setup --no-llm
```

**File generation problems:**

```bash
# Check permissions
ls -la .sc/

# Create directory structure
mkdir -p .sc/stacks

# Generate with verbose output
sc assistant dev setup --verbose
```

### Debug Information

```bash
# Get AI Assistant version and capabilities
sc assistant --help

# Check embedded documentation status
sc assistant search "" --limit 0 --verbose

# Test MCP server capabilities
curl http://localhost:9999/capabilities | jq .

# Validate generated configurations
sc validate -f .sc/stacks/client.yaml
```

## 🎓 Best Practices

### Development Workflow

1. **Start with analysis:**
   ```bash
   sc assistant dev analyze
   ```

2. **Generate configuration:**
   ```bash
   sc assistant dev setup
   ```

3. **Test locally:**
   ```bash
   docker-compose up -d
   ```

4. **Deploy to staging:**
   ```bash
   sc deploy -e staging
   ```

5. **Use AI for questions:**
   ```bash
   sc assistant chat
   ```

### DevOps Workflow

1. **Set up infrastructure:**
   ```bash
   sc assistant devops setup
   ```

2. **Configure secrets:**
   ```bash
   sc secrets add .sc/stacks/infrastructure/secrets.yaml
   ```

3. **Provision infrastructure:**
   ```bash
   sc provision -s infrastructure -e production
   ```

4. **Monitor and maintain:**
   ```bash
   sc assistant devops resources --list
   ```

### IDE Integration

1. **Start MCP server:**
   ```bash
   sc assistant mcp --port 9999
   ```

2. **Configure IDE (Windsurf/Cursor)**

3. **Use @simple-container** for contextual help

---

## 📚 Additional Resources

- **[Getting Started Guide](../getting-started/index.md)** - Basic Simple Container setup
- **[AI Assistant Commands](commands.md)** - Complete command reference  
- **[Developer Mode Guide](developer-mode.md)** - Detailed developer workflow
- **[DevOps Mode Guide](devops-mode.md)** - Infrastructure management
- **[MCP Integration Guide](mcp-integration.md)** - IDE setup and usage
- **[Troubleshooting Guide](troubleshooting.md)** - Common issues and solutions

---

**Need help?** Ask the AI Assistant: `sc assistant chat` 🤖

# MCP Integration

The Model Context Protocol (MCP) integration allows external tools like **Windsurf**, **Cursor**, and other AI-powered IDEs to access Simple Container's knowledge base and project context.

## 🎯 Overview

**MCP Server Features:**
- **Semantic Documentation Search** - Query 10,000+ indexed documents
- **Project Context Analysis** - Get project structure and Simple Container configuration
- **Resource Information** - Access details about supported cloud resources
- **Configuration Validation** - Verify Simple Container configurations
- **Real-time Assistance** - Live help during development

## 🚀 Quick Start

### Start MCP Server
```bash
# Start on default port (9999)
sc assistant mcp

# Custom host and port
sc assistant mcp --host 0.0.0.0 --port 8080

# Start in stdio mode (for IDE integration)
sc assistant mcp --stdio

# Start with verbose logging
sc assistant mcp --verbose
```

### Test Server

#### **HTTP Mode Testing**
```bash
# Health check
curl http://localhost:9999/health

# Get capabilities
curl http://localhost:9999/capabilities | jq

# Test ping
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"ping","id":"test"}'
```

#### **Stdio Mode Testing (MCP Compliant)**
```bash
# Test full MCP initialization sequence
printf '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":1}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","method":"tools/list","id":2}\n' | sc assistant mcp --stdio

# Test tool execution
printf '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":1}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","method":"tools/call","params":{"name":"search_documentation","arguments":{"query":"kubernetes deployment"}},"id":3}\n' | sc assistant mcp --stdio

# Test resources
printf '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":1}\n{"jsonrpc":"2.0","method":"notifications/initialized"}\n{"jsonrpc":"2.0","method":"resources/list","id":4}\n' | sc assistant mcp --stdio
```

## 🔌 IDE Integrations

### **Windsurf IDE Integration**

#### **Option 1: Global MCP Configuration**

In Windsurf IDE Settings → MCP Servers, add:

```json
{
  "simple-container": {
    "command": "sc",
    "args": ["assistant", "mcp", "--stdio"],
    "env": {
      "PATH": "/usr/local/bin:/usr/bin:/bin"
    }
  }
}
```

**Note**: The MCP server now fully complies with MCP specification 2024-11-05, including:
- Proper initialization handshake (`initialize` → `notifications/initialized`)
- Standard MCP methods (`tools/list`, `tools/call`, `resources/list`, `resources/read`)
- Protocol version negotiation and capability declaration
- Clean JSON-RPC 2.0 format with no embedded newlines

#### **Option 2: Project-Specific Configuration**

Create `.windsurf/tools.json` in your project:
```json
{
  "version": "1.0",
  "tools": [
    {
      "name": "simple-container-assistant",
      "type": "mcp",
      "endpoint": "http://localhost:9999/mcp",
      "description": "Simple Container AI assistant with documentation search and project analysis",
      "capabilities": [
        "search_documentation",
        "get_project_context",
        "get_supported_resources",
        "analyze_project"
      ],
      "autoStart": true,
      "icon": "🚀"
    }
  ],
  "settings": {
    "simple-container": {
      "mode": "developer",
      "autoAnalyze": true,
      "searchLimit": 10
    }
  }
}
```

### **Cursor IDE Integration**

Add to `.cursor/config.json`:
```json
{
  "mcp_servers": {
    "simple-container": {
      "url": "http://localhost:9999/mcp",
      "name": "Simple Container Assistant",
      "description": "Documentation search and project context for Simple Container",
      "methods": [
        "search_documentation",
        "get_project_context",
        "get_supported_resources"
      ],
      "autoConnect": true
    }
  },
  "ai": {
    "providers": {
      "simple-container": {
        "endpoint": "http://localhost:9999/mcp",
        "context": "simple-container-project"
      }
    }
  }
}
```

### **VS Code Extension**

Install Simple Container extension and configure:
```json
// settings.json
{
  "simple-container.assistant.enabled": true,
  "simple-container.assistant.mcpEndpoint": "http://localhost:9999/mcp",
  "simple-container.assistant.mode": "developer",
  "simple-container.assistant.searchOnType": true,
  "simple-container.assistant.contextAware": true
}
```

## 🔧 Troubleshooting

### **Common Issues**

#### **"Failed to initialize server" in Windsurf**
- **Cause**: MCP server not responding to initialization sequence
- **Solution**: Ensure `sc` command is in PATH and server starts correctly
- **Test**: Run `sc assistant mcp --stdio` manually and test with initialization sequence

#### **"Server not found" errors**
- **Cause**: Incorrect server name or configuration format
- **Solution**: Ensure server name in configuration matches exactly
- **Test**: Verify `.windsurf/tools.json` syntax with JSON validator

#### **Tool calls not working**
- **Cause**: Server not properly initialized before tool calls
- **Solution**: Ensure proper MCP initialization sequence (initialize → initialized → tool calls)
- **Test**: Use manual stdio testing commands above

#### **Signal handling issues**
- **Cause**: Server not responding to SIGTERM
- **Solution**: Updated implementation now handles signals properly in stdio mode
- **Test**: Start server and test with `timeout 2s sc assistant mcp --stdio`

### **Debug Commands**

```bash
# Check if sc command is available
which sc

# Test server startup
sc assistant mcp --stdio &
sleep 1
pkill -f "sc assistant mcp"

# Verify MCP protocol compliance
printf '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"debug","version":"1.0.0"}},"id":1}\n' | sc assistant mcp --stdio
```

## 📡 MCP API Reference

### **Available Methods**

#### **Standard MCP Methods (2024-11-05)**
| Method            | Purpose                    | Status |
|-------------------|----------------------------|--------|
| `initialize`      | Protocol initialization    | ✅      |
| `ping`            | Connectivity test          | ✅      |
| `tools/list`      | List available tools       | ✅      |
| `tools/call`      | Execute tools              | ✅      |
| `resources/list`  | List available resources   | ✅      |
| `resources/read`  | Read resource content      | ✅      |

#### **Available Tools (via `tools/call`)**
| Tool Name                 | Purpose                             | Status |
|---------------------------|-------------------------------------|--------|
| `search_documentation`    | Semantic doc search                 | ✅      |
| `get_project_context`     | Basic project info & SC config      | ✅      |
| `analyze_project`         | Detailed analysis & recommendations | ✅      |
| `get_supported_resources` | Resource catalog                    | ✅      |

**Note**: Legacy direct method calls have been removed. All functionality is now accessed through standard MCP `tools/call` method for better compliance and cleaner architecture.

**Key Differences:**
- **`get_project_context`**: Returns basic project info and Simple Container configuration status
- **`analyze_project`**: Returns detailed tech stack analysis, recommendations, and architectural insights

### **1. Search Documentation**

Search Simple Container documentation using semantic similarity:

```bash
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "search_documentation",
    "params": {
      "query": "PostgreSQL database configuration with Simple Container",
      "limit": 5,
      "type": "docs"
    },
    "id": "search-1"
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "documents": [
      {
        "id": "supported-resources.md_chunk_15",
        "content": "PostgreSQL database configuration. Use aws-rds-postgres for AWS or gcp-cloudsql-postgres for Google Cloud...",
        "path": "docs/docs/reference/supported-resources.md",
        "type": "docs",
        "similarity": 0.894,
        "metadata": {
          "file_name": "supported-resources.md",
          "provider": "aws,gcp",
          "resource_type": "postgres"
        }
      },
      {
        "id": "postgres-example.md_chunk_3",
        "content": "Complete PostgreSQL setup example with environment variables and connection pooling...",
        "path": "docs/docs/examples/databases/postgres-example.md",
        "type": "examples",
        "similarity": 0.847,
        "metadata": {
          "file_name": "postgres-example.md",
          "category": "database"
        }
      }
    ],
    "total": 2,
    "query": "PostgreSQL database configuration with Simple Container",
    "timestamp": "2024-10-05T13:19:02Z"
  },
  "id": "search-1"
}
```

### **2. Get Project Context**

Analyze the current project and Simple Container configuration:

```bash
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "get_project_context",
    "params": {
      "path": "."
    },
    "id": "context-1"
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "path": "/home/user/my-awesome-app",
    "name": "my-awesome-app",
    "sc_config_exists": true,
    "sc_config_path": "/home/user/my-awesome-app/.sc",
    "tech_stack": {
      "language": "javascript",
      "framework": "express",
      "runtime": "nodejs",
      "version": "18.x",
      "confidence": 0.95
    },
    "resources": [
      {
        "type": "postgres-db",
        "name": "PostgreSQL Database",
        "provider": "aws",
        "status": "configured"
      },
      {
        "type": "redis-cache",
        "name": "Redis Cache",
        "provider": "aws",
        "status": "configured"
      }
    ],
    "recommendations": [
      "Add health check endpoint",
      "Configure connection pooling",
      "Add request logging middleware"
    ],
    "metadata": {
      "analyzed_at": "2024-10-05T13:19:02Z",
      "mcp_version": "1.0",
      "simple_container_version": "1.5.0"
    }
  },
  "id": "context-1"
}
```

### **3. Get Supported Resources**

Get information about all available Simple Container resources:

```bash
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "get_supported_resources",
    "id": "resources-1"
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "resources": [
      {
        "type": "s3-bucket",
        "name": "S3 Bucket",
        "provider": "aws",
        "description": "Amazon S3 storage bucket for file uploads and static assets",
        "properties": ["name", "allowOnlyHttps", "corsConfig"],
        "schema_url": "/schemas/aws/s3bucket.json"
      },
      {
        "type": "aws-rds-postgres",
        "name": "PostgreSQL RDS",
        "provider": "aws",
        "description": "Amazon RDS PostgreSQL managed database",
        "properties": ["name", "instanceClass", "allocatedStorage", "engineVersion"],
        "schema_url": "/schemas/aws/rds-postgres.json"
      },
      {
        "type": "gcp-gke-autopilot-cluster",
        "name": "GKE Autopilot Cluster",
        "provider": "gcp",
        "description": "Google Kubernetes Engine autopilot cluster",
        "properties": ["name", "location", "gkeMinVersion"],
        "schema_url": "/schemas/gcp/gke-autopilot.json"
      }
    ],
    "providers": [
      {
        "name": "aws",
        "display_name": "Amazon Web Services",
        "resources": ["s3-bucket", "aws-rds-postgres", "aws-rds-mysql", "aws-ecs-fargate"]
      },
      {
        "name": "gcp",
        "display_name": "Google Cloud Platform",
        "resources": ["gcp-bucket", "gcp-gke-autopilot-cluster", "gcp-cloudsql-postgres"]
      }
    ],
    "total": 37
  },
  "id": "resources-1"
}
```

### **4. Analyze Project** (Developer Mode Only)

Perform detailed project analysis with tech stack detection:

```bash
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "analyze_project",
    "params": {
      "path": "."
    },
    "id": "analyze-1"
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "path": "/home/user/my-awesome-app",
    "tech_stacks": [
      {
        "language": "javascript",
        "framework": "express",
        "runtime": "nodejs",
        "version": "18.x",
        "dependencies": [
          {"name": "express", "version": "^4.18.0", "type": "runtime"},
          {"name": "pg", "version": "^8.8.0", "type": "runtime"},
          {"name": "redis", "version": "^4.3.0", "type": "runtime"}
        ],
        "confidence": 0.95,
        "evidence": ["package.json found", "express dependency found"]
      },
      {
        "language": "docker",
        "runtime": "docker",
        "framework": "nodejs",
        "confidence": 0.8,
        "evidence": ["Dockerfile found", "node:18 base image detected"]
      }
    ],
    "primary_stack": {
      "language": "javascript",
      "framework": "express"
    },
    "architecture": "standard-web-app",
    "recommendations": [
      {
        "type": "resource",
        "category": "database",
        "priority": "high",
        "title": "PostgreSQL Database",
        "description": "Add PostgreSQL database resource for data persistence",
        "resource": "aws-rds-postgres",
        "action": "add_resource"
      },
      {
        "type": "resource",
        "category": "cache",
        "priority": "medium",
        "title": "Redis Cache",
        "description": "Add Redis cache for session storage and caching",
        "resource": "redis-cache",
        "action": "add_resource"
      }
    ],
    "confidence": 0.92,
    "metadata": {
      "analyzed_at": "2024-10-05T13:19:02Z"
    }
  },
  "id": "analyze-1"
}
```

### **5. Generate Configuration**

Generate Simple Container configuration files:

```bash
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "generate_configuration",
    "params": {
      "project_path": ".",
      "project_type": "nodejs",
      "config_type": "client_yaml",
      "options": {
        "framework": "express",
        "database": "postgresql",
        "cache": "redis"
      }
    },
    "id": "generate-1"
  }'
```

## 🎛️ Server Configuration

### **Environment Variables**
```bash
# MCP server configuration
export SC_MCP_HOST=localhost
export SC_MCP_PORT=9999
export SC_MCP_CORS_ORIGIN="*"
export SC_MCP_LOG_LEVEL=info

# Documentation search
export SC_EMBEDDING_MODEL=text-embedding-3-small
export SC_SEARCH_LIMIT=10

# Enable/disable features
export SC_MCP_ENABLE_ANALYSIS=true
export SC_MCP_ENABLE_GENERATION=true
```

### **Configuration File**
Create `.sc/mcp-config.yaml`:
```yaml
server:
  host: localhost
  port: 9999
  cors:
    enabled: true
    origins: ["*"]
    methods: ["GET", "POST", "OPTIONS"]

logging:
    level: info
    format: json

features:
  documentation_search: true
  project_analysis: true
  configuration_generation: true

search:
  default_limit: 10
  max_limit: 50
  embedding_model: text-embedding-3-small

security:
  api_key_required: false
  rate_limiting:
    enabled: false
    requests_per_minute: 60
```

## 🔐 Security Configuration

### **API Key Authentication**
```bash
# Enable API key authentication
export SC_MCP_API_KEY=your-secret-api-key

# Client requests must include header:
# Authorization: Bearer your-secret-api-key
```

### **CORS Configuration**
```bash
# Restrict origins for production
export SC_MCP_CORS_ORIGIN="https://windsurf.dev,https://cursor.sh"

# Or configure specific domains
export SC_MCP_CORS_ORIGIN="*.mycompany.com"
```

### **Rate Limiting**
```yaml
# In .sc/mcp-config.yaml
security:
  rate_limiting:
    enabled: true
    requests_per_minute: 120
    burst_limit: 20
    whitelist_ips: ["127.0.0.1", "10.0.0.0/8"]
```

## 📊 Monitoring and Logging

### **Health Monitoring**
```bash
# Health check endpoint
curl http://localhost:9999/health

# Response:
{
  "status": "healthy",
  "timestamp": "2024-10-05T13:19:02Z",
  "version": "1.0",
  "name": "simple-container-mcp",
  "uptime": "2h30m15s",
  "requests_served": 1247,
  "documentation_count": 10543
}
```

### **Metrics Endpoint**
```bash
# Prometheus-compatible metrics
curl http://localhost:9999/metrics

# Output:
# mcp_requests_total{method="search_documentation"} 823
# mcp_requests_total{method="get_project_context"} 156
# mcp_request_duration_seconds{method="search_documentation"} 0.045
# mcp_documentation_search_results_total 4115
```

### **Logging Configuration**
```bash
# Structured JSON logging
export SC_MCP_LOG_FORMAT=json
export SC_MCP_LOG_LEVEL=info

# Log to file
export SC_MCP_LOG_FILE=/var/log/simple-container/mcp.log

# Sample log entry:
{
  "timestamp": "2024-10-05T13:19:02Z",
  "level": "info",
  "method": "search_documentation",
  "query": "postgres configuration",
  "results": 5,
  "duration_ms": 45,
  "client_ip": "127.0.0.1"
}
```

## 🚀 Performance Optimization

### **Caching Configuration**
```yaml
# In .sc/mcp-config.yaml
cache:
  enabled: true
  ttl: 300  # 5 minutes
  max_size: 1000

  # Cache documentation search results
  search_results:
    enabled: true
    ttl: 900  # 15 minutes

  # Cache project context
  project_context:
    enabled: true
    ttl: 60   # 1 minute
```

### **Connection Pooling**
```yaml
server:
  connection_pool:
    max_connections: 100
    idle_timeout: 300
    read_timeout: 30
    write_timeout: 30
```

### **Memory Management**
```bash
# Configure memory limits
export SC_MCP_MAX_MEMORY=512MB
export SC_MCP_GC_INTERVAL=60s

# Monitor memory usage
curl http://localhost:9999/debug/memory
```

## 🔧 Troubleshooting

### **Connection Issues**
```bash
# Check if server is running
curl http://localhost:9999/health

# Check port availability
netstat -tlnp | grep 9999

# Test with different port
sc assistant mcp --port 9998
```

### **Search Issues**
```bash
# Verify embeddings are generated
ls -la pkg/assistant/embeddings/embedded_docs.go

# Regenerate embeddings if missing
welder run generate-embeddings

# Test search directly
sc assistant search "test query"
```

### **IDE Integration Issues**
```bash
# Check MCP server logs
sc assistant mcp --verbose

# Validate JSON-RPC requests
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"ping","id":"test"}' | jq
```

## 📋 Examples

### **Custom Client Implementation**
```python
import requests
import json

class SimpleContainerMCP:
    def __init__(self, endpoint="http://localhost:9999/mcp"):
        self.endpoint = endpoint

    def search_docs(self, query, limit=5):
        payload = {
            "jsonrpc": "2.0",
            "method": "search_documentation",
            "params": {"query": query, "limit": limit},
            "id": f"search-{hash(query)}"
        }

        response = requests.post(self.endpoint, json=payload)
        return response.json()

    def get_context(self, path="."):
        payload = {
            "jsonrpc": "2.0",
            "method": "get_project_context",
            "params": {"path": path},
            "id": "context"
        }

        response = requests.post(self.endpoint, json=payload)
        return response.json()

# Usage
client = SimpleContainerMCP()
docs = client.search_docs("AWS S3 configuration")
context = client.get_context()
```

### **Shell Integration**
```bash
# Create helper functions in .bashrc
sc_search() {
    curl -s -X POST http://localhost:9999/mcp \
        -H "Content-Type: application/json" \
        -d "{\"jsonrpc\":\"2.0\",\"method\":\"search_documentation\",\"params\":{\"query\":\"$1\",\"limit\":3},\"id\":\"shell\"}" \
        | jq -r '.result.documents[].content' \
        | head -200
}

sc_context() {
    curl -s -X POST http://localhost:9999/mcp \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"get_project_context","params":{"path":"."},"id":"shell"}' \
        | jq '.result'
}

# Usage
sc_search "postgres setup"
sc_context
```

## 🔗 Next Steps

1. **[Set up IDE integration →](examples/ide-integration.md)**
2. **[Build custom MCP clients →](examples/custom-clients.md)**
3. **[Monitor MCP server performance →](examples/monitoring.md)**
4. **[Secure MCP in production →](examples/security.md)**

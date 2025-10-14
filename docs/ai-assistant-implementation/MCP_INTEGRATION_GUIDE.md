# Simple Container MCP Integration Guide

This guide explains how to integrate Simple Container's AI assistant with external LLM tools via the Model Context Protocol (MCP).

## 🎯 Overview

The Simple Container MCP server provides a JSON-RPC 2.0 interface that exposes:
- **Semantic Documentation Search** - Find relevant docs, examples, and schemas
- **Project Context** - Analyze Simple Container projects and configurations  
- **Resource Information** - Get details about supported cloud resources
- **Configuration Generation** - Generate project files (Phase 2)
- **Project Analysis** - Detect tech stacks and architectures (Phase 2)

## 🚀 Quick Start

### 1. Start the MCP Server

```bash
# Start MCP server on default port (9999)
sc assistant mcp

# Or specify custom host/port
sc assistant mcp --host 0.0.0.0 --port 8080
```

### 2. Test the Server

```bash
# Health check
curl http://localhost:9999/health

# Get capabilities
curl http://localhost:9999/capabilities

# Test ping via JSON-RPC
curl -X POST http://localhost:9999/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "ping",
    "id": "test-1"
  }'
```

## 📋 Available MCP Methods

### 1. `ping`
Simple connectivity test.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "ping",
  "id": "ping-1"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": "pong",
  "id": "ping-1"
}
```

### 2. `search_documentation`
Search Simple Container documentation using semantic similarity.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "search_documentation",
  "params": {
    "query": "AWS S3 bucket configuration",
    "limit": 5,
    "type": "docs"
  },
  "id": "search-1"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "documents": [
      {
        "id": "supported-resources.md_chunk_2",
        "content": "AWS S3 bucket configuration with Simple Container...",
        "path": "docs/docs/reference/supported-resources.md",
        "type": "docs",
        "similarity": 0.892,
        "metadata": {
          "path": "docs/docs/reference/supported-resources.md",
          "type": "docs",
          "file_name": "supported-resources.md"
        }
      }
    ],
    "total": 1,
    "query": "AWS S3 bucket configuration",
    "timestamp": "2024-10-05T13:10:44Z"
  },
  "id": "search-1"
}
```

### 3. `get_project_context`
Get context information about a Simple Container project.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "get_project_context",
  "params": {
    "path": "/path/to/project"
  },
  "id": "context-1"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "path": "/path/to/project",
    "name": "project",
    "sc_config_exists": true,
    "sc_config_path": "/path/to/project/.sc",
    "resources": [],
    "recommendations": [
      "Consider adding Simple Container configuration",
      "Review documentation for best practices"
    ],
    "metadata": {
      "analyzed_at": "2024-10-05T13:10:44Z",
      "mcp_version": "1.0"
    }
  },
  "id": "context-1"
}
```

### 4. `get_supported_resources`
Get information about all supported Simple Container resources.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "get_supported_resources",
  "id": "resources-1"
}
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
        "description": "Amazon S3 storage bucket"
      },
      {
        "type": "gcp-bucket", 
        "name": "GCS Bucket",
        "provider": "gcp",
        "description": "Google Cloud Storage bucket"
      }
    ],
    "providers": [
      {
        "name": "aws",
        "display_name": "Amazon Web Services",
        "resources": ["s3-bucket", "aws-rds-postgres"]
      }
    ],
    "total": 2
  },
  "id": "resources-1"
}
```

### 5. `get_capabilities`
Get server capabilities and feature status.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "get_capabilities",
  "id": "caps-1"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "name": "simple-container-mcp",
    "version": "1.0",
    "methods": [
      "search_documentation",
      "get_project_context", 
      "get_supported_resources",
      "get_capabilities",
      "ping"
    ],
    "features": {
      "documentation_search": true,
      "project_analysis": false,
      "configuration_generation": false,
      "interactive_chat": false
    },
    "endpoints": {
      "mcp": "/mcp",
      "health": "/health",
      "capabilities": "/capabilities"
    }
  },
  "id": "caps-1"
}
```

## 🔧 Integration Examples

### Windsurf IDE Integration

Create a `.windsurf/tools.json` configuration:

```json
{
  "tools": [
    {
      "name": "simple-container-assistant",
      "type": "mcp",
      "endpoint": "http://localhost:9999/mcp",
      "description": "Simple Container AI assistant with documentation search",
      "methods": [
        "search_documentation",
        "get_project_context",
        "get_supported_resources"
      ]
    }
  ]
}
```

### Cursor IDE Integration

Add to your project's `.cursor/config.json`:

```json
{
  "mcp_servers": {
    "simple-container": {
      "url": "http://localhost:9999/mcp",
      "description": "Simple Container documentation and project context"
    }
  }
}
```

### Custom LLM Integration

Python example using `requests`:

```python
import requests
import json

class SimpleContainerMCP:
    def __init__(self, base_url="http://localhost:9999"):
        self.base_url = base_url
        self.mcp_endpoint = f"{base_url}/mcp"
    
    def search_docs(self, query, limit=5):
        payload = {
            "jsonrpc": "2.0",
            "method": "search_documentation",
            "params": {"query": query, "limit": limit},
            "id": f"search-{hash(query)}"
        }
        
        response = requests.post(self.mcp_endpoint, json=payload)
        return response.json()
    
    def get_project_context(self, path="."):
        payload = {
            "jsonrpc": "2.0", 
            "method": "get_project_context",
            "params": {"path": path},
            "id": "context-1"
        }
        
        response = requests.post(self.mcp_endpoint, json=payload)
        return response.json()

# Usage
sc_mcp = SimpleContainerMCP()

# Search for S3 documentation
docs = sc_mcp.search_docs("AWS S3 bucket configuration")
print(f"Found {docs['result']['total']} relevant documents")

# Get current project context
context = sc_mcp.get_project_context()
print(f"Project: {context['result']['name']}")
```

## 🛡️ Security Considerations

### 1. Network Security
- **Default**: Server binds to `localhost:9999` (local access only)
- **Production**: Use firewall rules to restrict access
- **Authentication**: Currently no authentication (suitable for local development)

### 2. CORS Configuration
- **Default**: CORS enabled for all origins (`*`)
- **Production**: Configure specific origins in production environments
- **Headers**: Standard CORS headers for web integration

### 3. Input Validation
- **JSON-RPC**: Validates against JSON-RPC 2.0 specification
- **Parameters**: Type checking and validation for all method parameters
- **Path Access**: File system access limited to specified project paths

## 📊 Performance Characteristics

### Benchmarks (Local Testing)
- **Ping Request**: ~1ms response time
- **Documentation Search**: ~50-100ms (depends on corpus size)
- **Project Context**: ~10-20ms (file system access)
- **Concurrent Requests**: Supports 100+ concurrent connections

### Resource Usage
- **Memory**: ~10-50MB (depending on embedded documentation size)
- **CPU**: Minimal when idle, scales with request volume
- **Disk**: No additional disk I/O (embeddings in memory)

## 🔍 Debugging and Troubleshooting

### Enable Verbose Logging
```bash
sc assistant mcp --verbose
```

### Common Issues

**1. "Documentation database not found"**
```bash
# Generate embeddings first
welder run generate-embeddings

# Or directly
go run cmd/embed-docs/main.go --docs-path ./docs --output ./pkg/assistant/embeddings/embedded_docs.go
```

**2. "Port already in use"**
```bash
# Use different port
sc assistant mcp --port 9998

# Or find and kill existing process
lsof -ti:9999 | xargs kill
```

**3. "Method not found" errors**
Check the `/capabilities` endpoint to see available methods:
```bash
curl http://localhost:9999/capabilities | jq '.methods'
```

### Testing the Integration
```bash
# Run MCP server tests
go test ./pkg/assistant/mcp/... -v

# Test specific functionality
go test ./pkg/assistant/mcp/... -run TestMCPServer -v
```

## 🚀 Future Enhancements (Phases 2-4)

### Phase 2: Enhanced Capabilities
- **Project Analysis**: Detect tech stacks, languages, frameworks
- **Configuration Generation**: Generate Dockerfiles, docker-compose.yaml, .sc structures
- **Advanced Context**: Repository analysis, dependency detection

### Phase 3: Interactive Features  
- **Chat Interface**: Conversational project setup
- **LLM Integration**: Local and cloud LLM support via langchaingo
- **Session Management**: Persistent conversation context

### Phase 4: Enterprise Features
- **Authentication**: API keys, OAuth integration
- **Rate Limiting**: Request throttling and quotas
- **Monitoring**: Metrics, logging, health monitoring
- **Multi-tenancy**: Support for multiple projects/users

## 📚 Additional Resources

- [Model Context Protocol Specification](https://mcp.dev)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [Simple Container Documentation](https://docs.simple-container.com)
- [AI Assistant Implementation Plan](./AI_ASSISTANT_IMPLEMENTATION_PLAN.md)

This MCP integration enables powerful AI-assisted development workflows while maintaining Simple Container's core principles of simplicity and zero external dependencies.

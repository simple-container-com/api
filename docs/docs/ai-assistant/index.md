# AI Assistant

Simple Container's AI-powered project onboarding assistant provides intelligent setup and configuration guidance with two distinct modes aligned with our separation of concerns philosophy.

## 🎯 Two-Mode Architecture

### **Developer Mode** 🧑‍💻
**For Application Teams**
- **Purpose**: Set up application-specific configurations
- **Generates**: `client.yaml`, `docker-compose.yaml`, `Dockerfile`
- **Analysis**: Automatic project tech stack detection
- **Focus**: Application deployment, scaling, dependencies

### **DevOps Mode** 🛠️
**For Infrastructure Teams**  
- **Purpose**: Set up shared infrastructure and resources
- **Generates**: `server.yaml`, `secrets.yaml`, provisioner config
- **Analysis**: Cloud provider selection and resource planning
- **Focus**: Shared resources, templates, infrastructure management

## 🚀 Quick Start

### For Developers
```bash
# Analyze current project and generate application configs
sc assistant dev setup

# Interactive mode with guided setup
sc assistant dev setup --interactive

# Search documentation
sc assistant search "docker compose with postgres"
```

### For DevOps Teams
```bash
# Set up infrastructure for a new project
sc assistant devops setup

# Configure cloud provider and shared resources
sc assistant devops setup --cloud aws --interactive

# Start MCP server for external tools
sc assistant mcp --port 9999
```

## 📋 Available Commands

| Command | Mode | Description |
|---------|------|-------------|
| `sc assistant dev setup` | Developer | Generate client.yaml and compose files |
| `sc assistant dev analyze` | Developer | Analyze project tech stack |
| `sc assistant devops setup` | DevOps | Configure server.yaml and secrets |
| `sc assistant devops resources` | DevOps | Manage shared resources |
| `sc assistant search` | Both | Semantic documentation search |
| `sc assistant chat` | Both | Interactive assistant (Phase 3) |
| `sc assistant mcp` | Both | Start MCP server for external tools |

## 🎭 Mode Comparison

| Aspect | Developer Mode | DevOps Mode |
|--------|----------------|-------------|
| **Target Users** | Application developers | Infrastructure teams |
| **Project Analysis** | ✅ Full tech stack detection | ❌ Not needed |
| **Cloud Selection** | ⚪ Uses existing infrastructure | ✅ Primary decision point |
| **File Generation** | `client.yaml`, `docker-compose.yaml` | `server.yaml`, `secrets.yaml` |
| **Resource Focus** | Application dependencies | Shared infrastructure |
| **Complexity** | Higher (project analysis) | Lower (guided selection) |

## 🔄 Typical Workflow

### 1. DevOps Team Setup (First)
```bash
# DevOps sets up shared infrastructure
sc assistant devops setup --cloud aws
# Generates: server.yaml, secrets.yaml
# Creates: Database, storage, networking resources
```

### 2. Developer Team Setup (Second)
```bash  
# Developers set up their applications
sc assistant dev setup
# Generates: client.yaml, docker-compose.yaml  
# References: Parent resources from DevOps team
```

## 🌟 Key Features

### **Intelligent Project Analysis** (Developer Mode)
- **Language Detection**: Node.js, Python, Go, Java, etc.
- **Framework Recognition**: Express, Django, Gin, Spring, etc.
- **Dependency Analysis**: Database, cache, messaging requirements
- **Architecture Patterns**: Microservice, monolith, serverless detection

### **Cloud Provider Integration** (DevOps Mode)
- **AWS**: ECS Fargate, RDS, S3, ElastiCache, Lambda
- **GCP**: GKE Autopilot, Cloud SQL, Cloud Storage, Cloud Run
- **Azure**: Container Apps, PostgreSQL, Blob Storage (Phase 2)
- **Kubernetes**: Native deployments, Helm operators

### **Semantic Documentation Search** (Both Modes)
- **10,000+ Documents**: Indexed docs, examples, schemas
- **Sub-100ms Search**: Fast semantic similarity matching
- **Context Aware**: Results tailored to current project type
- **Multi-Source**: Documentation, examples, JSON schemas

## 📚 Documentation Structure

- **[Getting Started](getting-started.md)** - Quick setup for both modes
- **[Developer Mode](developer-mode.md)** - Application team workflows
- **[DevOps Mode](devops-mode.md)** - Infrastructure team workflows  
- **[MCP Integration](mcp-integration.md)** - External tool integration
- **[Commands Reference](commands.md)** - Complete command documentation
- **[Examples](examples/)** - Real-world usage scenarios
- **[Troubleshooting](troubleshooting.md)** - Common issues and solutions

## 🔧 Integration Examples

### Windsurf IDE Integration
```json
{
  "tools": [{
    "name": "simple-container-assistant",
    "type": "mcp",
    "endpoint": "http://localhost:9999/mcp"
  }]
}
```

### VS Code Integration  
```json
{
  "simple-container.assistant.mode": "developer",
  "simple-container.assistant.autoAnalyze": true
}
```

## 🎯 Success Metrics

- **Setup Time**: From 30+ minutes to under 5 minutes
- **Configuration Accuracy**: 95%+ generated configs work without modification  
- **User Adoption**: Target 80%+ of new users use assistant for initial setup
- **Documentation Discovery**: 90%+ accuracy in finding relevant docs

## 🚀 Roadmap

### **Phase 1** ✅ - Foundation (Complete)
- Documentation embedding system
- MCP server implementation  
- Semantic search capabilities
- CLI command structure

### **Phase 2** 🔄 - Analysis & Generation (In Progress)
- Project analysis engine
- Two-mode architecture implementation
- File generation system
- Cloud provider templates

### **Phase 3** 📋 - Interactive Experience
- Chat interface implementation
- LLM integration (langchaingo)
- Conversational project setup
- Advanced context management

### **Phase 4** 🏁 - Enterprise Features
- Advanced analytics and insights
- Team collaboration features
- Custom template creation
- Enterprise security and compliance

Ready to get started? Choose your path:
- 🧑‍💻 **[Developer Mode Setup →](developer-mode.md)**
- 🛠️ **[DevOps Mode Setup →](devops-mode.md)**

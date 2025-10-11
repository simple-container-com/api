# Troubleshooting

Common issues and solutions for Simple Container AI Assistant, organized by mode and functionality.

## 🔍 Quick Diagnostics

### **Health Check Commands**
```bash
# Check Simple Container installation
sc --version

# Verify AI assistant is available
sc assistant --help

# Test documentation search
sc assistant search "test" --limit 1

# Check MCP server status
curl -f http://localhost:9999/health || echo "MCP server not running"

# Verify embeddings are generated
ls -la pkg/assistant/embeddings/embedded_docs.go
```

### **System Requirements Check**
```bash
# Check Docker installation
docker --version && docker-compose --version

# Check Go installation (for building)
go version

# Check available memory
free -h

# Check disk space
df -h .
```

## 🧑‍💻 Developer Mode Issues

### **❌ Project Analysis Issues**

#### **Problem: No technology stack detected**
```
Error: no technology stacks detected in project
```

**Causes & Solutions:**

1. **Missing configuration files**
   ```bash
   # Check for language-specific files
   ls -la package.json requirements.txt go.mod composer.json Gemfile
   
   # If missing, create minimal configuration
   echo '{"name": "my-app", "version": "1.0.0"}' > package.json  # Node.js
   echo "Flask==2.0.0" > requirements.txt  # Python
   go mod init my-app  # Go
   ```

2. **Unsupported language/framework**
   ```bash
   # Override with manual specification
   sc assistant dev setup --language python --framework django --skip-analysis
   ```

3. **Complex monorepo structure**
   ```bash
   # Analyze specific service directory
   sc assistant dev analyze --path ./services/api
   sc assistant dev setup --path ./services/api
   ```

#### **Problem: Wrong framework detected**
```
Detected: React (confidence: 0.7)
Expected: Express.js API
```

**Solutions:**
```bash
# Override detection
sc assistant dev setup --framework express --skip-analysis

# Check package.json dependencies
cat package.json | jq '.dependencies'

# Verify main entry point
cat package.json | jq '.main,.scripts.start'
```

### **❌ Configuration Generation Issues**

#### **Problem: Generated client.yaml references non-existent resources**
```yaml
# In generated client.yaml
uses: [postgres-db, redis-cache]  # Resources don't exist
```

**Solutions:**

1. **Ensure DevOps has deployed infrastructure**
   ```bash
   # Check if infrastructure stack exists
   ls -la .sc/stacks/ | grep infrastructure
   
   # If not found, DevOps team needs to run:
   sc assistant devops setup
   sc provision -s infrastructure -e staging
   ```

2. **Check resource names match server.yaml**
   ```bash
   # View available resources
   cat .sc/stacks/infrastructure/server.yaml | grep -A 20 "resources:"
   
   # Update client.yaml with correct names
   vim .sc/stacks/my-app/client.yaml
   ```

3. **Use different parent stack**
   ```bash
   # Generate with specific parent
   sc assistant dev setup --parent my-infrastructure
   ```

#### **Problem: Docker Compose fails to start**
```
Error: Service 'postgres' failed to build: no such file or directory
```

**Solutions:**

1. **Missing Docker dependencies**
   ```bash
   # Check Docker is running
   docker info
   
   # Check docker-compose syntax
   docker-compose config
   
   # Pull required images
   docker-compose pull
   ```

2. **Port conflicts**
   ```bash
   # Check what's using ports
   netstat -tlnp | grep -E ':5432|:6379|:3000'
   
   # Kill conflicting processes or change ports in docker-compose.yaml
   ```

3. **Volume permission issues**
   ```bash
   # Fix volume permissions
   sudo chown -R $USER:$USER ./data
   
   # Or use different volume mount approach
   ```

### **❌ Deployment Issues**

#### **Problem: Parent stack not found**
```
Error: stack "infrastructure" not found in environment "staging"
```

**Solutions:**

1. **Verify infrastructure is deployed**
   ```bash
   # Check available stacks
   ls -la .sc/stacks/
   
   # If infrastructure missing, coordinate with DevOps team
   ```

2. **Check environment name spelling**
   ```bash
   # Check client.yaml for environment references
   grep -r "parentEnv:" .sc/stacks/*/client.yaml
   
   # Update client.yaml with correct environment
   ```

3. **Deploy infrastructure first**
   ```bash
   # DevOps team should run:
   sc provision -s infrastructure -e staging
   ```

## 🛠️ DevOps Mode Issues

### **❌ Infrastructure Setup Issues**

#### **Problem: Cloud credentials not working**
```
Error: unable to configure credentials for AWS
```

**Solutions:**

1. **Re-add credentials with correct permissions**
   ```bash
   # Remove old credentials
   sc secrets remove aws-access-key aws-secret-key
   
   # Add new credentials
   sc secrets add aws-access-key
   sc secrets add aws-secret-key
   
   # Verify credentials work
   aws sts get-caller-identity  # Test AWS CLI
   ```

2. **Check IAM permissions**
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": [
           "s3:*",
           "rds:*", 
           "ecs:*",
           "ec2:*",
           "iam:PassRole"
         ],
         "Resource": "*"
       }
     ]
   }
   ```

3. **Use different credential method**
   ```bash
   # Use AWS CLI profile instead
   export AWS_PROFILE=simple-container
   
   # Or use IAM roles for EC2/ECS
   ```

#### **Problem: Resource naming conflicts**
```
Error: S3 bucket "myapp-staging-uploads" already exists
```

**Solutions:**

1. **Use unique prefixes**
   ```bash
   # Set up with company prefix
   sc assistant devops setup --prefix mycompany-$(date +%Y%m%d)
   ```

2. **Choose different region**
   ```bash
   # Set up in different region
   sc assistant devops setup --region us-west-2
   ```

3. **Clean up existing resources**
   ```bash
   # Remove conflicting resources (be careful!)
   sc destroy -s infrastructure -e staging --target s3-bucket
   ```

### **❌ Multi-Environment Issues**

#### **Problem: Environment isolation problems**
```
Production database accidentally connected to staging app
```

**Solutions:**

1. **Use strict naming conventions**
   ```yaml
   # In server.yaml, always use environment suffixes
   resources:
     staging:
       postgres-db:
         name: myapp-staging-db  # Always include environment
     production:
       postgres-db:
         name: myapp-production-db  # Different name
   ```

2. **Separate AWS accounts/GCP projects**
   ```bash
   # Use different cloud accounts per environment
   sc assistant devops secrets --auth aws-staging
   sc assistant devops secrets --auth aws-production
   ```

3. **Add environment validation**
   ```yaml
   # In client.yaml, validate environment matches
   stacks:
     my-app:
       parent: infrastructure
       parentEnv: staging  # Must match deployment environment
   ```

## 🔍 Search & Documentation Issues

### **❌ Documentation Search Issues**

#### **Problem: Search returns no results**
```
🔍 Searching documentation for: "database setup"
Found 0 relevant documents
```

**Solutions:**

1. **Regenerate embeddings**
   ```bash
   # Check if embeddings exist
   ls -la pkg/assistant/embeddings/embedded_docs.go
   
   # Regenerate embeddings
   welder run generate-embeddings
   
   # Or manually
   go run cmd/embed-docs/main.go --docs-path ./docs --output ./pkg/assistant/embeddings/embedded_docs.go
   ```

2. **Check search query**
   ```bash
   # Try broader search terms
   sc assistant search "database"
   sc assistant search "postgres"
   sc assistant search "configuration"
   
   # Try different document types
   sc assistant search "database" --type examples
   ```

3. **Lower similarity threshold**
   ```bash
   # Allow less precise matches
   sc assistant search "database setup" --threshold 0.5
   ```

#### **Problem: Search results not relevant**
```
Search: "Node.js deployment"
Results: Python Django examples
```

**Solutions:**

1. **Use more specific queries**
   ```bash
   # Be more specific
   sc assistant search "Node.js Express deployment ECS"
   
   # Use exact framework names
   sc assistant search "Express.js container"
   ```

2. **Filter by provider or type**
   ```bash
   # Filter by cloud provider
   sc assistant search "Node.js" --provider aws
   
   # Filter by document type
   sc assistant search "Node.js" --type examples
   ```

## 🌐 MCP Server Issues

### **❌ Server Connection Issues**

#### **Problem: MCP server won't start**
```
Error: listen tcp :9999: bind: address already in use
```

**Solutions:**

1. **Check port availability**
   ```bash
   # Find what's using the port
   lsof -i :9999
   
   # Use different port
   sc assistant mcp --port 9998
   
   # Or kill the process using the port
   kill $(lsof -t -i :9999)
   ```

2. **Check firewall/network**
   ```bash
   # Test local connection
   telnet localhost 9999
   
   # Check firewall rules
   sudo ufw status  # Ubuntu
   firewall-cmd --list-ports  # RHEL/CentOS
   ```

#### **Problem: IDE can't connect to MCP server**
```
Failed to connect to Simple Container MCP server at localhost:9999
```

**Solutions:**

1. **Verify server is running**
   ```bash
   # Check server health
   curl http://localhost:9999/health
   
   # Check server logs
   sc assistant mcp --verbose
   ```

2. **Check IDE configuration**
   ```json
   // Verify .windsurf/tools.json
   {
     "tools": [{
       "name": "simple-container-assistant",
       "type": "mcp", 
       "endpoint": "http://localhost:9999/mcp"  // Correct endpoint
     }]
   }
   ```

3. **Test with curl**
   ```bash
   # Test JSON-RPC endpoint
   curl -X POST http://localhost:9999/mcp \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"ping","id":"test"}'
   ```

### **❌ Authentication Issues**

#### **Problem: API key authentication failing**
```
Error: unauthorized - invalid API key
```

**Solutions:**

1. **Check API key configuration**
   ```bash
   # Verify environment variable is set
   echo $SC_MCP_API_KEY
   
   # Restart server with API key
   export SC_MCP_API_KEY=your-secret-key
   sc assistant mcp
   ```

2. **Update client configuration**
   ```json
   // Add API key to IDE config
   {
     "tools": [{
       "endpoint": "http://localhost:9999/mcp",
       "headers": {
         "Authorization": "Bearer your-secret-key"
       }
     }]
   }
   ```

## 🚀 Performance Issues

### **❌ Slow Analysis/Generation**

#### **Problem: Project analysis takes too long**
```
Analyzing project... (taking over 60 seconds)
```

**Solutions:**

1. **Exclude large directories**
   ```bash
   # Add .scignore file
   cat > .scignore << EOF
   node_modules/
   .git/
   dist/
   build/
   __pycache__/
   .venv/
   EOF
   ```

2. **Analyze specific paths**
   ```bash
   # Analyze only application code
   sc assistant dev analyze --path ./src
   ```

3. **Increase system resources**
   ```bash
   # Check available memory
   free -h
   
   # Close other applications
   # Consider running on machine with more RAM
   ```

### **❌ Slow Documentation Search**

#### **Problem: Search takes too long**
```
Search taking 5+ seconds per query
```

**Solutions:**

1. **Reduce search scope**
   ```bash
   # Search fewer results
   sc assistant search "query" --limit 3
   
   # Search specific document types
   sc assistant search "query" --type docs
   ```

2. **Check system resources**
   ```bash
   # Monitor CPU/memory during search
   top
   
   # Check disk I/O
   iostat -x 1
   ```

3. **Optimize embeddings**
   ```bash
   # Regenerate embeddings with smaller chunks
   # Edit cmd/embed-docs/main.go to reduce chunk size
   # Then regenerate
   welder run generate-embeddings
   ```

## 🔧 Build & Installation Issues

### **❌ Build Errors**

#### **Problem: Missing embeddings during build**
```
Error: embedded_docs.go: no such file or directory
```

**Solutions:**

1. **Run embedding generation**
   ```bash
   # Generate embeddings first
   welder run generate-embeddings
   
   # Then build
   welder run build
   ```

2. **Check build order in welder.yaml**
   ```yaml
   # Ensure correct task order
   steps:
     - task: generate-schemas
     - task: generate-embeddings  # Must come before build
     - task: build-all
   ```

#### **Problem: Dependency conflicts**
```
Error: package github.com/philippgille/chromem-go: version conflict
```

**Solutions:**

1. **Update dependencies**
   ```bash
   # Clean and update modules
   go clean -modcache
   go mod tidy
   go mod download
   ```

2. **Check Go version**
   ```bash
   # Ensure Go 1.21+ 
   go version
   
   # Update if necessary
   ```

## 🆘 Getting Help

### **Diagnostic Information**

When reporting issues, include:

```bash
# System information
sc --version
go version
docker --version
uname -a

# Configuration
cat .sc/cfg.default.yaml
ls -la .sc/stacks/

# Logs
sc assistant mcp --verbose 2>&1 | head -50

# Error details
sc assistant dev analyze --verbose 2>&1
```

### **Support Channels**

1. **Documentation Search**
   ```bash
   # Search for your specific issue
   sc assistant search "your error message"
   ```

2. **Community Support**
   - GitHub Issues: [simple-container-com/api/issues](https://github.com/simple-container-com/api/issues)
   - Stack Overflow: Tag `simple-container`
   - Community Slack: [slack.simple-container.com](https://slack.simple-container.com)

3. **Professional Support**
   - Enterprise Support: support@simple-container.com
   - Consulting Services: consulting@simple-container.com

### **Creating Bug Reports**

Use this template for bug reports:

```markdown
## Bug Report

**Version:** sc --version output
**OS:** Operating system and version
**Mode:** Developer/DevOps

**Expected Behavior:**
What should happen

**Actual Behavior:**
What actually happens

**Reproduction Steps:**
1. Run command X
2. See error Y

**Configuration:**
- Cloud provider: AWS/GCP/K8s
- Environment: staging/production
- Project type: Node.js/Python/Go

**Logs:**
```
Paste relevant log output here
```

**Additional Context:**
Any other relevant information
```

## 🔗 Related Documentation

- **[Getting Started](getting-started.md)** - Basic setup and usage
- **[Commands Reference](commands.md)** - Complete command documentation  
- **[Developer Mode](developer-mode.md)** - Application team workflows
- **[DevOps Mode](devops-mode.md)** - Infrastructure team workflows
- **[MCP Integration](mcp-integration.md)** - External tool integration

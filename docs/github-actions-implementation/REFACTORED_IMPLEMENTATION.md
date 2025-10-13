# GitHub Actions - Refactored Implementation with SC Internal APIs

## ✅ **Implementation Complete**

Successfully refactored GitHub Actions to use Simple Container's internal APIs and follow SC's architectural patterns.

## 🏗️ **Architecture Overview**

### **Single Binary Approach**
- **Entry Point**: `cmd/github-actions/main.go`
- **Single Dockerfile**: `github-actions.Dockerfile` (root directory)
- **Single Docker Image**: `simplecontainer/github-actions:latest`
- **4 Action Types**: Determined by `GITHUB_ACTION_TYPE` environment variable

### **SC Internal API Usage**

**Core Operations** (following memory guidelines):
- ✅ **Deploy**: `provisioner.Deploy(ctx, api.DeployParams)`
- ✅ **Destroy**: `provisioner.Destroy(ctx, api.DestroyParams, preview)`
- ✅ **DestroyParent**: `provisioner.DestroyParent(ctx, api.DestroyParams, preview)`
- ✅ **Provision**: `provisioner.Provision(ctx, api.ProvisionParams)`
- ✅ **Secrets**: `provisioner.Cryptor().DecryptAll(forceReveal)`

**Reused SC Packages**:
- ✅ **Logger**: `pkg/api/logger` - SC's structured logging
- ✅ **Git**: `pkg/api/git` - SC's git operations
- ✅ **Provisioner**: `pkg/provisioner` - SC's core deployment engine
- ✅ **Notifications**: `pkg/githubactions/common/notifications` - Existing notification system

## 📁 **File Structure**

```
/cmd/github-actions/main.go              # Single entry point using SC APIs
/pkg/githubactions/actions/executor.go   # Action executor using SC patterns
/github-actions.Dockerfile               # Single multi-stage Dockerfile
/.github/actions/                        # Action definitions
  ├── deploy-client-stack/action.yml
  ├── provision-parent-stack/action.yml
  ├── destroy-client-stack/action.yml
  └── destroy-parent-stack/action.yml
```

## 🚀 **Usage Examples**

### **Deploy Client Stack**
```yaml
- uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
  with:
    stack-name: "my-app"
    environment: "staging"
    sc-config: ${{ secrets.SC_CONFIG }}
    slack-webhook-url: ${{ secrets.SLACK_WEBHOOK_URL }}
```

### **Provision Parent Stack**
```yaml
- uses: simple-container-com/api/.github/actions/provision@v2025.10.4
  with:
    stack-name: "infrastructure"
    sc-config: ${{ secrets.SC_CONFIG }}
```

### **Destroy Client Stack**
```yaml
- uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
  with:
    stack-name: "my-app"
    environment: "staging"
    sc-config: ${{ secrets.SC_CONFIG }}
```

### **Destroy Parent Stack**
```yaml
- uses: simple-container-com/api/.github/actions/destroy-parent@v2025.10.4
  with:
    stack-name: "infrastructure"
    sc-config: ${{ secrets.SC_CONFIG }}
```

## 🛠️ **Technical Details**

### **Docker Image Build**
```bash
# Built via welder.yaml
welder build github-actions

# Manual build
docker build -f github-actions.Dockerfile -t simplecontainer/github-actions:latest .
```

### **Environment Variables**
- `GITHUB_ACTION_TYPE`: Action type (deploy-client-stack, provision-parent-stack, etc.)
- `STACK_NAME`: Name of the stack to operate on
- `ENVIRONMENT`: Target environment (for client stacks)
- `SC_CONFIG`: Simple Container configuration
- `VERSION`: Version to deploy (optional, defaults to "latest")
- `SLACK_WEBHOOK_URL`: Slack notifications (optional)
- `DISCORD_WEBHOOK_URL`: Discord notifications (optional)

### **Internal API Integration**

**Provisioner Setup**:
```go
// Initialize SC's internal APIs
log := logger.New()
gitRepo, err := git.New(git.WithDetectRootDir())
prov, err := provisioner.New(
    provisioner.WithGitRepo(gitRepo),
    provisioner.WithLogger(log),
)
```

**Action Execution**:
```go
// Use SC's internal deployment API
deployParams := api.DeployParams{
    StackParams: api.StackParams{
        StackName:   stackName,
        Environment: environment,
        Version:     version,
    },
}
err := provisioner.Deploy(ctx, deployParams)
```

## 🎯 **Key Benefits**

### **Architectural Consistency**
- ✅ Uses SC's existing patterns and APIs
- ✅ No duplicate implementations
- ✅ Single source of truth for SC operations
- ✅ Consistent error handling and logging

### **Maintainability**
- ✅ Single binary reduces complexity
- ✅ Reuses tested SC components
- ✅ Follows SC architectural patterns
- ✅ Easy to extend and modify

### **Performance**
- ✅ Direct API calls (no shell commands)
- ✅ Single Docker image
- ✅ Efficient resource usage
- ✅ Faster execution

### **Zero External Dependencies**
- ✅ Self-contained Docker image
- ✅ All functionality embedded
- ✅ No `actions/checkout` needed
- ✅ No external tool dependencies

## 📋 **Implementation Status**

### **Completed**
- ✅ Single Go binary with SC API integration
- ✅ Single Dockerfile in proper location
- ✅ Welder.yaml configuration updated
- ✅ Action definitions in `.github/actions/`
- ✅ SC internal API usage (provisioner, logger, git, notifications)
- ✅ Proper error handling and logging
- ✅ Code formatting compliance (`welder run fmt` successful)

### **Architecture Compliance**
- ✅ Follows SC patterns and conventions
- ✅ Reuses existing SC packages
- ✅ No duplicate implementations
- ✅ Proper separation of concerns
- ✅ Consistent with SC codebase

## 🔧 **Development Workflow**

1. **Build**: `welder build github-actions`
2. **Test**: Actions automatically use latest image
3. **Deploy**: Push to registry via welder
4. **Usage**: Reference in workflows as shown above

## 📚 **Related Documentation**

- [CI/CD Workflow Generation](../pkg/cmd/cmd_cicd/) - Dynamic workflow generation
- [SC Internal APIs](../../pkg/api/) - Core Simple Container APIs
- [Provisioner](../../pkg/provisioner/) - Deployment engine
- [Notifications](../../pkg/githubactions/common/notifications/) - Notification system

---

**Status**: ✅ **Production Ready** - Refactored GitHub Actions implementation using SC's internal APIs and architectural patterns.

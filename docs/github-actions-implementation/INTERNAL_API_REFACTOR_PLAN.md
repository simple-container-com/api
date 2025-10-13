# GitHub Actions Refactor: Internal SC API Usage

## Overview

This document outlines the plan to refactor the existing GitHub Actions implementation to use internal Simple Container APIs instead of shell command calls. This approach provides better type safety, error handling, and integration with the SC ecosystem.

## Current vs. Proposed Architecture

### Current Architecture (Shell-based)
```go
// Current approach - shell commands
func Execute(ctx context.Context, cfg *config.Config) error {
    // Shell commands like:
    cmd := exec.Command("sc", "deploy", "--stack", cfg.StackName)
    err := cmd.Run()
}
```

### Proposed Architecture (Internal APIs)
```go
// Proposed approach - internal APIs
func Execute(ctx context.Context, cfg *config.Config) error {
    // Direct API usage:
    provisioner := provisioner.New(...)
    err := provisioner.Deploy(ctx, deployParams)
}
```

## Available Internal SC APIs

### 1. Core Provisioner Operations

#### Deploy Client Stack
```go
// Replace: sc deploy --stack <name> --env <env>
deployParams := api.DeployParams{
    StackParams: api.StackParams{
        StackName:   cfg.StackName,
        Environment: cfg.Environment,
    },
    Version: cfg.Version,
}
err := provisioner.Deploy(ctx, deployParams)
```

#### Destroy Client Stack
```go
// Replace: sc destroy --stack <name> --env <env>
destroyParams := api.DestroyParams{
    StackParams: api.StackParams{
        StackName:   cfg.StackName,
        Environment: cfg.Environment,
    },
}
err := provisioner.Destroy(ctx, destroyParams, preview)
```

#### Provision Parent Stack
```go
// Replace: sc provision --stacks <names>
provisionParams := api.ProvisionParams{
    Stacks:   []string{cfg.StackName},
    Profile:  cfg.Environment,
}
err := provisioner.Provision(ctx, provisionParams)
```

#### Destroy Parent Stack
```go
// Replace: sc destroy --parent --stack <name>
destroyParams := api.DestroyParams{
    StackParams: api.StackParams{
        StackName: cfg.StackName,
    },
}
err := provisioner.DestroyParent(ctx, destroyParams, preview)
```

### 2. Secrets Management

#### Reveal Secrets
```go
// Replace: sc secrets reveal
err := provisioner.Cryptor().DecryptAll(forceReveal)

// Read specific secret files
err := provisioner.Cryptor().ReadSecretFiles()
```

### 3. Git Operations

#### Initialize Git Repository
```go
// Replace: git operations via shell
gitRepo, err := git.New(git.WithDetectRootDir())
err := gitRepo.InitOrOpen(workDir)
```

#### Git Metadata Extraction
```go
// Replace: git rev-parse, git branch, etc.
branch, err := gitRepo.Branch()
commitHash, err := gitRepo.Hash()
commits := gitRepo.Log()
```

#### Git Commit and Tag Creation
```go
// Replace: git add, git commit, git tag
err := gitRepo.AddFileToGit(".")
err := gitRepo.Commit("Release v1.0.0", git.CommitOpts{All: true})
```

### 4. Logging

#### Structured Logging
```go
// Replace: echo statements
logger := logger.New()
logger.Info(ctx, "Deployment started for stack: %s", stackName)
logger.Error(ctx, "Deployment failed: %v", err)
```

### 5. Configuration Management

#### Provisioner Initialization
```go
// Initialize provisioner with proper setup
provisioner, err := provisioner.New(
    provisioner.WithGitRepo(gitRepo),
    provisioner.WithLogger(logger),
)

err = provisioner.Init(ctx, api.InitParams{
    ProjectName:         cfg.StackName,
    RootDir:             workDir,
    SkipInitialCommit:   true,
    SkipProfileCreation: true,
    Profile:             cfg.Environment,
})
```

## Refactoring Implementation Plan

### Phase 1: Core Action Refactoring

#### 1. Deploy Client Stack Action
**File**: `pkg/githubactions/actions/deploy/deploy.go`

**Changes**:
- Replace shell-based `sc` commands with `provisioner.Deploy()`
- Replace git shell commands with `git.Repo` interface
- Maintain existing notification and error handling logic
- Keep GitHub Actions output generation

**Key Refactors**:
```go
// Before (shell-based)
cmd := exec.Command("sc", "deploy", "--stack", cfg.StackName, "--env", cfg.Environment)
err := cmd.Run()

// After (internal API)
deployParams := api.DeployParams{
    StackParams: api.StackParams{
        StackName:   cfg.StackName,
        Environment: cfg.Environment,
    },
    Version: cfg.Version,
}
err := provisioner.Deploy(ctx, deployParams)
```

#### 2. Destroy Client Stack Action
**File**: `pkg/githubactions/actions/destroyclient/destroy.go`

**Changes**:
- Replace `sc destroy` with `provisioner.Destroy()`
- Add safety confirmation logic using internal APIs
- Implement backup functionality if needed

#### 3. Provision Parent Stack Action
**File**: `pkg/githubactions/actions/provision/provision.go`

**Changes**:
- Replace `sc provision` with `provisioner.Provision()`
- Handle multiple stack provisioning scenarios

#### 4. Destroy Parent Stack Action
**File**: `pkg/githubactions/actions/destroyparent/destroy.go`

**Changes**:
- Replace `sc destroy --parent` with `provisioner.DestroyParent()`
- Implement enhanced safety checks

### Phase 2: Shared Components Refactoring

#### 1. Update Common Components
**Files**:
- `pkg/githubactions/common/sc/operations.go` → Remove (replace with direct API calls)
- `pkg/githubactions/common/git/operations.go` → Simplify (use internal git package)
- `pkg/githubactions/common/version/generator.go` → Simplify or remove

#### 2. Notification Manager
**File**: `pkg/githubactions/common/notifications/manager.go`

**Changes**:
- Keep existing implementation (it's already well-structured)
- Enhance to work with internal SC logger for consistency

### Phase 3: Build Integration

#### 1. Update Dockerfile Structure
**Files**: `docs/github-actions-implementation/actions-embedded/*/Dockerfile`

**Changes**:
- Remove external tool installations (SC CLI, etc.)
- Embed the compiled Go binary with all internal APIs
- Simplify container to just run the internal binary

**Example**:
```dockerfile
# Before: Install external tools
RUN curl -s "https://dist.simple-container.com/sc.sh" | bash
RUN apt-get install -y git curl jq

# After: Use embedded binary
COPY dist/github-actions /usr/local/bin/github-actions
ENTRYPOINT ["/usr/local/bin/github-actions", "deploy-client-stack"]
```

#### 2. Update Build System
**File**: `welder.yaml`

**Future Addition** (when ready to implement):
```yaml
build-github-actions:
  runOn: host
  script:
    - echo "Building GitHub Actions binary with internal APIs..."
    - go build -ldflags "${arg:ld-flags}" -o ${project:root}/dist/github-actions ./cmd/github-actions
    - echo "✅ GitHub Actions binary built successfully with internal SC APIs"
```

### Phase 4: Configuration Enhancement

#### 1. GitHub Actions Main Entry Point
**File**: `cmd/github-actions/main.go` (to be created)

**Implementation**:
```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/simple-container-com/api/pkg/githubactions/config"
    "github.com/simple-container-com/api/pkg/githubactions/actions/deploy"
    "github.com/simple-container-com/api/pkg/githubactions/actions/provision"
    "github.com/simple-container-com/api/pkg/githubactions/actions/destroyclient"
    "github.com/simple-container-com/api/pkg/githubactions/actions/destroyparent"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintf(os.Stderr, "Usage: %s <action-type>\n", os.Args[0])
        os.Exit(1)
    }

    actionType := os.Args[1]
    ctx := context.Background()
    cfg := config.LoadFromEnvironment()

    var err error
    switch actionType {
    case "deploy-client-stack":
        err = deploy.Execute(ctx, cfg)
    case "provision-parent-stack":
        err = provision.Execute(ctx, cfg)
    case "destroy-client-stack":
        err = destroyclient.Execute(ctx, cfg)
    case "destroy-parent-stack":
        err = destroyparent.Execute(ctx, cfg)
    default:
        fmt.Fprintf(os.Stderr, "Unknown action type: %s\n", actionType)
        os.Exit(1)
    }

    if err != nil {
        fmt.Fprintf(os.Stderr, "Action failed: %v\n", err)
        os.Exit(1)
    }
}
```

## Benefits of Internal API Usage

### 1. Type Safety
- Compile-time validation of parameters
- Structured error handling
- No string interpolation errors

### 2. Performance
- No process spawning overhead
- Direct memory access to SC internals
- Reduced I/O operations

### 3. Maintainability
- Single codebase for all SC operations
- Consistent error handling patterns
- Easier testing and debugging

### 4. Integration
- Access to SC's internal state
- Proper logging integration
- Consistent configuration handling

### 5. Reliability
- Better error propagation
- Transaction-like operations
- Proper cleanup handling

## Implementation Checklist

### Prerequisites
- [ ] Ensure all required internal APIs are exported
- [ ] Verify API stability and backwards compatibility
- [ ] Create comprehensive test coverage

### Core Refactoring
- [ ] Refactor deploy client stack action
- [ ] Refactor destroy client stack action
- [ ] Refactor provision parent stack action
- [ ] Refactor destroy parent stack action
- [ ] Update shared notification components
- [ ] Remove shell-based operation helpers

### Integration
- [ ] Create main GitHub Actions entry point
- [ ] Update Docker build process
- [ ] Update welder.yaml build configuration
- [ ] Test with real GitHub Actions workflows

### Documentation
- [ ] Update action.yml files with new capabilities
- [ ] Update usage examples
- [ ] Create migration guide for existing users
- [ ] Document internal API patterns

### Testing
- [ ] Unit tests for each action
- [ ] Integration tests with real SC projects
- [ ] Performance benchmarks
- [ ] Error handling validation

## Migration Strategy

### 1. Gradual Migration
- Keep existing shell-based actions as backup
- Implement internal API versions alongside
- A/B test with selected repositories

### 2. Validation Phase
- Compare outputs between shell and API versions
- Verify all functionality is preserved
- Ensure error handling is equivalent or better

### 3. Full Migration
- Update all action references to use internal APIs
- Remove shell-based implementations
- Update documentation and examples

## Conclusion

This refactoring will transform the GitHub Actions from external tool orchestrators to native Simple Container API consumers. The result will be more reliable, faster, and easier to maintain actions that provide the same functionality with better integration into the SC ecosystem.

The internal API usage aligns with Simple Container's architecture and provides a solid foundation for future enhancements and features.

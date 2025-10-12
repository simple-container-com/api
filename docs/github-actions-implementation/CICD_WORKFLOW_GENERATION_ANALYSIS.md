# CI/CD Workflow Generation Analysis

## Current State Analysis

### ✅ Basic Foundation Exists
Simple Container already has a basic CI/CD configuration structure in `server.yaml`:

```yaml
# Current minimal implementation
cicd:
  type: github-actions
  config:
    auth-token: "${secret:GITHUB_TOKEN}"
```

**Current Limitations:**
- Only supports basic auth token configuration
- No workflow generation capabilities
- No organizational standardization features
- No integration with our new self-contained GitHub Actions

## 🎯 Required Enhancements for Workflow Generation

### 1. **Extended CiCd Configuration Schema**

**Current Structure** (`pkg/clouds/github/github_actions.go`):
```go
type ActionsCiCdConfig struct {
    AuthToken string `json:"auth-token" yaml:"auth-token"`
}
```

**Required Enhanced Structure**:
```go
type ActionsCiCdConfig struct {
    // Basic authentication
    AuthToken string `json:"auth-token" yaml:"auth-token"`
    
    // Organization settings
    Organization OrganizationConfig `json:"organization" yaml:"organization"`
    
    // Workflow generation settings
    WorkflowGeneration WorkflowGenerationConfig `json:"workflow-generation" yaml:"workflow-generation"`
    
    // Environment-specific deployment configurations
    Environments map[string]EnvironmentConfig `json:"environments" yaml:"environments"`
    
    // Notification settings
    Notifications NotificationConfig `json:"notifications" yaml:"notifications"`
    
    // Custom runners and execution settings
    Execution ExecutionConfig `json:"execution" yaml:"execution"`
    
    // Validation and testing
    Validation ValidationConfig `json:"validation" yaml:"validation"`
}

type OrganizationConfig struct {
    Name            string   `json:"name" yaml:"name"`
    DefaultRunners  []string `json:"default-runners" yaml:"default-runners"`
    RequiredSecrets []string `json:"required-secrets" yaml:"required-secrets"`
    BranchProtection bool    `json:"branch-protection" yaml:"branch-protection"`
    Reviewers       []string `json:"reviewers" yaml:"reviewers"`
}

type WorkflowGenerationConfig struct {
    Enabled     bool     `json:"enabled" yaml:"enabled"`
    OutputPath  string   `json:"output-path" yaml:"output-path"` // .github/workflows/
    Templates   []string `json:"templates" yaml:"templates"`    // deploy, destroy, provision
    AutoUpdate  bool     `json:"auto-update" yaml:"auto-update"`
    CustomActions map[string]string `json:"custom-actions" yaml:"custom-actions"`
}

type EnvironmentConfig struct {
    Type          string            `json:"type" yaml:"type"`           // staging, production, preview
    Runners       []string          `json:"runners" yaml:"runners"`
    Protection    bool              `json:"protection" yaml:"protection"`
    Reviewers     []string          `json:"reviewers" yaml:"reviewers"`
    Secrets       []string          `json:"secrets" yaml:"secrets"`
    Variables     map[string]string `json:"variables" yaml:"variables"`
    DeployFlags   []string          `json:"deploy-flags" yaml:"deploy-flags"`
    AutoDeploy    bool              `json:"auto-deploy" yaml:"auto-deploy"`
    ValidationCmd string            `json:"validation-command" yaml:"validation-command"`
}

type NotificationConfig struct {
    SlackWebhook   string              `json:"slack-webhook" yaml:"slack-webhook"`
    DiscordWebhook string              `json:"discord-webhook" yaml:"discord-webhook"`
    UserMappings   map[string]string   `json:"user-mappings" yaml:"user-mappings"`
    CCOnStart      bool                `json:"cc-on-start" yaml:"cc-on-start"`
    Channels       map[string]string   `json:"channels" yaml:"channels"` // env -> channel
}

type ExecutionConfig struct {
    DefaultTimeout  string            `json:"default-timeout" yaml:"default-timeout"`
    Concurrency     ConcurrencyConfig `json:"concurrency" yaml:"concurrency"`
    RetryPolicy     RetryConfig       `json:"retry-policy" yaml:"retry-policy"`
    CustomRunners   map[string]string `json:"custom-runners" yaml:"custom-runners"`
}

type ValidationConfig struct {
    Required      bool              `json:"required" yaml:"required"`
    Commands      map[string]string `json:"commands" yaml:"commands"`     // env -> command
    HealthChecks  map[string]string `json:"health-checks" yaml:"health-checks"`
    TestSuites    []string          `json:"test-suites" yaml:"test-suites"`
}
```

### 2. **Enhanced Server.yaml Configuration Example**

```yaml
schemaVersion: "1.0"

# Enhanced CI/CD configuration for workflow generation
cicd:
  type: github-actions
  config:
    auth-token: "${secret:GITHUB_TOKEN}"
    
    # Organization-wide settings
    organization:
      name: "mycompany"
      default-runners: ["ubuntu-latest"]
      required-secrets: ["SC_CONFIG", "DOCKER_HUB_TOKEN"]
      branch-protection: true
      reviewers: ["devops-team", "tech-leads"]
    
    # Workflow generation settings
    workflow-generation:
      enabled: true
      output-path: ".github/workflows/"
      templates: ["deploy", "destroy", "provision", "pr-preview"]
      auto-update: true
      custom-actions:
        deploy: "simple-container-com/api/.github/actions/deploy-client-stack@v1"
        destroy: "simple-container-com/api/.github/actions/destroy-client-stack@v1"
        provision: "simple-container-com/api/.github/actions/provision-parent-stack@v1"
    
    # Environment-specific configurations
    environments:
      staging:
        type: "staging"
        runners: ["ubuntu-latest"]
        protection: false
        auto-deploy: true
        deploy-flags: ["--skip-preview"]
        validation-command: "curl -f https://staging-api.mycompany.com/health"
        
      production:
        type: "production"
        runners: ["blacksmith-8vcpu-ubuntu-2204"]
        protection: true
        reviewers: ["senior-devs", "devops-team"]
        auto-deploy: false
        deploy-flags: ["--verbose", "--skip-refresh"]
        validation-command: |
          sleep 30
          curl -f https://api.mycompany.com/health
          curl -f https://api.mycompany.com/metrics
        
      preview:
        type: "preview"
        runners: ["ubuntu-latest"]
        protection: false
        auto-deploy: true
        deploy-flags: ["--skip-preview", "--skip-refresh"]
    
    # Notification settings
    notifications:
      slack-webhook: "${secret:SLACK_WEBHOOK_URL}"
      discord-webhook: "${secret:DISCORD_WEBHOOK_URL}"
      cc-on-start: true
      user-mappings:
        "john.doe": "U12345678"
        "jane.smith": "U87654321"
      channels:
        staging: "#deployments-staging"
        production: "#deployments-prod"
    
    # Execution settings
    execution:
      default-timeout: "30m"
      concurrency:
        group: "${{ github.workflow }}-${{ github.ref }}"
        cancel-in-progress: false
      custom-runners:
        high-cpu: "blacksmith-16vcpu-ubuntu-2204"
        gpu-enabled: "blacksmith-gpu-ubuntu-2204"
    
    # Validation settings
    validation:
      required: true
      commands:
        staging: "npm test && npm run e2e:staging"
        production: "npm run test:prod && npm run security:scan"
      health-checks:
        api: "/health"
        metrics: "/metrics"
      test-suites: ["unit", "integration", "e2e"]
```

## 3. **Implementation Requirements**

### **A. Enhanced GitHub Provider** (`pkg/clouds/github/`)

**New Files Needed:**
```
pkg/clouds/github/
├── github_actions.go          # Enhanced ActionsCiCdConfig
├── workflow_generator.go      # Workflow generation logic
├── templates/                 # Workflow templates
│   ├── deploy.yml.tpl
│   ├── destroy.yml.tpl
│   ├── provision.yml.tpl
│   └── pr-preview.yml.tpl
└── validation.go              # Configuration validation
```

### **B. New CLI Command** 

```bash
sc cicd generate --stack myorg/infrastructure --output .github/workflows/
sc cicd validate --stack myorg/infrastructure --config server.yaml
sc cicd sync --stack myorg/infrastructure    # Update existing workflows based on server.yaml changes
```

### **C. Workflow Templates**

**Deploy Workflow Template** (`templates/deploy.yml.tpl`):
```yaml
name: Deploy {{ .StackName }}
on:
  push:
    branches: [{{ .DefaultBranch }}]
  workflow_dispatch:
    inputs:
      environment:
        type: choice
        options: {{ range .Environments }}
          - {{ .Name }}{{ end }}
        default: '{{ .DefaultEnvironment }}'

jobs:
  {{- range .Environments }}
  deploy-{{ .Name }}:
    {{- if .Protection }}
    environment: {{ .Name }}
    {{- end }}
    runs-on: {{ index .Runners 0 }}
    steps:
      - name: Deploy to {{ .Name }}
        uses: {{ $.CustomActions.deploy }}
        with:
          stack-name: "{{ $.StackName }}"
          environment: "{{ .Name }}"
          sc-config: ${{ "{{" }} secrets.SC_CONFIG {{ "}}" }}
          {{- if .ValidationCommand }}
          validation-command: |
            {{ .ValidationCommand }}
          {{- end }}
          {{- range .DeployFlags }}
          sc-deploy-flags: "{{ . }}"
          {{- end }}
  {{- end }}
```

## 4. **Integration with Self-Contained Actions**

### **Automatic Action Selection**
```yaml
# In server.yaml - automatically maps to our self-contained actions
cicd:
  config:
    custom-actions:
      # Automatically resolves to our implementation
      deploy: "simple-container-com/api/.github/actions/deploy-client-stack@v1"
      provision: "simple-container-com/api/.github/actions/provision-parent-stack@v1"
      destroy-client: "simple-container-com/api/.github/actions/destroy-client-stack@v1"
      destroy-parent: "simple-container-com/api/.github/actions/destroy-parent-stack@v1"
```

### **Generated Workflow Benefits**
- **Zero External Dependencies**: Uses our self-contained actions
- **Organization Standards**: Consistent runners, secrets, validation
- **Environment Management**: Automatic GitHub Environment integration
- **Professional Notifications**: Slack/Discord with user mapping
- **Advanced Features**: PR previews, multi-stage deployments, rollbacks

## 5. **Real-World Generated Workflow Example**

**Input** (from server.yaml):
```yaml
cicd:
  type: github-actions
  config:
    organization:
      name: "acme-corp"
    environments:
      staging:
        auto-deploy: true
        runners: ["ubuntu-latest"]
      production:
        protection: true
        runners: ["blacksmith-8vcpu"]
        reviewers: ["devops-team"]
```

**Generated Output** (`.github/workflows/deploy.yml`):
```yaml
name: Deploy ACME Corp Application
on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      environment:
        type: choice
        options: [staging, production]

jobs:
  deploy-staging:
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - name: Deploy to Staging
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "acme-app"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          
  deploy-production:
    if: github.event_name == 'workflow_dispatch' && github.event.inputs.environment == 'production'
    environment: production
    runs-on: blacksmith-8vcpu-ubuntu-2204
    steps:
      - name: Deploy to Production
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "acme-app"
          environment: "production"
          sc-config: ${{ secrets.SC_CONFIG }}
```

## 6. **Implementation Plan**

### **Phase 1: Core Infrastructure**
1. ✅ Enhanced `ActionsCiCdConfig` struct with all new fields
2. ✅ Configuration validation and parsing
3. ✅ Basic workflow template engine
4. ✅ CLI command structure (`sc cicd generate`)

### **Phase 2: Template System**
1. ✅ Workflow template files for each action type
2. ✅ Template rendering with organization settings
3. ✅ Environment-specific configuration injection
4. ✅ Integration with self-contained actions

### **Phase 3: Advanced Features**
1. ✅ GitHub Environment integration
2. ✅ Notification system configuration
3. ✅ Custom runner support
4. ✅ Validation command integration

### **Phase 4: Organization Features**
1. ✅ Multi-stack workflow generation
2. ✅ Branch protection rule integration
3. ✅ Reviewer assignment automation
4. ✅ Workflow synchronization (`sc cicd sync`)

## 7. **Benefits for Organizations**

### **Standardization**
- **Consistent Workflows**: All projects use same patterns
- **Organization Policies**: Branch protection, reviewers, secrets
- **Runner Management**: Standardized compute resources
- **Security**: Centralized secret management

### **Developer Experience**  
- **Zero Setup**: Workflows auto-generated from infrastructure config
- **No GitHub Actions Knowledge**: Just configure server.yaml
- **Professional Quality**: Enterprise-grade workflows out of the box
- **Maintenance Free**: Updates via `sc cicd sync`

### **DevOps Benefits**
- **Infrastructure as Code**: CI/CD defined alongside infrastructure  
- **Version Control**: Workflow changes tracked with infrastructure
- **Audit Trail**: All changes through standard approval process
- **Compliance**: Consistent security and governance policies

This enhancement would transform Simple Container from having basic CI/CD configuration to a complete organizational GitHub Actions workflow generation system that integrates seamlessly with our new self-contained actions.

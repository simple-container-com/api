# BMAD Prototype Examples for Simple Container

## 🎯 Overview

This document provides concrete examples and prototypes demonstrating how the BMAD-inspired agentic system would work in practice. These examples show real user interactions, context documents, and agent behaviors for common Simple Container use cases.

## 🔍 Example 1: Go Microservice with Redis and MongoDB

### User Interaction Flow

```
User: "Help me set up Simple Container for my Go API project"

🧠 SC Analyst Agent (Alex): 
"I'll analyze your project to understand its requirements and resources."

[Analyzing project structure...]
[Running resource detection...]
[Assessing complexity...]

"Analysis complete! I've detected a Go microservice using Gin HTTP framework with:
• MongoDB database (85% confidence) - primary data storage
• Redis cache (80% confidence) - session and data caching  
• 17 environment variables - including database connections and API keys
• S3 integration (70% confidence) - file storage capabilities

This is a moderately complex setup that will benefit from managed database services. 
Handing off to our infrastructure architect..."

🏛️ SC DevOps Architect (Morgan):
"Based on Alex's analysis, I recommend a single-image deployment with managed services:
• MongoDB Atlas M10 cluster for reliability and automatic backups
• AWS ElastiCache Redis for high-performance caching
• S3 bucket with encryption for secure file storage
• Estimated monthly cost: $180-250

This architecture provides excellent scalability and ~99.5% uptime. 
Coordinating with our setup orchestrator..."

🎯 SC Setup Master (Jordan):
"Perfect! I'll orchestrate the complete setup process. This will take approximately 15-20 minutes:

Phase 1: Project structure setup (2-3 minutes)
Phase 2: Resource configuration (5-8 minutes)  
Phase 3: Application configuration (3-5 minutes)
Phase 4: Validation and testing (5-7 minutes)

I'll need a few configuration values from you - all will be encrypted securely.
Ready to proceed?"

User: "Yes, let's do it!"

🎯 SC Setup Master (Jordan):
"Excellent! I need these secure values:

1. MongoDB connection string (for database access)
   Example: mongodb+srv://<USER>:<PASS>@<cluster>.example.com/<dbname>

2. JWT secret key (for token signing)  
   Example: your-256-bit-secret-key

3. AWS credentials (for S3 access)
   - Access Key ID
   - Secret Access Key

Please provide these values:"

[User provides secure values]

⚙️ SC Config Executor (Casey):
"Generating configurations with security best practices...

✅ Created .sc/stacks/my-api/ directory structure
✅ Generated client.yaml with MongoDB, Redis, and S3 resources
✅ Encrypted secrets.yaml with 7 secure values
✅ Configured single-image deployment settings
✅ Validated all configurations

All configurations are ready and tested. Preparing for deployment validation..."

🚀 SC Deployment Specialist (Riley):
"Running final validations and deployment readiness checks...

✅ Configuration syntax validation passed
✅ Resource connectivity tests passed
✅ Security validation passed  
✅ Performance baseline established

Your Go microservice is ready for deployment!

Next steps:
• Run: sc deploy -s my-api -e staging
• Monitor: Available dashboards will be created automatically
• Access: Your API will be available at https://my-api-staging.simple-container.com

Setup complete! Your infrastructure is provisioned and ready."
```

### Generated Context Documents

#### Project Context Document
```yaml
# .sc-analysis/project-context-20241016-140000.md
metadata:
  agent_id: "sc-analyst"
  agent_name: "Alex"
  created_at: "2024-10-16T14:00:00Z"
  project_path: "/home/user/my-go-api"
  
content:
  project_profile:
    language: "Go"
    framework: "Gin HTTP v1.9.1"
    architecture_pattern: "microservice"
    complexity_score: 7.8
    estimated_setup_time: "15-20 minutes"
    
  detected_resources:
    databases:
      - type: "mongodb"
        confidence: 0.85
        purpose: "primary_database"
        evidence: ["models/user.go", "repositories/user_repository.go", "go.mod:go.mongodb.org/mongo-driver"]
      - type: "redis"
        confidence: 0.80
        purpose: "caching"
        evidence: ["cache/session_cache.go", "go.mod:github.com/go-redis/redis/v9"]
        
    storage:
      - type: "s3"
        confidence: 0.70
        purpose: "file_storage"
        evidence: ["handlers/file_upload.go", "go.mod:github.com/aws/aws-sdk-go-v2"]
        
  environment_analysis:
    variables_count: 17
    critical_secrets: ["DATABASE_URL", "REDIS_URL", "JWT_SECRET", "AWS_ACCESS_KEY_ID"]

handoff_instructions:
  next_agent: "sc-devops-architect"
  context_summary: "Go microservice with multi-database architecture requiring managed services"
  key_findings:
    - "Multi-database setup suggests production-ready application"
    - "Caching layer indicates performance optimization needs"
    - "File upload functionality requires secure cloud storage"
  ready_for_infrastructure_design: true
```

#### Infrastructure Strategy Document
```yaml
# .sc-analysis/infrastructure-strategy-20241016-140500.md
metadata:
  agent_id: "sc-devops-architect"
  agent_name: "Morgan"
  based_on: "project-context-20241016-140000"
  
content:
  infrastructure_architecture:
    deployment_strategy:
      selected: "single-image"
      rationale: "Go microservice with external dependencies suitable for containerized deployment"
      
    resource_architecture:
      database:
        resource_type: "mongodb-atlas"
        configuration:
          cluster_tier: "M10"
          region: "us-east-1"
      cache:
        resource_type: "aws-elasticache"
        configuration:
          node_type: "cache.t3.micro"
          
  cost_analysis:
    estimated_monthly: "$180-250"
    cost_breakdown:
      compute: "$80-120"
      database: "$60-90" 
      cache: "$15-20"
      storage: "$10-15"

handoff_instructions:
  next_agent: "sc-setup-master"
  context_summary: "Single-image deployment with managed MongoDB, Redis, and S3"
  ready_for_orchestration: true
```

---

## 🌐 Example 2: React + Node.js Full-Stack Application

### User Interaction Flow (Abbreviated)

```
User: "I have a React frontend with a Node.js backend that uses PostgreSQL and needs file uploads"

🧠 SC Analyst Agent: 
"I detect a full-stack JavaScript application:
• React frontend with TypeScript
• Node.js Express backend  
• PostgreSQL database
• File upload functionality
• 23 environment variables

This requires a cloud-compose deployment with separate frontend and backend services..."

🏛️ SC DevOps Architect:
"Recommending cloud-compose deployment:
• Frontend: Static hosting with CDN
• Backend: Node.js container with auto-scaling
• Database: Managed PostgreSQL (AWS RDS)
• Storage: S3 with CloudFront for uploaded files
• Estimated cost: $120-180/month"

🔧 SC Config Planner:
"Complex full-stack setup requires coordinated configuration:
• Frontend build and deployment pipeline
• Backend API with database connections
• CORS configuration between frontend/backend
• Shared environment variables and secrets"

🎯 SC Setup Master:
"I'll orchestrate this 4-phase setup:
Phase 1: Frontend configuration (5 minutes)
Phase 2: Backend API setup (8 minutes)  
Phase 3: Database and storage setup (10 minutes)
Phase 4: Integration and testing (7 minutes)
Total time: ~30 minutes"
```

### Generated client.yaml (Preview)
```yaml
stacks:
  staging:
    type: cloud-compose
    config:
      services:
        frontend:
          type: static-site
          buildCommand: "npm run build"
          buildDir: "dist"
          domain: "myapp-staging.simple-container.com"
          
        backend:
          type: single-image
          dockerfile: "backend/Dockerfile"
          port: 3000
          uses: [database, storage]
          secrets: [DATABASE_URL, JWT_SECRET, AWS_ACCESS_KEY_ID]
          
      uses:
        - database:
            type: postgres-rds
            instance: db.t3.micro
            
        - storage:
            type: s3-bucket
            versioning: true
            cdn: true
```

---

## 📱 Example 3: Simple Static Website

### User Interaction Flow (Abbreviated)

```
User: "I just want to deploy my HTML/CSS/JS website"

🧠 SC Analyst Agent:
"I detect a simple static website:
• HTML, CSS, JavaScript files
• No server-side processing required
• No database dependencies
• Minimal configuration needed

This is perfect for static hosting!"

🏛️ SC DevOps Architect:
"Recommending static site deployment:
• CDN hosting for global performance
• SSL certificate for security
• Custom domain support
• Estimated cost: $5-15/month"

🎯 SC Setup Master:
"This is a simple setup! Just 2 phases:
Phase 1: Build configuration (1 minute)
Phase 2: Deployment setup (2 minutes)
Total time: ~3 minutes

No sensitive configuration needed - let's proceed!"

⚙️ SC Config Executor:
"✅ Generated minimal client.yaml for static hosting
✅ No secrets required
✅ Optimized for fast global delivery

Your website will be available at: https://mysite.simple-container.com"
```

### Generated client.yaml (Simple)
```yaml
stacks:
  production:
    type: static-site
    config:
      buildCommand: "npm run build"  # Optional if build needed
      buildDir: "dist"               # Or "." for direct deployment
      domain: "mysite.simple-container.com"
      cdn: true
      compression: true
```

---

## 🔄 Example 4: Error Handling and Recovery

### Scenario: Network Connectivity Issue During Setup

```
🎯 SC Setup Master:
"Starting Phase 2: Resource configuration..."

⚙️ SC Config Executor:
"Configuring MongoDB Atlas connection..."
❌ Error: Connection timeout while validating MongoDB cluster

🎯 SC Setup Master:
"I encountered a connectivity issue. Let me diagnose and provide options:

Issue: Unable to connect to MongoDB Atlas
Possible causes:
1. Network connectivity issue
2. Incorrect connection string
3. Database cluster not ready

Options:
1. Retry with current settings (recommended)
2. Skip validation and proceed (advanced users)
3. Re-enter MongoDB connection string
4. Switch to different database provider

What would you like to do?"

User: "Let's retry"

🎯 SC Setup Master:
"Retrying with exponential backoff..."

⚙️ SC Config Executor:
"✅ MongoDB connection successful on retry
Continuing with resource configuration..."
```

### Advanced Error Recovery

```yaml
# Error Recovery Context Document
error_recovery:
  incident_id: "connectivity-20241016-141500"
  error_type: "network_connectivity"
  affected_phase: "resource_configuration"
  recovery_strategy: "retry_with_backoff"
  
  attempted_solutions:
    - solution: "immediate_retry"
      result: "failed"
      error: "same_timeout_error"
    - solution: "exponential_backoff_retry"
      result: "success"
      duration: "45_seconds"
      
  lessons_learned:
    - "MongoDB Atlas clusters may have initial connection delays"
    - "Exponential backoff is effective for transient network issues"
    
  prevention_measures:
    - "Add connection pre-validation step"
    - "Implement circuit breaker pattern for external services"
```

---

## 🧪 Agent Testing Scenarios

### SC Analyst Testing

```go
func TestSCAnalystAgent(t *testing.T) {
    testCases := []struct {
        name           string
        projectPath    string
        expectedLang   string
        expectedDBs    []string
        minConfidence  float64
    }{
        {
            name:           "Go microservice with MongoDB and Redis",
            projectPath:    "testdata/go-microservice",
            expectedLang:   "Go",
            expectedDBs:    []string{"mongodb", "redis"},
            minConfidence:  0.8,
        },
        {
            name:           "React frontend only",
            projectPath:    "testdata/react-frontend",
            expectedLang:   "JavaScript",
            expectedDBs:    []string{},
            minConfidence:  0.9,
        },
    }
    
    agent := NewSCAnalystAgent()
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            context := &AgentContext{
                ProjectContext: &ProjectAnalysisContext{
                    Path: tc.projectPath,
                },
            }
            
            task := &Task{
                Type: TaskTypeAnalyzeProject,
                Description: "Analyze project for setup requirements",
            }
            
            result, err := agent.Execute(context, task)
            assert.NoError(t, err)
            assert.True(t, result.Success)
            
            // Validate analysis results
            contextDoc := result.ContextDocument
            assert.Equal(t, tc.expectedLang, contextDoc.Content["project_profile"]["language"])
            
            detectedDBs := contextDoc.Content["detected_resources"]["databases"]
            for _, expectedDB := range tc.expectedDBs {
                assert.Contains(t, detectedDBs, expectedDB)
            }
            
            assert.GreaterOrEqual(t, contextDoc.Metadata.ConfidenceScore, tc.minConfidence)
        })
    }
}
```

### Integration Testing

```go
func TestFullWorkflowIntegration(t *testing.T) {
    // Setup test environment
    tempDir := t.TempDir()
    projectPath := setupTestProject(tempDir, "go-microservice-template")
    
    // Initialize orchestrator with all agents
    orchestrator := NewAgentOrchestrator()
    orchestrator.RegisterAgent(NewSCAnalystAgent())
    orchestrator.RegisterAgent(NewSCDevOpsArchitectAgent())
    orchestrator.RegisterAgent(NewSCSetupMasterAgent())
    orchestrator.RegisterAgent(NewSCConfigExecutorAgent())
    orchestrator.RegisterAgent(NewSCDeploymentSpecialist())
    
    // Execute full workflow
    userRequest := &UserRequest{
        Message: "Help me setup Simple Container",
        ProjectPath: projectPath,
    }
    
    response, err := orchestrator.ProcessUserRequest(userRequest)
    assert.NoError(t, err)
    assert.True(t, response.Success)
    
    // Validate workflow completion
    assert.Equal(t, WorkflowStatusComplete, response.Workflow.Status)
    assert.NotEmpty(t, response.GeneratedConfigurations)
    
    // Validate generated files
    clientYAML := filepath.Join(projectPath, ".sc/stacks/test-app/client.yaml")
    assert.FileExists(t, clientYAML)
    
    secretsYAML := filepath.Join(projectPath, ".sc/stacks/test-app/secrets.yaml")
    assert.FileExists(t, secretsYAML)
}
```

---

## 📊 Performance Benchmarks

### Agent Performance Targets

```yaml
performance_targets:
  sc_analyst:
    analysis_time: "<30 seconds for typical projects"
    memory_usage: "<100MB during analysis"
    accuracy_rate: ">95% resource detection accuracy"
    
  sc_devops_architect:
    strategy_generation: "<15 seconds"
    cost_calculation: "<5 seconds"  
    architecture_validation: "<10 seconds"
    
  sc_setup_master:
    orchestration_overhead: "<200ms per agent handoff"
    user_response_time: "<2 seconds"
    workflow_completion: "Target times ±20%"
    
  sc_config_executor:
    config_generation: "<30 seconds for complex setups"
    validation_time: "<45 seconds including connectivity tests"
    file_operations: "<5 seconds"
    
  sc_deployment_specialist:
    validation_suite: "<60 seconds full validation"
    deployment_execution: "<300 seconds typical deployment"
    monitoring_setup: "<30 seconds dashboard creation"
```

### Context Management Performance

```yaml
context_performance:
  context_transfer: "<100ms between agents"
  context_storage: "<50ms save/load operations"  
  context_cache_hit: ">80% cache hit rate"
  context_validation: "<10 seconds full validation"
  context_compression: ">50% size reduction for large contexts"
```

---

## 🎯 Success Criteria Validation

### User Experience Metrics
- **Setup Time**: 60% reduction vs manual configuration
- **Context Questions**: 95% reduction in repetitive questions  
- **User Satisfaction**: >90% positive feedback
- **Error Recovery**: >95% successful error resolution

### Technical Metrics
- **Agent Reliability**: >99% successful agent execution
- **Context Accuracy**: >95% accurate context transfer
- **Configuration Validity**: 100% valid generated configurations
- **Deployment Success**: >99% successful deployments

### Business Metrics
- **Time to Value**: 70% faster Simple Container adoption
- **User Retention**: 40% improvement in user onboarding completion
- **Support Tickets**: 50% reduction in setup-related support requests
- **User Growth**: 25% increase in successful Simple Container setups

---

**Conclusion**: These prototype examples demonstrate how the BMAD-inspired agentic system transforms Simple Container's AI Assistant from a generic conversational interface into an intelligent, specialized system that provides professional-grade setup automation with rich context understanding and seamless user experience.

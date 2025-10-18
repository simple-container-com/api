# BMAD Context Management for Simple Container

## 🧠 Overview

Context management is the cornerstone of the BMAD-inspired agentic system. This document details the advanced context engineering strategies that eliminate information loss, enable seamless agent handoffs, and provide rich, actionable intelligence throughout workflows.

## 🎯 Core Context Principles

### 1. Context-Driven Intelligence
- **Rich Context Documents**: Each agent operates with complete project understanding
- **No Repetitive Questions**: Context eliminates the need to re-ask for information
- **Actionable Intelligence**: Context provides specific, implementable guidance

### 2. Stateless Agent Design with Stateful Context
- **Externalized Intelligence**: All project understanding stored in context documents
- **Agent Reproducibility**: Same context + task = consistent results
- **Scalable Architecture**: Context can be cached, replicated, and distributed

### 3. Progressive Context Enhancement
- **Layered Knowledge Building**: Each agent adds specialized expertise to context
- **Context Validation**: Quality checks ensure context accuracy and completeness
- **Context Evolution**: Context documents evolve with project understanding

---

## 📋 Context Document Architecture

### Universal Context Document Structure

```yaml
# .sc-analysis/{agent-id}-{timestamp}.md
metadata:
  # Identity and Provenance
  agent_id: "sc-analyst"
  agent_name: "Alex"
  task_id: "analyze-project-20241016-001"
  created_at: "2024-10-16T14:40:00Z"
  updated_at: "2024-10-16T14:45:30Z"
  version: "1.2"
  
  # Context Lineage
  based_on: ["user-request-20241016-140000"]
  references: ["existing-analysis-20241015-100000"]
  dependencies: []
  
  # Project Context
  project_path: "/path/to/project"
  project_id: "my-go-api"
  workspace_id: "main"
  
  # Quality Metrics
  confidence_score: 0.95
  completeness_score: 0.88
  validation_errors: []
  
# Agent-Specific Content (varies by agent type)
content:
  # SC Analyst: project_profile, detected_resources, complexity_assessment
  # DevOps Architect: infrastructure_architecture, cost_analysis, security_design
  # Setup Master: workflow_plan, user_interaction_points, progress_tracking
  # Config Executor: generated_configurations, validation_results
  # Deployment Specialist: deployment_results, monitoring_setup, operational_guidance
  
# Standardized Handoff Instructions
handoff_instructions:
  next_agent: "sc-devops-architect"
  context_summary: "Go microservice with MongoDB, Redis, and S3 detected"
  key_decisions:
    - "Multi-database architecture requiring managed services"
    - "Caching layer suggests performance optimization needs"
    - "File upload capabilities requiring cloud storage"
  execution_readiness: "analysis_complete"
  recommended_actions: ["infrastructure_design", "deployment_strategy_selection"]
  
# Context Relationships and Cross-References
relationships:
  supersedes: ["previous-analysis-id"]
  relates_to: ["infrastructure-strategy-id", "user-preferences-id"]
  validates: []
  invalidates: ["outdated-analysis-id"]
```

### Context Document Types

#### 1. Project Context Document (SC Analyst)
```yaml
# Focus: Comprehensive project understanding
content:
  project_profile:
    language: "Go"
    framework: "Gin HTTP + Cobra CLI"
    architecture_pattern: "microservice"
    complexity_score: 7.8
    
  detected_resources:
    databases: [...]
    storage: [...]
    external_apis: [...]
    environment_variables: {...}
    
  deployment_recommendations:
    primary_strategy: "single-image"
    scaling_approach: "horizontal"
    resource_requirements: {...}
```

#### 2. Infrastructure Strategy Document (DevOps Architect)
```yaml
# Focus: Infrastructure design and resource architecture
content:
  infrastructure_architecture:
    deployment_strategy: {...}
    resource_architecture: {...}
    cost_optimization: {...}
    security_architecture: {...}
    monitoring_strategy: {...}
```

#### 3. Setup Workflow Document (Setup Master)
```yaml
# Focus: Orchestration and user experience
content:
  workflow_plan:
    setup_phases: [...]
    total_duration: "15-20 minutes"
    
  user_interaction_points:
    required_inputs: [...]
    optional_preferences: [...]
    
  progress_tracking:
    total_steps: 12
    progress_indicators: [...]
```

#### 4. Execution Log Document (Config Executor)
```yaml
# Focus: Configuration generation and validation
content:
  generated_configurations:
    client_yaml: {...}
    secrets_yaml: {...}
    
  validation_results:
    overall_status: "passed"
    tests_run: 15
    connectivity_tests: {...}
    security_tests: {...}
```

#### 5. Deployment Results Document (Deployment Specialist)
```yaml
# Focus: Deployment validation and operational readiness
content:
  deployment_validation: {...}
  deployment_results: {...}
  monitoring_setup: {...}
  operational_guidance: {...}
```

---

## 🔄 Context Transfer Patterns

### Pattern 1: Sequential Context Enhancement

```go
type ContextTransfer struct {
    sourceAgent   string
    targetAgent   string
    sourceContext *ContextDocument
    transferRules []TransferRule
}

type TransferRule struct {
    SourcePath   string // JSONPath to source data
    TargetPath   string // JSONPath to target location
    Transform    func(interface{}) interface{} // Optional transformation
    Required     bool   // Whether this transfer is mandatory
    Validation   func(interface{}) error      // Validation function
}

func (ct *ContextTransfer) Execute() (*ContextDocument, error) {
    targetContext := &ContextDocument{
        Metadata: ct.buildTargetMetadata(),
        Content:  make(map[string]interface{}),
    }
    
    // Apply transfer rules
    for _, rule := range ct.transferRules {
        sourceData := ct.extractData(rule.SourcePath)
        if sourceData == nil && rule.Required {
            return nil, fmt.Errorf("required data missing: %s", rule.SourcePath)
        }
        
        // Apply transformation if specified
        targetData := sourceData
        if rule.Transform != nil {
            targetData = rule.Transform(sourceData)
        }
        
        // Validate transferred data
        if rule.Validation != nil {
            if err := rule.Validation(targetData); err != nil {
                return nil, fmt.Errorf("validation failed for %s: %w", rule.TargetPath, err)
            }
        }
        
        ct.setData(targetContext, rule.TargetPath, targetData)
    }
    
    return targetContext, nil
}
```

### Pattern 2: Context Enrichment

```go
type ContextEnricher struct {
    enrichers map[string]Enricher
}

type Enricher interface {
    Enrich(ctx *ContextDocument) error
    Priority() int
    Dependencies() []string
}

// Example: Project Analysis Enricher
type ProjectAnalysisEnricher struct {
    analyzer *analysis.ProjectAnalyzer
}

func (pe *ProjectAnalysisEnricher) Enrich(ctx *ContextDocument) error {
    projectPath := ctx.Metadata.ProjectPath
    if projectPath == "" {
        return errors.New("project path not specified in context")
    }
    
    // Perform analysis and enrich context
    analysisResult, err := pe.analyzer.AnalyzeProject(projectPath)
    if err != nil {
        return fmt.Errorf("project analysis failed: %w", err)
    }
    
    // Add enriched data to context
    ctx.Content["project_analysis"] = map[string]interface{}{
        "detected_languages": analysisResult.Languages,
        "detected_frameworks": analysisResult.Frameworks,
        "detected_resources": analysisResult.Resources,
        "complexity_metrics": analysisResult.ComplexityMetrics,
    }
    
    // Update metadata
    ctx.Metadata.ConfidenceScore = analysisResult.ConfidenceScore
    ctx.Metadata.UpdatedAt = time.Now()
    
    return nil
}
```

### Pattern 3: Context Validation

```go
type ContextValidator struct {
    validators []Validator
    schema     *ContextSchema
}

type Validator interface {
    Validate(ctx *ContextDocument) []ValidationError
    Severity() ValidationSeverity
}

type ValidationError struct {
    Path        string
    Message     string
    Severity    ValidationSeverity
    Suggestion  string
}

type ValidationSeverity int

const (
    ValidationInfo ValidationSeverity = iota
    ValidationWarning
    ValidationError
    ValidationCritical
)

func (cv *ContextValidator) ValidateContext(ctx *ContextDocument) ValidationResult {
    var errors []ValidationError
    
    // Schema validation
    if schemaErrors := cv.validateSchema(ctx); len(schemaErrors) > 0 {
        errors = append(errors, schemaErrors...)
    }
    
    // Custom validators
    for _, validator := range cv.validators {
        validationErrors := validator.Validate(ctx)
        errors = append(errors, validationErrors...)
    }
    
    return ValidationResult{
        Valid:      len(errors) == 0,
        Errors:     errors,
        Score:      cv.calculateValidationScore(errors),
        ValidatedAt: time.Now(),
    }
}
```

---

## 🚀 Context Storage and Persistence

### File-Based Context Storage

```go
type FileContextStorage struct {
    basePath    string
    encryptor   crypto.Encryptor
    compressor  compression.Compressor
}

func (fs *FileContextStorage) SaveContext(ctx *ContextDocument) error {
    // Generate file path
    filePath := fs.generateFilePath(ctx)
    
    // Serialize context
    data, err := json.Marshal(ctx)
    if err != nil {
        return fmt.Errorf("context serialization failed: %w", err)
    }
    
    // Encrypt sensitive data
    if fs.containsSensitiveData(ctx) {
        data, err = fs.encryptor.Encrypt(data)
        if err != nil {
            return fmt.Errorf("context encryption failed: %w", err)
        }
    }
    
    // Compress if beneficial
    if len(data) > 1024 { // Only compress larger contexts
        data, err = fs.compressor.Compress(data)
        if err != nil {
            return fmt.Errorf("context compression failed: %w", err)
        }
    }
    
    // Write to file
    return os.WriteFile(filePath, data, 0600)
}

func (fs *FileContextStorage) LoadContext(contextID string) (*ContextDocument, error) {
    filePath := fs.resolveFilePath(contextID)
    
    data, err := os.ReadFile(filePath)
    if err != nil {
        return nil, fmt.Errorf("context file read failed: %w", err)
    }
    
    // Decompress if needed
    if fs.isCompressed(data) {
        data, err = fs.compressor.Decompress(data)
        if err != nil {
            return nil, fmt.Errorf("context decompression failed: %w", err)
        }
    }
    
    // Decrypt if needed
    if fs.isEncrypted(data) {
        data, err = fs.encryptor.Decrypt(data)
        if err != nil {
            return nil, fmt.Errorf("context decryption failed: %w", err)
        }
    }
    
    // Deserialize context
    var ctx ContextDocument
    if err := json.Unmarshal(data, &ctx); err != nil {
        return nil, fmt.Errorf("context deserialization failed: %w", err)
    }
    
    return &ctx, nil
}
```

### Context Storage Structure

```
.sc-analysis/
├── project-context-20241016-140000.md         # SC Analyst output
├── infrastructure-strategy-20241016-142000.md # DevOps Architect output  
├── configuration-strategy-20241016-143000.md  # Config Planner output
├── setup-workflow-20241016-144000.md          # Setup Master output
├── execution-log-20241016-145000.md           # Config Executor output
├── deployment-results-20241016-150000.md      # Deployment Specialist output
├── .context-cache/                            # Cached context data
│   ├── compressed/                            # Compressed contexts
│   └── encrypted/                             # Encrypted sensitive contexts
└── .context-index.json                        # Context document index
```

---

## 📊 Context Performance Optimization

### Context Caching Strategy

```go
type ContextCache struct {
    cache    map[string]*CachedContext
    index    *ContextIndex
    metrics  *CacheMetrics
    mutex    sync.RWMutex
}

type CachedContext struct {
    Document   *ContextDocument
    CachedAt   time.Time
    AccessedAt time.Time
    AccessCount int
    Size       int64
}

func (cc *ContextCache) Get(contextID string) (*ContextDocument, bool) {
    cc.mutex.RLock()
    defer cc.mutex.RUnlock()
    
    cached, exists := cc.cache[contextID]
    if !exists {
        cc.metrics.CacheMisses++
        return nil, false
    }
    
    // Check TTL
    if time.Since(cached.CachedAt) > cc.getTTL(contextID) {
        delete(cc.cache, contextID)
        cc.metrics.CacheExpiries++
        return nil, false
    }
    
    // Update access metrics
    cached.AccessedAt = time.Now()
    cached.AccessCount++
    cc.metrics.CacheHits++
    
    return cached.Document, true
}

func (cc *ContextCache) Set(contextID string, doc *ContextDocument) {
    cc.mutex.Lock()
    defer cc.mutex.Unlock()
    
    // Check cache size limits
    if cc.shouldEvict() {
        cc.evictLRU()
    }
    
    cached := &CachedContext{
        Document:    doc,
        CachedAt:    time.Now(),
        AccessedAt:  time.Now(),
        AccessCount: 0,
        Size:        cc.calculateSize(doc),
    }
    
    cc.cache[contextID] = cached
    cc.index.AddContext(contextID, doc.Metadata)
}
```

### Context Compression

```go
type ContextCompressor struct {
    algorithm compression.Algorithm
    threshold int64 // Minimum size to compress
}

func (cc *ContextCompressor) ShouldCompress(ctx *ContextDocument) bool {
    size := cc.estimateSize(ctx)
    return size > cc.threshold
}

func (cc *ContextCompressor) Compress(ctx *ContextDocument) (*ContextDocument, error) {
    // Identify compressible sections
    compressibleSections := []string{
        "content.detected_resources.environment_variables",
        "content.generated_configurations",
        "content.validation_results.detailed_logs",
    }
    
    compressed := ctx.DeepCopy()
    
    for _, section := range compressibleSections {
        data := cc.extractSection(compressed, section)
        if data != nil {
            compressedData, err := cc.algorithm.Compress(data)
            if err != nil {
                return nil, err
            }
            cc.setSection(compressed, section, compressedData)
            cc.markAsCompressed(compressed, section)
        }
    }
    
    return compressed, nil
}
```

---

## 🔒 Context Security

### Sensitive Data Detection

```go
type SensitiveDataDetector struct {
    patterns []SensitivePattern
    scanner  *SecretScanner
}

type SensitivePattern struct {
    Name        string
    Pattern     *regexp.Regexp
    Severity    SensitivityLevel
    Mask        bool
    Encrypt     bool
}

type SensitivityLevel int

const (
    SensitivityLow SensitivityLevel = iota
    SensitivityMedium
    SensitivityHigh
    SensitivityCritical
)

func (sdd *SensitiveDataDetector) ScanContext(ctx *ContextDocument) (*SensitivityReport, error) {
    report := &SensitivityReport{
        ContextID: ctx.Metadata.TaskID,
        ScannedAt: time.Now(),
    }
    
    // Scan context content recursively
    findings := sdd.scanRecursively(ctx.Content, "")
    
    for _, finding := range findings {
        report.Findings = append(report.Findings, SensitiveFinding{
            Path:        finding.Path,
            Type:        finding.Type,
            Severity:    finding.Severity,
            Value:       finding.MaskedValue,
            Suggestion:  finding.Suggestion,
        })
        
        // Apply security measures based on severity
        if finding.Severity >= SensitivityHigh {
            if err := sdd.applySecurityMeasures(ctx, finding); err != nil {
                return nil, err
            }
        }
    }
    
    return report, nil
}
```

### Context Encryption

```go
type ContextEncryption struct {
    keyManager  crypto.KeyManager
    algorithms  map[SensitivityLevel]crypto.Algorithm
}

func (ce *ContextEncryption) EncryptSensitiveData(ctx *ContextDocument, findings []SensitiveFinding) error {
    for _, finding := range findings {
        if finding.Severity >= SensitivityHigh {
            algorithm := ce.algorithms[finding.Severity]
            key := ce.keyManager.GetKey(ctx.Metadata.ProjectID, finding.Severity)
            
            encryptedValue, err := algorithm.Encrypt(finding.Value, key)
            if err != nil {
                return fmt.Errorf("encryption failed for %s: %w", finding.Path, err)
            }
            
            // Replace original value with encrypted version
            ce.setContextValue(ctx, finding.Path, EncryptedValue{
                Algorithm:     algorithm.Name(),
                EncryptedData: encryptedValue,
                KeyID:         key.ID(),
                CreatedAt:     time.Now(),
            })
        }
    }
    
    return nil
}
```

---

## 📈 Context Quality Metrics

### Context Completeness Scoring

```go
type CompletenessScorer struct {
    requirements map[string]RequirementSet
}

type RequirementSet struct {
    Required []string // Required fields
    Optional []string // Optional fields
    Weights  map[string]float64 // Field importance weights
}

func (cs *CompletenessScorer) CalculateScore(ctx *ContextDocument) float64 {
    agentType := ctx.Metadata.AgentID
    requirements, exists := cs.requirements[agentType]
    if !exists {
        return 1.0 // No requirements defined
    }
    
    var totalWeight, achievedWeight float64
    
    // Check required fields
    for _, field := range requirements.Required {
        weight := requirements.Weights[field]
        if weight == 0 {
            weight = 1.0 // Default weight
        }
        
        totalWeight += weight
        if cs.hasField(ctx, field) {
            achievedWeight += weight
        }
    }
    
    // Check optional fields
    for _, field := range requirements.Optional {
        weight := requirements.Weights[field] * 0.5 // Optional fields worth less
        totalWeight += weight
        if cs.hasField(ctx, field) {
            achievedWeight += weight
        }
    }
    
    if totalWeight == 0 {
        return 1.0
    }
    
    return achievedWeight / totalWeight
}
```

### Context Confidence Scoring

```go
type ConfidenceScorer struct {
    scorers []ConfidenceScorer
}

type ConfidenceScorer interface {
    CalculateConfidence(ctx *ContextDocument) (float64, error)
    Weight() float64
}

// Example: Resource Detection Confidence Scorer
type ResourceDetectionConfidenceScorer struct{}

func (rdcs *ResourceDetectionConfidenceScorer) CalculateConfidence(ctx *ContextDocument) (float64, error) {
    resources, err := rdcs.extractResources(ctx)
    if err != nil {
        return 0, err
    }
    
    var totalConfidence float64
    var resourceCount int
    
    for _, resource := range resources {
        if confidence := resource.Confidence; confidence > 0 {
            totalConfidence += confidence
            resourceCount++
        }
    }
    
    if resourceCount == 0 {
        return 1.0, nil // No resources detected is still valid
    }
    
    return totalConfidence / float64(resourceCount), nil
}

func (rdcs *ResourceDetectionConfidenceScorer) Weight() float64 {
    return 0.3 // 30% of overall confidence
}
```

---

## 🎯 Context Success Metrics

### Quantitative Metrics
- **Context Transfer Success Rate**: >99% successful transfers between agents
- **Context Completeness**: >90% average completeness score
- **Context Confidence**: >85% average confidence score
- **Context Cache Hit Rate**: >80% cache hit rate
- **Context Validation Pass Rate**: >95% validation success

### Qualitative Metrics
- **Information Preservation**: No critical information lost in transfers
- **Context Actionability**: Context enables immediate action by receiving agents
- **Context Clarity**: Context documents are clear and understandable
- **Context Relevance**: Context contains only relevant, actionable information

---

**Next Steps**: Review this context management strategy and proceed to begin implementation using the established [`IMPLEMENTATION_ROADMAP.md`](./IMPLEMENTATION_ROADMAP.md).

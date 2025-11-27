package github

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/simple-container-com/api/pkg/api"
)

// WorkflowGenerator generates GitHub Actions workflows from Simple Container configuration
type WorkflowGenerator struct {
	config     *EnhancedActionsCiCdConfig
	stackName  string
	outputPath string
	templates  map[string]*template.Template
}

// WorkflowTemplateData contains data passed to workflow templates
type WorkflowTemplateData struct {
	StackName          string
	Organization       OrganizationConfig
	Environments       map[string]EnvironmentConfig
	CustomActions      map[string]string
	RequiredSecrets    []string
	DefaultBranch      string
	DefaultEnvironment string
	Notifications      NotificationConfig
	Execution          ExecutionConfig
	Validation         ValidationConfig
	SCVersion          string
}

// NewWorkflowGenerator creates a new workflow generator
func NewWorkflowGenerator(config *EnhancedActionsCiCdConfig, stackName, outputPath string) *WorkflowGenerator {
	return &WorkflowGenerator{
		config:     config,
		stackName:  stackName,
		outputPath: outputPath,
		templates:  make(map[string]*template.Template),
	}
}

// LoadTemplates loads workflow templates from embedded templates
func (wg *WorkflowGenerator) LoadTemplates() error {
	templates := map[string]string{
		"deploy":         deployTemplate,
		"destroy":        destroyTemplate,
		"destroy-parent": destroyParentTemplate,
		"provision":      provisionTemplate,
		"pr-preview":     prPreviewTemplate,
	}

	for name, tmplContent := range templates {
		tmpl, err := template.New(name).Funcs(templateFuncs()).Parse(tmplContent)
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", name, err)
		}
		wg.templates[name] = tmpl
	}

	return nil
}

// GenerateWorkflows generates all configured workflow files
func (wg *WorkflowGenerator) GenerateWorkflows() error {
	if err := wg.LoadTemplates(); err != nil {
		return fmt.Errorf("failed to load templates: %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(wg.outputPath, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Prepare template data
	templateData := wg.prepareTemplateData()

	// Generate workflows for each configured template
	for _, templateName := range wg.config.WorkflowGeneration.Templates {
		if err := wg.generateWorkflow(templateName, templateData); err != nil {
			return fmt.Errorf("failed to generate workflow %s: %w", templateName, err)
		}
	}

	return nil
}

// prepareTemplateData prepares data for template rendering
func (wg *WorkflowGenerator) prepareTemplateData() *WorkflowTemplateData {
	// Determine default environment (first staging, then first production, then first overall)
	defaultEnv := wg.getDefaultEnvironment()

	// Ensure defaults are applied
	scVersion := wg.config.WorkflowGeneration.SCVersion
	if scVersion == "" {
		scVersion = "latest"
	}

	// Ensure concurrency group has a default
	concurrencyGroup := wg.config.Execution.Concurrency.Group
	if concurrencyGroup == "" {
		concurrencyGroup = fmt.Sprintf("deploy-%s-${{ github.ref }}", wg.stackName)
	}

	// Update the execution config with defaults
	execution := wg.config.Execution
	execution.Concurrency.Group = concurrencyGroup

	// Ensure custom actions have defaults with proper versioning
	customActions := wg.config.WorkflowGeneration.CustomActions
	if len(customActions) == 0 {
		// Use SCVersion for action versioning, defaulting to @main for latest
		actionVersion := "@main" // Use main branch by default for latest version
		if scVersion != "" && scVersion != "latest" {
			actionVersion = "@" + scVersion // Use specific CalVer tag if provided
		}

		customActions = map[string]string{
			"deploy":       "simple-container-com/api/.github/actions/deploy" + actionVersion,
			"destroy":      "simple-container-com/api/.github/actions/destroy" + actionVersion,
			"provision":    "simple-container-com/api/.github/actions/provision" + actionVersion,
			"cancel-stack": "simple-container-com/api/.github/actions/cancel-stack" + actionVersion,
		}
	}

	return &WorkflowTemplateData{
		StackName:          wg.stackName,
		Organization:       wg.config.Organization,
		Environments:       wg.config.Environments,
		CustomActions:      customActions,
		RequiredSecrets:    wg.config.GetRequiredSecrets(),
		DefaultBranch:      wg.config.Organization.DefaultBranch,
		DefaultEnvironment: defaultEnv,
		Notifications:      wg.config.Notifications,
		Execution:          execution,
		Validation:         wg.config.Validation,
		SCVersion:          scVersion,
	}
}

// getDefaultEnvironment determines the default environment for deployments
func (wg *WorkflowGenerator) getDefaultEnvironment() string {
	// First priority: environments with auto-deploy enabled
	for name, env := range wg.config.Environments {
		if env.AutoDeploy {
			return name
		}
	}

	// Second priority: staging environments (by type)
	for name, env := range wg.config.Environments {
		if env.Type == "staging" {
			return name
		}
	}

	// Third priority: environments named "staging"
	if _, exists := wg.config.Environments["staging"]; exists {
		return "staging"
	}

	// Fourth priority: production environments (by type)
	for name, env := range wg.config.Environments {
		if env.Type == "production" {
			return name
		}
	}

	// Fall back to any environment
	for name := range wg.config.Environments {
		return name
	}

	return "staging" // Default fallback
}

// generateWorkflow generates a single workflow file
func (wg *WorkflowGenerator) generateWorkflow(templateName string, data *WorkflowTemplateData) error {
	tmpl, exists := wg.templates[templateName]
	if !exists {
		return fmt.Errorf("template %s not found", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	filename := fmt.Sprintf("%s-%s.yml", templateName, wg.stackName)
	filepath := filepath.Join(wg.outputPath, filename)

	if err := os.WriteFile(filepath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write workflow file %s: %w", filepath, err)
	}

	fmt.Printf("Generated workflow: %s\n", filepath)
	return nil
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"join":      strings.Join,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"title": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"quote": func(s string) string {
			return fmt.Sprintf(`"%s"`, s)
		},
		"yamlList": func(items []string) string {
			if len(items) == 0 {
				return "[]"
			}
			var result []string
			for _, item := range items {
				result = append(result, fmt.Sprintf(`"%s"`, item))
			}
			return "[" + strings.Join(result, ", ") + "]"
		},
		"envNamesExcluding": func(environments map[string]EnvironmentConfig, excludeType string) string {
			var names []string
			for name, env := range environments {
				if env.Type != excludeType {
					names = append(names, name)
				}
			}
			return strings.Join(names, ", ")
		},
		"timeoutMinutes": func(timeout string) string {
			// Remove 'm' suffixes and any other non-numeric characters, keeping only the number
			cleaned := strings.ReplaceAll(timeout, "m", "")
			cleaned = strings.ReplaceAll(cleaned, "minutes", "")
			cleaned = strings.TrimSpace(cleaned)
			if cleaned == "" {
				return "30"
			}
			return cleaned
		},
		"defaultAction": func(actionType, scVersion string) string {
			// Build default action reference with proper versioning
			baseAction := "simple-container-com/api/.github/actions/" + actionType

			// Use SCVersion for action versioning, defaulting to @main for latest
			if scVersion == "" || scVersion == "latest" {
				return baseAction + "@main" // Use main branch for latest version
			}
			return baseAction + "@" + scVersion // Use specific CalVer tag
		},
		"indent": func(spaces int, text string) string {
			indent := strings.Repeat(" ", spaces)
			lines := strings.Split(text, "\n")
			var indentedLines []string
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					indentedLines = append(indentedLines, indent+line)
				} else {
					indentedLines = append(indentedLines, "")
				}
			}
			return strings.Join(indentedLines, "\n")
		},
		"secretRef": func(secret string) string {
			return fmt.Sprintf("${{ secrets.%s }}", secret)
		},
		"envVarRef": func(envVar string) string {
			return fmt.Sprintf("${{ github.event.inputs.%s }}", envVar)
		},
		"replace": func(input, old, new string) string {
			return strings.ReplaceAll(input, old, new)
		},
	}
}

// ValidateConfiguration validates the enhanced CI/CD configuration
func ValidateConfiguration(config *api.Config) error {
	enhancedConfig, err := ConvertToEnhancedConfig(config)
	if err != nil {
		return fmt.Errorf("failed to convert config: %w", err)
	}

	enhancedConfig.SetDefaults()
	return enhancedConfig.Validate()
}

// ConvertToEnhancedConfig converts a generic config to enhanced CI/CD config
func ConvertToEnhancedConfig(config *api.Config) (*EnhancedActionsCiCdConfig, error) {
	enhanced := &EnhancedActionsCiCdConfig{}
	// TODO: Implement proper config conversion
	return enhanced, nil
}

// GenerateWorkflowsFromServerConfig generates workflows from server.yaml configuration
func GenerateWorkflowsFromServerConfig(serverDesc *api.ServerDescriptor, stackName, outputPath string) error {
	if serverDesc.CiCd.Type != CiCdTypeGithubActions {
		return fmt.Errorf("unsupported CI/CD type: %s", serverDesc.CiCd.Type)
	}

	enhancedConfig, err := ConvertToEnhancedConfig(&serverDesc.CiCd.Config)
	if err != nil {
		return fmt.Errorf("failed to convert CI/CD config: %w", err)
	}

	enhancedConfig.SetDefaults()
	if err := enhancedConfig.Validate(); err != nil {
		return fmt.Errorf("invalid CI/CD configuration: %w", err)
	}

	if !enhancedConfig.IsWorkflowGenerationEnabled() {
		return fmt.Errorf("workflow generation is not enabled in configuration")
	}

	generator := NewWorkflowGenerator(enhancedConfig, stackName, outputPath)
	return generator.GenerateWorkflows()
}

// SyncWorkflows updates existing workflows based on configuration changes
func SyncWorkflows(serverDesc *api.ServerDescriptor, stackName, workflowsPath string) error {
	if serverDesc.CiCd.Type != CiCdTypeGithubActions {
		return fmt.Errorf("unsupported CI/CD type: %s", serverDesc.CiCd.Type)
	}

	enhancedConfig, err := ConvertToEnhancedConfig(&serverDesc.CiCd.Config)
	if err != nil {
		return fmt.Errorf("failed to convert CI/CD config: %w", err)
	}

	if !enhancedConfig.WorkflowGeneration.AutoUpdate {
		fmt.Println("Auto-update is disabled, skipping workflow sync")
		return nil
	}

	// Remove old workflows if they exist
	oldWorkflows := []string{
		fmt.Sprintf("deploy-%s.yml", stackName),
		fmt.Sprintf("destroy-%s.yml", stackName),
		fmt.Sprintf("provision-%s.yml", stackName),
		fmt.Sprintf("pr-preview-%s.yml", stackName),
	}

	for _, workflow := range oldWorkflows {
		oldPath := filepath.Join(workflowsPath, workflow)
		if _, err := os.Stat(oldPath); err == nil {
			fmt.Printf("Removing old workflow: %s\n", oldPath)
			os.Remove(oldPath)
		}
	}

	// Generate new workflows
	return GenerateWorkflowsFromServerConfig(serverDesc, stackName, workflowsPath)
}

// GetWorkflowTemplateNames returns available workflow template names
func GetWorkflowTemplateNames() []string {
	return []string{"deploy", "destroy", "destroy-parent", "provision", "pr-preview"}
}

// PreviewWorkflow generates a workflow without writing to file (for preview)
func PreviewWorkflow(serverDesc *api.ServerDescriptor, stackName, templateName string) (string, error) {
	if serverDesc.CiCd.Type != CiCdTypeGithubActions {
		return "", fmt.Errorf("unsupported CI/CD type: %s", serverDesc.CiCd.Type)
	}

	enhancedConfig, err := ConvertToEnhancedConfig(&serverDesc.CiCd.Config)
	if err != nil {
		return "", fmt.Errorf("failed to convert CI/CD config: %w", err)
	}

	enhancedConfig.SetDefaults()

	generator := NewWorkflowGenerator(enhancedConfig, stackName, "")
	if err := generator.LoadTemplates(); err != nil {
		return "", fmt.Errorf("failed to load templates: %w", err)
	}

	tmpl, exists := generator.templates[templateName]
	if !exists {
		return "", fmt.Errorf("template %s not found", templateName)
	}

	templateData := generator.prepareTemplateData()
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// ValidationResults contains the results of workflow validation
type ValidationResults struct {
	IsValid       bool
	ValidFiles    []string
	MissingFiles  []string
	OutdatedFiles []string
	InvalidFiles  map[string][]string
	Differences   map[string][]string
}

// TotalIssues returns the total number of validation issues
func (vr *ValidationResults) TotalIssues() int {
	return len(vr.MissingFiles) + len(vr.OutdatedFiles) + len(vr.InvalidFiles)
}

// SyncPlan contains the plan for synchronizing workflows
type SyncPlan struct {
	FilesToCreate []string
	FilesToUpdate []FileUpdate
	FilesToRemove []string
}

// FileUpdate represents a file that needs to be updated
type FileUpdate struct {
	File    string
	Changes []string
}

// IsUpToDate returns true if no changes are needed
func (sp *SyncPlan) IsUpToDate() bool {
	return len(sp.FilesToCreate) == 0 && len(sp.FilesToUpdate) == 0 && len(sp.FilesToRemove) == 0
}

// WorkflowPreview contains preview data for workflows
type WorkflowPreview struct {
	StackName string
	Config    *EnhancedActionsCiCdConfig
	Workflows []WorkflowInfo
}

// WorkflowInfo contains information about a workflow
type WorkflowInfo struct {
	Name        string
	FileName    string
	Description string
	Content     string
	Triggers    []string
	Jobs        []JobInfo
}

// JobInfo contains information about a workflow job
type JobInfo struct {
	Name        string
	Runner      string
	Environment string
	Steps       []StepInfo
}

// StepInfo contains information about a workflow step
type StepInfo struct {
	Name   string
	Action string
}

// ValidateWorkflows validates existing workflow files against configuration
func (wg *WorkflowGenerator) ValidateWorkflows() (*ValidationResults, error) {
	results := &ValidationResults{
		IsValid:       true,
		ValidFiles:    []string{},
		MissingFiles:  []string{},
		OutdatedFiles: []string{},
		InvalidFiles:  make(map[string][]string),
		Differences:   make(map[string][]string),
	}

	if err := wg.LoadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	templateData := wg.prepareTemplateData()

	for _, templateName := range wg.config.WorkflowGeneration.Templates {
		filename := fmt.Sprintf("%s-%s.yml", templateName, wg.stackName)
		filepath := filepath.Join(wg.outputPath, filename)

		// Check if file exists
		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			results.MissingFiles = append(results.MissingFiles, filename)
			results.IsValid = false
			continue
		}

		// Read existing file
		existingContent, err := os.ReadFile(filepath)
		if err != nil {
			results.InvalidFiles[filename] = []string{fmt.Sprintf("Could not read file: %v", err)}
			results.IsValid = false
			continue
		}

		// Generate expected content
		expectedContent, err := wg.generateWorkflowContent(templateName, templateData)
		if err != nil {
			results.InvalidFiles[filename] = []string{fmt.Sprintf("Could not generate expected content: %v", err)}
			results.IsValid = false
			continue
		}

		// Compare content
		if string(existingContent) != expectedContent {
			results.OutdatedFiles = append(results.OutdatedFiles, filename)
			results.Differences[filename] = []string{"Content differs from expected"}
			results.IsValid = false
		} else {
			results.ValidFiles = append(results.ValidFiles, filename)
		}
	}

	return results, nil
}

// GetSyncPlan creates a synchronization plan for workflows
func (wg *WorkflowGenerator) GetSyncPlan() (*SyncPlan, error) {
	plan := &SyncPlan{
		FilesToCreate: []string{},
		FilesToUpdate: []FileUpdate{},
		FilesToRemove: []string{},
	}

	if err := wg.LoadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	templateData := wg.prepareTemplateData()

	// Check each configured template
	for _, templateName := range wg.config.WorkflowGeneration.Templates {
		filename := fmt.Sprintf("%s-%s.yml", templateName, wg.stackName)
		filepath := filepath.Join(wg.outputPath, filename)

		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			// File doesn't exist, needs to be created
			plan.FilesToCreate = append(plan.FilesToCreate, filename)
		} else {
			// File exists, check if it needs updating
			existingContent, err := os.ReadFile(filepath)
			if err != nil {
				continue // Skip files we can't read
			}

			expectedContent, err := wg.generateWorkflowContent(templateName, templateData)
			if err != nil {
				continue // Skip templates we can't generate
			}

			if string(existingContent) != expectedContent {
				plan.FilesToUpdate = append(plan.FilesToUpdate, FileUpdate{
					File:    filename,
					Changes: []string{"Content updated to match configuration"},
				})
			}
		}
	}

	return plan, nil
}

// SyncWorkflows executes the synchronization plan
func (wg *WorkflowGenerator) SyncWorkflows(plan *SyncPlan) error {
	if err := wg.LoadTemplates(); err != nil {
		return fmt.Errorf("failed to load templates: %w", err)
	}

	templateData := wg.prepareTemplateData()

	// Create new files
	for _, filename := range plan.FilesToCreate {
		templateName := strings.Split(filename, "-")[0] // Extract template name
		if err := wg.generateWorkflow(templateName, templateData); err != nil {
			return fmt.Errorf("failed to create workflow %s: %w", filename, err)
		}
	}

	// Update existing files
	for _, update := range plan.FilesToUpdate {
		templateName := strings.Split(update.File, "-")[0] // Extract template name
		if err := wg.generateWorkflow(templateName, templateData); err != nil {
			return fmt.Errorf("failed to update workflow %s: %w", update.File, err)
		}
	}

	// Remove obsolete files
	for _, filename := range plan.FilesToRemove {
		filepath := filepath.Join(wg.outputPath, filename)
		if err := os.Remove(filepath); err != nil {
			return fmt.Errorf("failed to remove workflow %s: %w", filename, err)
		}
	}

	return nil
}

// PreviewWorkflow generates a preview of workflows without writing files
func (wg *WorkflowGenerator) PreviewWorkflow() (*WorkflowPreview, error) {
	if err := wg.LoadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	preview := &WorkflowPreview{
		StackName: wg.stackName,
		Config:    wg.config,
		Workflows: []WorkflowInfo{},
	}

	templateData := wg.prepareTemplateData()

	for _, templateName := range wg.config.WorkflowGeneration.Templates {
		content, err := wg.generateWorkflowContent(templateName, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to generate content for template %s: %w", templateName, err)
		}

		workflow := WorkflowInfo{
			Name:        fmt.Sprintf("%s-%s", templateName, wg.stackName),
			FileName:    fmt.Sprintf("%s-%s.yml", templateName, wg.stackName),
			Description: fmt.Sprintf("Generated %s workflow for %s", templateName, wg.stackName),
			Content:     content,
			Triggers:    []string{"push", "pull_request"},
			Jobs: []JobInfo{{
				Name:        "deploy",
				Runner:      "ubuntu-latest",
				Environment: templateData.DefaultEnvironment,
				Steps: []StepInfo{
					{Name: "Deploy Stack", Action: "simple-container-com/api/.github/actions/deploy@main"},
				},
			}},
		}

		preview.Workflows = append(preview.Workflows, workflow)
	}

	return preview, nil
}

// generateWorkflowContent generates workflow content for a template
func (wg *WorkflowGenerator) generateWorkflowContent(templateName string, data *WorkflowTemplateData) (string, error) {
	tmpl, exists := wg.templates[templateName]
	if !exists {
		return "", fmt.Errorf("template %s not found", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

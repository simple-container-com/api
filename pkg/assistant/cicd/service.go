package cicd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/simple-container-com/api/pkg/clouds/github"
)

// Logger interface for debug logging during CI/CD operations
type Logger interface {
	Debug(ctx context.Context, format string, args ...interface{})
}

// Service provides core CI/CD functionality that can be shared across CLI, MCP, and chat interfaces
type Service struct{}

// NewService creates a new CI/CD service instance
func NewService() *Service {
	return &Service{}
}

// GenerateParams contains parameters for workflow generation
type GenerateParams struct {
	StackName   string
	Output      string
	ConfigFile  string
	Force       bool
	DryRun      bool
	Parent      bool
	Staging     bool
	SkipRefresh bool
}

// ValidateParams contains parameters for workflow validation
type ValidateParams struct {
	StackName    string
	ConfigFile   string
	WorkflowsDir string
	ShowDiff     bool
	Verbose      bool
	Parent       bool
	Staging      bool
}

// PreviewParams contains parameters for workflow preview
type PreviewParams struct {
	StackName   string
	ConfigFile  string
	ShowContent bool
	Parent      bool
	Staging     bool
}

// SyncParams contains parameters for workflow synchronization
type SyncParams struct {
	StackName   string
	ConfigFile  string
	DryRun      bool
	Force       bool
	Parent      bool
	Staging     bool
	SkipRefresh bool
}

// Result contains the result of a CI/CD operation
type Result struct {
	Success  bool
	Message  string
	Files    []string
	Warnings []string
	Data     map[string]interface{}
}

// GenerateWorkflows generates GitHub Actions workflows from server.yaml configuration
func (s *Service) GenerateWorkflows(params GenerateParams) (*Result, error) {
	return s.GenerateWorkflowsWithContext(context.Background(), nil, params)
}

func (s *Service) GenerateWorkflowsWithContext(ctx context.Context, logger Logger, params GenerateParams) (*Result, error) {
	// Process stack name and auto-detect config file
	stackName := processStackName(params.StackName)
	configFile, err := autoDetectConfigFileWithLogging(ctx, logger, params.ConfigFile, stackName)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Failed to resolve config file: %v", err),
		}, nil
	}

	// Load and validate server configuration
	serverDesc, err := validateAndLoadServerConfig(configFile)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Configuration error: %v", err),
		}, nil
	}

	// Create enhanced config
	enhancedConfig := createEnhancedConfig(serverDesc, stackName, params.Parent, params.Staging)

	// Set up output directory
	outputDir := params.Output
	if outputDir == "" {
		outputDir = ".github/workflows/"
	}
	if !filepath.IsAbs(outputDir) {
		abs, err := filepath.Abs(outputDir)
		if err != nil {
			return &Result{
				Success: false,
				Message: fmt.Sprintf("Failed to resolve output path: %v", err),
			}, nil
		}
		outputDir = abs
	}

	// Handle dry run
	if params.DryRun {
		return s.previewGeneration(enhancedConfig, stackName, outputDir)
	}

	// Check for existing files
	if !params.Force {
		existingFiles := s.checkExistingWorkflows(enhancedConfig, stackName, outputDir)
		if len(existingFiles) > 0 {
			return &Result{
				Success: false,
				Message: "Workflow files already exist. Use --force to overwrite.",
				Data: map[string]interface{}{
					"existing_files": existingFiles,
				},
			}, nil
		}
	}

	// Generate workflows
	generator := github.NewWorkflowGenerator(enhancedConfig, stackName, outputDir, params.SkipRefresh)
	if err := generator.GenerateWorkflows(); err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Failed to generate workflows: %v", err),
		}, nil
	}

	// Get required secrets for guidance
	requiredSecrets := getRequiredSecrets(enhancedConfig)

	result := &Result{
		Success: true,
		Message: fmt.Sprintf("🚀 CI/CD workflows generated successfully!\n\n📁 Output directory: %s", outputDir),
		Data: map[string]interface{}{
			"output_directory": outputDir,
			"stack_name":       stackName,
			"config_file":      configFile,
			"required_secrets": requiredSecrets,
		},
	}

	return result, nil
}

// ValidateWorkflows validates existing workflow files against server.yaml configuration
func (s *Service) ValidateWorkflows(params ValidateParams) (*Result, error) {
	return s.ValidateWorkflowsWithContext(context.Background(), nil, params)
}

func (s *Service) ValidateWorkflowsWithContext(ctx context.Context, logger Logger, params ValidateParams) (*Result, error) {
	// Process stack name and auto-detect config file
	stackName := processStackName(params.StackName)
	configFile, err := autoDetectConfigFileWithLogging(ctx, logger, params.ConfigFile, stackName)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Failed to resolve config file: %v", err),
		}, nil
	}

	// Load and validate server configuration
	serverDesc, err := validateAndLoadServerConfig(configFile)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Configuration error: %v", err),
		}, nil
	}

	// Create enhanced config
	enhancedConfig := createEnhancedConfig(serverDesc, stackName, params.Parent, params.Staging)

	// Set up workflows directory
	workflowsDir := params.WorkflowsDir
	if workflowsDir == "" {
		workflowsDir = ".github/workflows"
	}

	// Validate workflows directory exists
	if _, err := os.Stat(workflowsDir); os.IsNotExist(err) {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Workflows directory does not exist: %s", workflowsDir),
		}, nil
	}

	// Perform validation
	generator := github.NewWorkflowGenerator(enhancedConfig, stackName, workflowsDir, false)
	validationResults, err := generator.ValidateWorkflows()
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Validation failed: %v", err),
		}, nil
	}

	// Process validation results
	var message string
	var warnings []string
	allValid := validationResults.IsValid

	// Add valid files to warnings list
	for _, validFile := range validationResults.ValidFiles {
		warnings = append(warnings, fmt.Sprintf("✅ %s: Valid", validFile))
	}

	// Add missing files to warnings list
	for _, missingFile := range validationResults.MissingFiles {
		warnings = append(warnings, fmt.Sprintf("❌ %s: Missing", missingFile))
	}

	// Add outdated files to warnings list
	for _, outdatedFile := range validationResults.OutdatedFiles {
		warnings = append(warnings, fmt.Sprintf("⚠️ %s: Outdated", outdatedFile))
	}

	// Add invalid files to warnings list
	for invalidFile, issues := range validationResults.InvalidFiles {
		warnings = append(warnings, fmt.Sprintf("❌ %s: Invalid (%s)", invalidFile, issues[0]))
	}

	if allValid {
		message = "✅ All CI/CD workflows are valid and up-to-date"
	} else {
		message = "⚠️ Some CI/CD workflows need attention"
	}

	return &Result{
		Success:  allValid,
		Message:  message,
		Warnings: warnings,
		Data: map[string]interface{}{
			"validation_results": validationResults,
			"stack_name":         stackName,
			"config_file":        configFile,
			"workflows_dir":      workflowsDir,
		},
	}, nil
}

// PreviewWorkflows shows what workflows would be generated without creating files
func (s *Service) PreviewWorkflows(params PreviewParams) (*Result, error) {
	// Process stack name and auto-detect config file
	stackName := processStackName(params.StackName)
	configFile, err := autoDetectConfigFile(params.ConfigFile, stackName)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Failed to resolve config file: %v", err),
		}, nil
	}

	// Load and validate server configuration
	serverDesc, err := validateAndLoadServerConfig(configFile)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Configuration error: %v", err),
		}, nil
	}

	// Create enhanced config
	enhancedConfig := createEnhancedConfig(serverDesc, stackName, params.Parent, params.Staging)

	// Generate preview
	return s.previewGeneration(enhancedConfig, stackName, ".github/workflows/")
}

// SyncWorkflows synchronizes workflows to GitHub repository
func (s *Service) SyncWorkflows(params SyncParams) (*Result, error) {
	// Process stack name and auto-detect config file
	stackName := processStackName(params.StackName)
	configFile, err := autoDetectConfigFile(params.ConfigFile, stackName)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Failed to resolve config file: %v", err),
		}, nil
	}

	// Load and validate server configuration
	serverDesc, err := validateAndLoadServerConfig(configFile)
	if err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Configuration error: %v", err),
		}, nil
	}

	// Create enhanced config
	enhancedConfig := createEnhancedConfig(serverDesc, stackName, params.Parent, params.Staging)

	workflowsDir := ".github/workflows/"

	if params.DryRun {
		// Show what would be synced
		return s.previewGeneration(enhancedConfig, stackName, workflowsDir)
	}

	// Interactive mode - show differences and ask for confirmation
	if !params.Force {
		existingFiles := s.checkExistingWorkflows(enhancedConfig, stackName, workflowsDir)
		if len(existingFiles) > 0 {
			// Show preview of changes
			preview, err := s.previewGeneration(enhancedConfig, stackName, workflowsDir)
			if err != nil {
				return &Result{
					Success: false,
					Message: fmt.Sprintf("Failed to generate preview: %v", err),
				}, nil
			}

			// Return interactive prompt for confirmation
			return &Result{
				Success: false,
				Message: fmt.Sprintf("The following workflow files will be updated:\n%s\n\nProceed with sync? (Use --force to skip confirmation)", preview.Message),
				Data: map[string]interface{}{
					"existing_files":     existingFiles,
					"needs_confirmation": true,
					"preview":            preview.Message,
				},
			}, nil
		}
	}

	// Generate workflows (sync is essentially generate + git operations)
	generator := github.NewWorkflowGenerator(enhancedConfig, stackName, workflowsDir, params.SkipRefresh)
	if err := generator.GenerateWorkflows(); err != nil {
		return &Result{
			Success: false,
			Message: fmt.Sprintf("Failed to sync workflows: %v", err),
		}, nil
	}

	// TODO: Add git operations for actual sync to repository
	// For now, we just generate the files

	return &Result{
		Success: true,
		Message: fmt.Sprintf("🔄 CI/CD workflows synced successfully to %s", workflowsDir),
		Data: map[string]interface{}{
			"stack_name":    stackName,
			"config_file":   configFile,
			"workflows_dir": workflowsDir,
		},
	}, nil
}

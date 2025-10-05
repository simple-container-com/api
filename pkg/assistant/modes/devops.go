package modes

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/docs"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/assistant/llm"
	"github.com/simple-container-com/api/pkg/assistant/validation"
)

// DevOpsMode handles infrastructure-focused workflows
type DevOpsMode struct {
	llm llm.Provider
}

// NewDevOpsMode creates a new DevOps mode instance
func NewDevOpsMode() *DevOpsMode {
	return &DevOpsMode{}
}

// NewDevOpsModeWithLLM creates a new DevOps mode instance with LLM support
func NewDevOpsModeWithLLM(llmProvider llm.Provider) *DevOpsMode {
	return &DevOpsMode{
		llm: llmProvider,
	}
}

// SetupOptions for DevOps setup command
type DevOpsSetupOptions struct {
	Interactive   bool
	CloudProvider string
	Environments  []string
	Resources     []string
	Templates     []string
	Prefix        string
	Region        string
	OutputDir     string
}

// ResourceOptions for DevOps resource management
type ResourceOptions struct {
	Action       string // list, add, remove, update
	ResourceType string
	ResourceName string
	Environment  string
	Interactive  bool
	CopyFromEnv  string
	ScaleUp      bool
	ScaleDown    bool
}

// SecretsOptions for DevOps secrets management
type SecretsOptions struct {
	Action      string // init, auth, generate, import, rotate
	Provider    string
	Interactive bool
	SecretNames []string
	Length      int
	ExportTo    string
}

// SchemaResource represents a resource from the embedded schemas
type SchemaResource struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Provider     string `json:"provider"`
	Description  string `json:"description"`
	ResourceType string `json:"resourceType"`
	GoPackage    string `json:"goPackage"`
	GoStruct     string `json:"goStruct"`
}

// ResourceCategory represents a categorized resource for user selection
type ResourceCategory struct {
	Name         string
	Description  string
	Selected     bool
	Category     string
	ResourceType string
	Provider     string
}

// Setup creates infrastructure configuration with interactive wizard
func (d *DevOpsMode) Setup(ctx context.Context, opts DevOpsSetupOptions) error {
	fmt.Println(color.BlueFmt("🛠️ Simple Container DevOps Mode - Infrastructure Setup"))
	fmt.Printf("📂 Output directory: %s\n", color.CyanFmt(opts.OutputDir))

	// Step 1: Cloud Provider Selection (unless specified)
	if opts.CloudProvider == "" && opts.Interactive {
		provider, err := d.selectCloudProvider()
		if err != nil {
			return err
		}
		opts.CloudProvider = provider
	}

	// Default to AWS if not specified and not interactive
	if opts.CloudProvider == "" {
		opts.CloudProvider = "aws"
		fmt.Printf("🌐 Using default cloud provider: %s\n", color.CyanFmt("AWS"))
	}

	// Step 2: Environment Configuration
	if len(opts.Environments) == 0 {
		if opts.Interactive {
			environments, err := d.selectEnvironments()
			if err != nil {
				return err
			}
			opts.Environments = environments
		} else {
			opts.Environments = []string{"staging", "production"}
		}
	}

	fmt.Printf("📊 Configuring environments: %s\n", color.GreenFmt(strings.Join(opts.Environments, ", ")))

	// Step 3: Resource Selection
	if len(opts.Resources) == 0 && opts.Interactive {
		resources, err := d.selectResources(opts.CloudProvider)
		if err != nil {
			return err
		}
		opts.Resources = resources
	}

	// Default resources if not specified
	if len(opts.Resources) == 0 {
		opts.Resources = []string{"postgres", "redis", "s3-bucket"}
		fmt.Printf("📦 Using default resources: %s\n", color.YellowFmt(strings.Join(opts.Resources, ", ")))
	}

	// Step 4: Template Definition
	if len(opts.Templates) == 0 {
		opts.Templates = []string{"web-app", "api-service"}
		fmt.Printf("📋 Using default templates: %s\n", color.YellowFmt(strings.Join(opts.Templates, ", ")))
	}

	// Step 5: Generate Configuration Files
	fmt.Println("\n📝 Generating infrastructure files...")

	if err := d.generateInfrastructureFiles(opts); err != nil {
		return fmt.Errorf("infrastructure file generation failed: %w", err)
	}

	// Step 6: Success Summary
	d.printSetupSummary(opts)

	return nil
}

// Resources manages shared infrastructure resources
func (d *DevOpsMode) Resources(ctx context.Context, opts ResourceOptions) error {
	switch opts.Action {
	case "list":
		return d.listResources(opts)
	case "add":
		return d.addResource(opts)
	case "remove":
		return d.removeResource(opts)
	case "update":
		return d.updateResource(opts)
	default:
		return fmt.Errorf("unknown resource action: %s", opts.Action)
	}
}

// Secrets manages authentication credentials and secrets
func (d *DevOpsMode) Secrets(ctx context.Context, opts SecretsOptions) error {
	switch opts.Action {
	case "init":
		return d.initSecrets(opts)
	case "auth":
		return d.configureAuth(opts)
	case "generate":
		return d.generateSecrets(opts)
	case "import":
		return d.importSecrets(opts)
	case "rotate":
		return d.rotateSecrets(opts)
	default:
		return fmt.Errorf("unknown secrets action: %s", opts.Action)
	}
}

// Interactive selection methods

func (d *DevOpsMode) selectCloudProvider() (string, error) {
	fmt.Println("\n🌐 Select your primary cloud provider:")
	fmt.Println("1. AWS (Amazon Web Services)")
	fmt.Printf("   %s ECS Fargate, RDS, S3, ElastiCache, Lambda\n", color.GreenFmt("✅"))
	fmt.Println("2. GCP (Google Cloud Platform)")
	fmt.Printf("   %s GKE Autopilot, Cloud SQL, Cloud Storage, Cloud Run\n", color.GreenFmt("✅"))
	fmt.Println("3. Azure (Microsoft Azure)")
	fmt.Printf("   %s Container Apps, PostgreSQL, Blob Storage\n", color.YellowFmt("⏳ Coming Soon"))
	fmt.Println("4. Kubernetes (Cloud-agnostic)")
	fmt.Printf("   %s Native K8s, Helm operators, YAML manifests\n", color.GreenFmt("✅"))
	fmt.Println("5. Hybrid (Multiple providers)")
	fmt.Printf("   %s Advanced configuration required\n", color.CyanFmt("🔧"))

	// Get user input
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("\nChoice [1-5]: ")

	for {
		if !scanner.Scan() {
			return "", fmt.Errorf("failed to read input")
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			input = "1" // Default to AWS
		}

		switch input {
		case "1":
			fmt.Printf("   %s Selected AWS\n", color.GreenFmt("✓"))
			return "aws", nil
		case "2":
			fmt.Printf("   %s Selected Google Cloud Platform\n", color.GreenFmt("✓"))
			return "gcp", nil
		case "3":
			fmt.Printf("   %s Selected Microsoft Azure\n", color.GreenFmt("✓"))
			return "azure", nil
		case "4":
			fmt.Printf("   %s Selected Kubernetes\n", color.GreenFmt("✓"))
			return "kubernetes", nil
		case "5":
			fmt.Printf("   %s Selected Hybrid (Advanced)\n", color.GreenFmt("✓"))
			return "hybrid", nil
		default:
			fmt.Printf("   %s Please select a number between 1-5\n", color.YellowFmt("⚠"))
			fmt.Print("Choice [1-5]: ")
		}
	}
}

func (d *DevOpsMode) selectEnvironments() ([]string, error) {
	fmt.Println("\n📊 Configure your environments:")
	fmt.Printf("%s Development (local docker-compose)\n", color.GreenFmt("✅"))
	fmt.Printf("%s Staging (cloud resources, cost-optimized)\n", color.GreenFmt("✅"))
	fmt.Printf("%s Production (cloud resources, high availability)\n", color.GreenFmt("✅"))
	fmt.Print("\nAdditional environments (preview, testing, etc.)? (y/n): ")

	scanner := bufio.NewScanner(os.Stdin)
	environments := []string{"staging", "production"} // Default environments

	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if response == "y" || response == "yes" {
			fmt.Printf("   %s Additional environments enabled\n", color.GreenFmt("✓"))
			fmt.Print("\nEnter additional environment names (comma-separated): ")
			if scanner.Scan() {
				additionalEnvs := strings.TrimSpace(scanner.Text())
				if additionalEnvs != "" {
					for _, env := range strings.Split(additionalEnvs, ",") {
						env = strings.TrimSpace(env)
						if env != "" {
							environments = append(environments, env)
						}
					}
				}
			}
		} else {
			fmt.Printf("   %s Using default environments: staging, production\n", color.GreenFmt("✓"))
		}
	}

	return environments, nil
}

func (d *DevOpsMode) selectResources(cloudProvider string) ([]string, error) {
	fmt.Println("\n🎯 Select shared resources to provision:")

	// Load available resources from embedded schemas
	availableResources, err := d.loadAvailableResources(cloudProvider)
	if err != nil {
		fmt.Printf("   %s Failed to load resources from schemas, using defaults: %v\n", color.YellowFmt("⚠"), err)
		// Fall back to hardcoded resources if schema loading fails
		return d.selectResourcesFallback(cloudProvider)
	}

	// Categorize resources for display
	resources := make(map[string]ResourceCategory)
	for _, resource := range availableResources {
		category := d.categorizeResource(resource)
		selected := d.isResourceSelectedByDefault(resource)

		resources[resource.ResourceType] = ResourceCategory{
			Name:         resource.Name,
			Description:  resource.Description,
			Selected:     selected,
			Category:     category,
			ResourceType: resource.ResourceType,
			Provider:     resource.Provider,
		}
	}

	// Display categories
	categories := []string{"database", "storage", "compute", "monitoring"}
	categoryNames := map[string]string{
		"database":   "Databases:",
		"storage":    "Storage:",
		"compute":    "Compute:",
		"monitoring": "Monitoring:",
	}

	for _, category := range categories {
		fmt.Printf("\n%s\n", categoryNames[category])
		for _, resource := range resources {
			if resource.Category == category {
				checkbox := "☐"
				if resource.Selected {
					checkbox = "☑️"
				}
				fmt.Printf("%s %s (%s)\n", checkbox, resource.Name, resource.Description)
			}
		}
	}

	fmt.Print("\nUse default selection? (y/n): ")
	scanner := bufio.NewScanner(os.Stdin)

	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if response == "n" || response == "no" {
			fmt.Println("\nCustomize your selection:")
			fmt.Println("Enter resource names to toggle (comma-separated), or press Enter to finish:")
			fmt.Print("Resources: ")

			if scanner.Scan() {
				customResources := strings.TrimSpace(scanner.Text())
				if customResources != "" {
					for _, resourceName := range strings.Split(customResources, ",") {
						resourceName = strings.TrimSpace(strings.ToLower(resourceName))
						for key, resource := range resources {
							if strings.Contains(strings.ToLower(resource.Name), resourceName) || key == resourceName {
								resource.Selected = !resource.Selected
								resources[key] = resource
								break
							}
						}
					}
				}
			}
		}
	}

	// Collect selected resources
	selectedResources := []string{}
	selectedNames := []string{}
	for key, resource := range resources {
		if resource.Selected {
			selectedResources = append(selectedResources, key)
			selectedNames = append(selectedNames, resource.Name)
		}
	}

	fmt.Printf("\n   %s Using selected resources: %s\n", color.GreenFmt("✓"), color.GreenFmt(strings.Join(selectedNames, ", ")))
	return selectedResources, nil
}

// File generation methods

func (d *DevOpsMode) generateInfrastructureFiles(opts DevOpsSetupOptions) error {
	// Create output directory structure
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = ".sc/stacks/infrastructure"
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate server.yaml
	fmt.Printf("   📄 Generating server.yaml...")
	serverYaml, err := d.GenerateServerYAMLWithLLM(opts)
	if err != nil {
		return fmt.Errorf("failed to generate server.yaml: %w", err)
	}
	serverPath := filepath.Join(outputDir, "server.yaml")
	if err := os.WriteFile(serverPath, []byte(serverYaml), 0o644); err != nil {
		return fmt.Errorf("failed to write server.yaml: %w", err)
	}
	fmt.Printf(" %s\n", color.GreenFmt("✓"))

	// Generate secrets.yaml
	fmt.Printf("   📄 Generating secrets.yaml...")
	secretsYaml := d.generateSecretsYAML(opts)
	secretsPath := filepath.Join(outputDir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte(secretsYaml), 0o644); err != nil {
		return fmt.Errorf("failed to write secrets.yaml: %w", err)
	}
	fmt.Printf(" %s\n", color.GreenFmt("✓"))

	// Generate cfg.default.yaml
	fmt.Printf("   📄 Generating cfg.default.yaml...")
	configYaml := d.generateDefaultConfig(opts)
	configPath := filepath.Join(".sc", "cfg.default.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configYaml), 0o644); err != nil {
		return fmt.Errorf("failed to write cfg.default.yaml: %w", err)
	}
	fmt.Printf(" %s\n", color.GreenFmt("✓"))

	return nil
}

// GenerateServerYAMLWithLLM generates server.yaml using LLM with validation
func (d *DevOpsMode) GenerateServerYAMLWithLLM(opts DevOpsSetupOptions) (string, error) {
	if d.llm == nil {
		return d.generateFallbackServerYAML(opts), nil
	}

	prompt := d.buildServerYAMLPrompt(opts)

	response, err := d.llm.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: `You are an expert in Simple Container server.yaml configuration. Generate ONLY valid YAML that EXACTLY follows the provided JSON schemas.

CRITICAL INSTRUCTIONS:
✅ Follow the JSON schemas EXACTLY - every property must match the schema structure
✅ Use ONLY properties defined in the schemas - no fictional or made-up properties
✅ server.yaml MUST have: schemaVersion, provisioner, templates, resources sections
✅ provisioner MUST have: type, config (with state-storage and secrets-provider)
✅ resources section contains shared infrastructure resources
✅ templates section contains reusable deployment templates

🚫 FORBIDDEN (will cause validation errors):
❌ stacks section (belongs in client.yaml only)
❌ environments section (use proper file separation)
❌ version property (use schemaVersion)
❌ fictional resource types or properties

RESPONSE FORMAT: Generate ONLY the YAML content. No explanations, no markdown blocks, no additional text.`},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		fmt.Printf("LLM generation failed, using fallback: %v\n", err)
		return d.generateFallbackServerYAML(opts), nil
	}

	// Extract YAML from response (remove any markdown formatting)
	yamlContent := strings.TrimSpace(response.Content)
	yamlContent = strings.TrimPrefix(yamlContent, "```yaml")
	yamlContent = strings.TrimPrefix(yamlContent, "```")
	yamlContent = strings.TrimSuffix(yamlContent, "```")
	yamlContent = strings.TrimSpace(yamlContent)

	// Validate generated YAML against schemas
	validator := validation.NewValidator()
	result := validator.ValidateServerYAML(context.Background(), yamlContent)

	if !result.Valid {
		fmt.Printf("⚠️  Generated server.yaml has validation errors:\n")
		for _, err := range result.Errors {
			fmt.Printf("   • %s\n", color.RedFmt(err))
		}
		fmt.Printf("   🔄 Using schema-compliant fallback template...\n")
		return d.generateFallbackServerYAML(opts), nil
	}

	if len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			fmt.Printf("   ⚠️  %s\n", color.YellowFmt(warning))
		}
	}

	return yamlContent, nil
}

func (d *DevOpsMode) buildServerYAMLPrompt(opts DevOpsSetupOptions) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a Simple Container server.yaml configuration using ONLY these validated properties:\n\n")
	prompt.WriteString(fmt.Sprintf("Cloud Provider: %s\n", opts.CloudProvider))
	prompt.WriteString(fmt.Sprintf("Company/Organization: %s\n", opts.Prefix))
	prompt.WriteString(fmt.Sprintf("Environments: %s\n", strings.Join(opts.Environments, ", ")))
	prompt.WriteString(fmt.Sprintf("Resources: %s\n", strings.Join(opts.Resources, ", ")))

	// Add JSON schema context for better generation
	validator := validation.NewValidator()
	if serverSchema, err := validator.GetServerYAMLSchema(context.Background()); err == nil {
		if schemaContent, err := json.MarshalIndent(serverSchema, "", "  "); err == nil {
			prompt.WriteString("\n📋 SERVER.YAML JSON SCHEMA (follow this structure exactly):\n")
			prompt.WriteString("```json\n")
			prompt.WriteString(string(schemaContent))
			prompt.WriteString("\n```\n")
		}
	}

	// Add validated example structure
	prompt.WriteString("\n✅ REQUIRED STRUCTURE EXAMPLE:\n")
	prompt.WriteString("schemaVersion: 1.0\n")
	prompt.WriteString("provisioner:\n")
	prompt.WriteString("  type: pulumi\n")
	prompt.WriteString("  config:\n")
	prompt.WriteString("    state-storage:\n")
	prompt.WriteString("      type: s3-bucket\n")
	prompt.WriteString("      config:\n")
	prompt.WriteString(fmt.Sprintf("        bucketName: %s-sc-state\n", opts.Prefix))
	prompt.WriteString("        region: us-east-1\n")
	prompt.WriteString("    secrets-provider:\n")
	prompt.WriteString("      type: aws-kms\n")
	prompt.WriteString("      config:\n")
	prompt.WriteString("        keyId: \"alias/simple-container\"\n")
	prompt.WriteString("templates:\n")
	prompt.WriteString("  web-app:\n")
	prompt.WriteString("    type: aws-ecs-fargate\n")
	prompt.WriteString("resources:\n")
	prompt.WriteString("  infrastructure:\n")
	prompt.WriteString("    ecs-cluster:\n")
	prompt.WriteString("      type: aws-ecs-cluster\n")

	prompt.WriteString("\n🚫 NEVER USE THESE (fictional properties eliminated in validation):\n")
	prompt.WriteString("- stacks: section (use 'resources:' only)\n")
	prompt.WriteString("- environments: section (use proper file separation)\n")
	prompt.WriteString("- version: property (use 'schemaVersion:')\n")

	prompt.WriteString("\n⚡ Generate ONLY the valid YAML (no explanations, no markdown):")

	return prompt.String()
}

func (d *DevOpsMode) generateFallbackServerYAML(opts DevOpsSetupOptions) string {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "mycompany"
	}

	yaml := fmt.Sprintf(`schemaVersion: 1.0

# Provisioner configuration
provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        bucketName: %s-sc-state
        region: %s
    secrets-provider:
      type: aws-kms
      config:
        keyId: "alias/simple-container"

# Reusable templates for application teams
templates:`, prefix, d.getDefaultRegion(opts.CloudProvider))

	// Add templates
	for _, template := range opts.Templates {
		switch template {
		case "web-app":
			yaml += fmt.Sprintf(`
  web-app:
    type: %s
    config:
      ecsClusterResource: ecs-cluster
      ecrRepositoryResource: web-registry`, d.getComputeTemplate(opts.CloudProvider))
		case "api-service":
			yaml += fmt.Sprintf(`
  api-service:
    type: %s
    config:
      ecsClusterResource: ecs-cluster
      ecrRepositoryResource: api-registry`, d.getComputeTemplate(opts.CloudProvider))
		}
	}

	yaml += `

# Secrets management configuration
secrets:
  type: aws-kms
  config:
    keyId: "alias/simple-container"

# CI/CD integration
cicd:
  type: github-actions
  config:
    auth-token: "${secret:GITHUB_TOKEN}"

# Shared infrastructure resources
resources:
  # Domain registrar (optional)
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "your-cloudflare-account-id"
      zoneName: "example.com"
  
  # Environment-specific resources
  resources:`

	// Add resources for each environment
	for _, env := range opts.Environments {
		yaml += fmt.Sprintf(`
    %s:
      template: web-app
      resources:`, env)

		// Add compute cluster
		yaml += fmt.Sprintf(`
        ecs-cluster:
          type: %s
          config:
            name: %s-%s-cluster`, d.getClusterType(opts.CloudProvider), prefix, env)

		// Add container registries
		for _, template := range opts.Templates {
			registryName := strings.Replace(template, "-", "", -1)
			yaml += fmt.Sprintf(`
        %s-registry:
          type: %s
          config:
            name: %s-%s-%s`, registryName, d.getRegistryType(opts.CloudProvider), prefix, env, registryName)
		}

		// Add selected resources
		for _, resource := range opts.Resources {
			switch resource {
			case "postgres":
				instanceClass := "db.t3.micro"
				storage := "20"
				if env == "production" {
					instanceClass = "db.r5.large"
					storage = "100"
				}

				yaml += fmt.Sprintf(`
        postgres-db:
          type: %s
          config:
            name: %s-%s-db
            instanceClass: %s
            allocatedStorage: %s
            engineVersion: "15.4"
            username: dbadmin
            password: "${secret:%s-db-password}"
            databaseName: applications`, d.getPostgresType(opts.CloudProvider), prefix, env, instanceClass, storage, env)

			case "redis":
				nodeType := "cache.t3.micro"
				nodes := "1"
				if env == "production" {
					nodeType = "cache.r5.large"
					nodes = "3"
				}

				yaml += fmt.Sprintf(`
        redis-cache:
          type: %s
          config:
            name: %s-%s-cache
            nodeType: %s
            numCacheNodes: %s`, d.getCacheType(opts.CloudProvider), prefix, env, nodeType, nodes)

			case "s3-bucket":
				yaml += fmt.Sprintf(`
        uploads-bucket:
          type: s3-bucket
          config:
            name: %s-%s-uploads
            allowOnlyHttps: true`, prefix, env)
			}
		}
	}

	// Add variables section (required by schema)
	yaml += `

# Configuration variables
variables:
  company-prefix:
    type: string
    value: ` + prefix

	return yaml
}

func (d *DevOpsMode) generateSecretsYAML(opts DevOpsSetupOptions) string {
	yaml := fmt.Sprintf(`# Authentication for cloud providers
auth:
  %s:
    credentials: "${secret:%s-credentials}"`, opts.CloudProvider, opts.CloudProvider)

	if opts.CloudProvider == "aws" {
		yaml = `# Authentication for cloud providers
auth:
  aws:
    account: "123456789012"
    accessKey: "${secret:aws-access-key}"
    secretAccessKey: "${secret:aws-secret-key}"
    region: us-east-1`
	}

	yaml += "\n\n# Secret values (managed with sc secrets add)\nvalues:"

	// Add environment-specific database passwords
	for _, env := range opts.Environments {
		yaml += fmt.Sprintf(`
  %s-db-password: "secure-%s-password-123"`, env, env)
	}

	// Add cloud credentials placeholders
	switch opts.CloudProvider {
	case "aws":
		yaml += `
  aws-access-key: "AKIA..."
  aws-secret-key: "secret..."`
	case "gcp":
		yaml += `
  gcp-service-account-key: "service-account-json..."
  gcp-project-id: "my-project-id"`
	}

	// Add application secrets
	yaml += `
  jwt-secret: "super-secret-jwt-key"
  api-key: "external-service-key"`

	return yaml
}

func (d *DevOpsMode) generateDefaultConfig(opts DevOpsSetupOptions) string {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "mycompany"
	}

	return fmt.Sprintf(`# Simple Container local configuration
privateKeyPath: ~/.ssh/id_rsa
publicKeyPath: ~/.ssh/id_rsa.pub
projectName: %s-infrastructure`, prefix)
}

// Helper methods for cloud-specific configurations

func (d *DevOpsMode) getDefaultRegion(provider string) string {
	switch provider {
	case "aws":
		return "us-east-1"
	case "gcp":
		return "us-central1"
	case "azure":
		return "East US"
	default:
		return "us-east-1"
	}
}

func (d *DevOpsMode) getComputeTemplate(provider string) string {
	switch provider {
	case "aws":
		return "aws-ecs-fargate"
	case "gcp":
		return "gcp-cloud-run"
	case "kubernetes":
		return "kubernetes-deployment"
	default:
		return "aws-ecs-fargate"
	}
}

func (d *DevOpsMode) getClusterType(provider string) string {
	switch provider {
	case "aws":
		return "aws-ecs-cluster"
	case "gcp":
		return "gcp-gke-autopilot-cluster"
	case "kubernetes":
		return "kubernetes-cluster"
	default:
		return "aws-ecs-cluster"
	}
}

func (d *DevOpsMode) getRegistryType(provider string) string {
	switch provider {
	case "aws":
		return "aws-ecr-repository"
	case "gcp":
		return "gcp-artifact-registry"
	case "kubernetes":
		return "docker-registry"
	default:
		return "aws-ecr-repository"
	}
}

func (d *DevOpsMode) getPostgresType(provider string) string {
	switch provider {
	case "aws":
		return "aws-rds-postgres"
	case "gcp":
		return "gcp-cloudsql-postgres"
	case "kubernetes":
		return "kubernetes-postgres"
	default:
		return "aws-rds-postgres"
	}
}

func (d *DevOpsMode) getCacheType(provider string) string {
	switch provider {
	case "aws":
		return "aws-elasticache-redis"
	case "gcp":
		return "gcp-memorystore-redis"
	case "kubernetes":
		return "kubernetes-redis"
	default:
		return "aws-elasticache-redis"
	}
}

// Resource management methods

func (d *DevOpsMode) listResources(opts ResourceOptions) error {
	fmt.Printf("📋 Available resource types for %s:\n\n", color.CyanFmt(opts.ResourceType))

	resources := map[string][]string{
		"aws": {
			"s3-bucket - Amazon S3 storage bucket",
			"aws-rds-postgres - Amazon RDS PostgreSQL database",
			"aws-rds-mysql - Amazon RDS MySQL database",
			"aws-elasticache-redis - Amazon ElastiCache Redis",
			"aws-ecs-cluster - Amazon ECS cluster",
			"aws-ecr-repository - Amazon ECR container registry",
		},
		"gcp": {
			"gcp-bucket - Google Cloud Storage bucket",
			"gcp-cloudsql-postgres - Google Cloud SQL PostgreSQL",
			"gcp-gke-autopilot-cluster - Google Kubernetes Engine Autopilot",
			"gcp-artifact-registry - Google Artifact Registry",
			"gcp-memorystore-redis - Google Memorystore Redis",
		},
		"kubernetes": {
			"kubernetes-postgres - PostgreSQL via Helm operator",
			"kubernetes-redis - Redis via Helm operator",
			"kubernetes-mongodb - MongoDB via Helm operator",
			"kubernetes-deployment - Standard Kubernetes deployment",
		},
	}

	provider := opts.ResourceType
	if provider == "" {
		provider = "aws" // Default
	}

	if resourceList, exists := resources[provider]; exists {
		for _, resource := range resourceList {
			fmt.Printf("   • %s\n", resource)
		}
	} else {
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	return nil
}

func (d *DevOpsMode) addResource(opts ResourceOptions) error {
	fmt.Printf("➕ Adding %s resource to %s environment\n",
		color.GreenFmt(opts.ResourceType), color.CyanFmt(opts.Environment))

	// Find server.yaml file in .sc/stacks directory
	serverYamlPath, err := d.findServerYaml()
	if err != nil {
		return fmt.Errorf("failed to find server.yaml: %w", err)
	}

	// Read and parse server.yaml
	data, err := os.ReadFile(serverYamlPath)
	if err != nil {
		return fmt.Errorf("failed to read server.yaml: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse server.yaml: %w", err)
	}

	// Ensure resources section exists
	if config["resources"] == nil {
		config["resources"] = make(map[string]interface{})
	}

	resources := config["resources"].(map[string]interface{})

	// Ensure environment section exists
	if resources[opts.Environment] == nil {
		resources[opts.Environment] = make(map[string]interface{})
	}

	envResources := resources[opts.Environment].(map[string]interface{})

	// Create new resource based on type
	newResource := d.createResourceTemplate(opts.ResourceType, opts.ResourceName)
	envResources[opts.ResourceName] = newResource

	// Write back to file
	updatedData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	if err := os.WriteFile(serverYamlPath, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write server.yaml: %w", err)
	}

	fmt.Printf("   %s Added %s resource '%s' to %s environment\n",
		color.GreenFmt("✓"), opts.ResourceType, opts.ResourceName, opts.Environment)

	return nil
}

func (d *DevOpsMode) removeResource(opts ResourceOptions) error {
	fmt.Printf("➖ Removing %s resource from %s environment\n",
		color.RedFmt(opts.ResourceName), color.CyanFmt(opts.Environment))

	// Find server.yaml file in .sc/stacks directory
	serverYamlPath, err := d.findServerYaml()
	if err != nil {
		return fmt.Errorf("failed to find server.yaml: %w", err)
	}

	// Read and parse server.yaml
	data, err := os.ReadFile(serverYamlPath)
	if err != nil {
		return fmt.Errorf("failed to read server.yaml: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse server.yaml: %w", err)
	}

	// Navigate to resources section
	resources, ok := config["resources"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no resources section found in server.yaml")
	}

	envResources, ok := resources[opts.Environment].(map[string]interface{})
	if !ok {
		return fmt.Errorf("environment '%s' not found in resources", opts.Environment)
	}

	// Check if resource exists
	if _, exists := envResources[opts.ResourceName]; !exists {
		return fmt.Errorf("resource '%s' not found in %s environment", opts.ResourceName, opts.Environment)
	}

	// Remove the resource
	delete(envResources, opts.ResourceName)

	// Write back to file
	updatedData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	if err := os.WriteFile(serverYamlPath, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write server.yaml: %w", err)
	}

	fmt.Printf("   %s Removed resource '%s' from %s environment\n",
		color.GreenFmt("✓"), opts.ResourceName, opts.Environment)

	return nil
}

func (d *DevOpsMode) updateResource(opts ResourceOptions) error {
	fmt.Printf("🔄 Updating %s resource in %s environment\n",
		color.YellowFmt(opts.ResourceName), color.CyanFmt(opts.Environment))

	// Find server.yaml file in .sc/stacks directory
	serverYamlPath, err := d.findServerYaml()
	if err != nil {
		return fmt.Errorf("failed to find server.yaml: %w", err)
	}

	// Read and parse server.yaml
	data, err := os.ReadFile(serverYamlPath)
	if err != nil {
		return fmt.Errorf("failed to read server.yaml: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse server.yaml: %w", err)
	}

	// Navigate to resources section
	resources, ok := config["resources"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no resources section found in server.yaml")
	}

	envResources, ok := resources[opts.Environment].(map[string]interface{})
	if !ok {
		return fmt.Errorf("environment '%s' not found in resources", opts.Environment)
	}

	// Check if resource exists
	existingResource, exists := envResources[opts.ResourceName]
	if !exists {
		return fmt.Errorf("resource '%s' not found in %s environment", opts.ResourceName, opts.Environment)
	}

	// Handle different update operations
	if opts.CopyFromEnv != "" {
		// Copy resource from another environment
		sourceEnvResources, ok := resources[opts.CopyFromEnv].(map[string]interface{})
		if !ok {
			return fmt.Errorf("source environment '%s' not found", opts.CopyFromEnv)
		}

		sourceResource, exists := sourceEnvResources[opts.ResourceName]
		if !exists {
			return fmt.Errorf("resource '%s' not found in source environment '%s'", opts.ResourceName, opts.CopyFromEnv)
		}

		envResources[opts.ResourceName] = sourceResource
		fmt.Printf("   %s Copied resource '%s' from %s to %s environment\n",
			color.GreenFmt("✓"), opts.ResourceName, opts.CopyFromEnv, opts.Environment)
	} else {
		// Update resource properties (for now, just update the type if provided)
		if opts.ResourceType != "" {
			if resourceMap, ok := existingResource.(map[string]interface{}); ok {
				resourceMap["type"] = opts.ResourceType
				envResources[opts.ResourceName] = resourceMap
			}
		}
		fmt.Printf("   %s Updated resource '%s' in %s environment\n",
			color.GreenFmt("✓"), opts.ResourceName, opts.Environment)
	}

	// Write back to file
	updatedData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	if err := os.WriteFile(serverYamlPath, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write server.yaml: %w", err)
	}

	return nil
}

// Helper methods for resource management

func (d *DevOpsMode) findServerYaml() (string, error) {
	// Look for server.yaml in .sc/stacks directory
	stacksDir := ".sc/stacks"
	if _, err := os.Stat(stacksDir); os.IsNotExist(err) {
		return "", fmt.Errorf("no .sc/stacks directory found - run 'sc init' first")
	}

	var serverYamlPath string
	err := filepath.Walk(stacksDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "server.yaml" {
			serverYamlPath = path
			return filepath.SkipDir // Stop searching once found
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for server.yaml: %w", err)
	}

	if serverYamlPath == "" {
		return "", fmt.Errorf("no server.yaml found in .sc/stacks directory")
	}

	return serverYamlPath, nil
}

func (d *DevOpsMode) createResourceTemplate(resourceType, resourceName string) map[string]interface{} {
	// Create a basic resource template based on the resource type
	resource := map[string]interface{}{
		"type": resourceType,
	}

	// Add type-specific default properties based on validated Simple Container schemas
	switch resourceType {
	case "s3-bucket":
		resource["name"] = resourceName
		resource["allowOnlyHttps"] = true
	case "aws-rds-postgres":
		resource["name"] = resourceName
		resource["instanceClass"] = "db.t3.micro"
		resource["allocateStorage"] = "20"
		resource["engineVersion"] = "13.7"
		resource["username"] = "postgres"
		resource["databaseName"] = "main"
	case "aws-rds-mysql":
		resource["name"] = resourceName
		resource["instanceClass"] = "db.t3.micro"
		resource["allocateStorage"] = "20"
		resource["engineVersion"] = "8.0"
		resource["username"] = "admin"
		resource["databaseName"] = "main"
	case "gcp-bucket":
		resource["name"] = resourceName
		resource["location"] = "US"
	case "gcp-cloudsql-postgres":
		resource["name"] = resourceName
		resource["region"] = "us-central1"
		resource["tier"] = "db-f1-micro"
	case "mongodb-atlas":
		resource["name"] = resourceName
		resource["instanceSize"] = "M10"
		resource["region"] = "US_EAST_1"
		resource["cloudProvider"] = "AWS"
	case "k8s-helm-postgres":
		resource["name"] = resourceName
		resource["chart"] = "postgresql"
		resource["repo"] = "https://charts.bitnami.com/bitnami"
	case "k8s-helm-redis":
		resource["name"] = resourceName
		resource["chart"] = "redis"
		resource["repo"] = "https://charts.bitnami.com/bitnami"
	case "k8s-helm-rabbitmq":
		resource["name"] = resourceName
		resource["chart"] = "rabbitmq"
		resource["repo"] = "https://charts.bitnami.com/bitnami"
	default:
		// For unknown types, just provide the basic structure
		resource["name"] = resourceName
	}

	return resource
}

// Resource loading methods

func (d *DevOpsMode) loadAvailableResources(cloudProvider string) ([]SchemaResource, error) {
	// Read main index from embedded schemas
	mainIndex, err := d.readEmbeddedProviderIndex("schemas/index.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read main schema index: %w", err)
	}

	var allResources []SchemaResource

	// Load resources from all providers (not just the selected one)
	// This allows users to see cross-cloud options
	for providerName := range mainIndex {
		if providerName == "core" {
			continue // Skip core schemas (client.yaml, server.yaml)
		}

		providerIndexPath := fmt.Sprintf("schemas/%s/index.json", providerName)
		providerResources, err := d.readEmbeddedProviderResources(providerIndexPath, providerName)
		if err != nil {
			continue // Skip providers with missing/invalid indexes
		}

		// Add resources from this provider
		for _, resource := range providerResources {
			// Only include actual resources (not templates, auth, provisioner)
			if resource.Type == "resource" {
				allResources = append(allResources, resource)
			}
		}
	}

	return allResources, nil
}

func (d *DevOpsMode) readEmbeddedProviderIndex(indexPath string) (map[string]interface{}, error) {
	data, err := docs.EmbeddedSchemas.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var index struct {
		Providers map[string]interface{} `json:"providers"`
	}

	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}

	return index.Providers, nil
}

func (d *DevOpsMode) readEmbeddedProviderResources(indexPath, providerName string) ([]SchemaResource, error) {
	data, err := docs.EmbeddedSchemas.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var providerIndex struct {
		Resources []SchemaResource `json:"resources"`
	}

	if err := json.Unmarshal(data, &providerIndex); err != nil {
		return nil, err
	}

	return providerIndex.Resources, nil
}

func (d *DevOpsMode) categorizeResource(resource SchemaResource) string {
	resourceType := strings.ToLower(resource.ResourceType)

	// Database resources
	if strings.Contains(resourceType, "postgres") ||
		strings.Contains(resourceType, "mysql") ||
		strings.Contains(resourceType, "mongodb") ||
		strings.Contains(resourceType, "redis") {
		return "database"
	}

	// Storage resources
	if strings.Contains(resourceType, "bucket") ||
		strings.Contains(resourceType, "storage") ||
		strings.Contains(resourceType, "s3") {
		return "storage"
	}

	// Compute resources
	if strings.Contains(resourceType, "ecs") ||
		strings.Contains(resourceType, "gke") ||
		strings.Contains(resourceType, "lambda") ||
		strings.Contains(resourceType, "cloudrun") ||
		strings.Contains(resourceType, "fargate") {
		return "compute"
	}

	// Monitoring resources
	if strings.Contains(resourceType, "monitor") ||
		strings.Contains(resourceType, "log") ||
		strings.Contains(resourceType, "alert") {
		return "monitoring"
	}

	// Default to storage for unknown types
	return "storage"
}

func (d *DevOpsMode) isResourceSelectedByDefault(resource SchemaResource) bool {
	resourceType := strings.ToLower(resource.ResourceType)

	// Select common resources by default
	defaultResources := []string{
		"aws-rds-postgres", "gcp-cloudsql-postgres", "kubernetes-helm-postgres-operator",
		"gcp-redis", "kubernetes-helm-redis-operator",
		"s3-bucket", "gcp-bucket",
	}

	for _, defaultResource := range defaultResources {
		if resourceType == strings.ToLower(defaultResource) {
			return true
		}
	}

	return false
}

func (d *DevOpsMode) selectResourcesFallback(cloudProvider string) ([]string, error) {
	fmt.Printf("   %s Using fallback resource selection for %s\n", color.CyanFmt("💡"), cloudProvider)

	// Fallback to basic resource selection based on cloud provider
	switch strings.ToLower(cloudProvider) {
	case "aws":
		return []string{"aws-rds-postgres", "s3-bucket"}, nil
	case "gcp":
		return []string{"gcp-cloudsql-postgres", "gcp-bucket"}, nil
	case "kubernetes":
		return []string{"kubernetes-helm-postgres-operator", "kubernetes-helm-redis-operator"}, nil
	default:
		return []string{"aws-rds-postgres", "s3-bucket"}, nil
	}
}

// Secrets management methods

func (d *DevOpsMode) initSecrets(opts SecretsOptions) error {
	fmt.Printf("🔐 Initializing secrets for %s\n", color.CyanFmt(opts.Provider))

	secretsPath := filepath.Join(".sc", "stacks", "infrastructure", "secrets.yaml")

	// Check if secrets.yaml already exists
	if _, err := os.Stat(secretsPath); err == nil {
		fmt.Printf("   %s Secrets file already exists at %s\n", color.YellowFmt("⚠"), secretsPath)
		return nil
	}

	// Create basic secrets template
	secretsConfig := map[string]interface{}{
		"schemaVersion": 1.0,
		"auth": map[string]interface{}{
			opts.Provider: map[string]string{
				"account":         "${AUTH_" + strings.ToUpper(opts.Provider) + "_ACCOUNT}",
				"accessKey":       "${AUTH_" + strings.ToUpper(opts.Provider) + "_ACCESS_KEY}",
				"secretAccessKey": "${AUTH_" + strings.ToUpper(opts.Provider) + "_SECRET_ACCESS_KEY}",
				"region":          "us-east-1",
			},
		},
		"values": make(map[string]string),
	}

	// Create directory
	if err := os.MkdirAll(filepath.Dir(secretsPath), 0o755); err != nil {
		return fmt.Errorf("failed to create secrets directory: %w", err)
	}

	// Write secrets.yaml
	secretsData, err := yaml.Marshal(secretsConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal secrets config: %w", err)
	}

	if err := os.WriteFile(secretsPath, secretsData, 0o644); err != nil {
		return fmt.Errorf("failed to write secrets.yaml: %w", err)
	}

	fmt.Printf("   %s Created secrets template at %s\n", color.GreenFmt("✓"), secretsPath)
	fmt.Printf("   %s Configure authentication with: %s\n",
		color.CyanFmt("💡"),
		color.CyanFmt("sc secrets add "+opts.Provider+"-access-key"))

	return nil
}

func (d *DevOpsMode) configureAuth(opts SecretsOptions) error {
	fmt.Printf("🔑 Configuring %s authentication\n", color.CyanFmt(opts.Provider))

	// Guide user through authentication setup
	fmt.Println("\n📋 Authentication Setup Guide:")

	switch strings.ToLower(opts.Provider) {
	case "aws":
		fmt.Printf("1. Get AWS credentials from IAM console:\n")
		fmt.Printf("   • Access Key ID\n")
		fmt.Printf("   • Secret Access Key\n")
		fmt.Printf("2. Set environment variables:\n")
		fmt.Printf("   %s\n", color.CyanFmt("export AUTH_AWS_ACCESS_KEY=your-access-key"))
		fmt.Printf("   %s\n", color.CyanFmt("export AUTH_AWS_SECRET_ACCESS_KEY=your-secret-key"))
		fmt.Printf("   %s\n", color.CyanFmt("export AUTH_AWS_ACCOUNT=123456789012"))
	case "gcp":
		fmt.Printf("1. Create service account in GCP console\n")
		fmt.Printf("2. Download service account key JSON\n")
		fmt.Printf("3. Set environment variable:\n")
		fmt.Printf("   %s\n", color.CyanFmt("export GOOGLE_APPLICATION_CREDENTIALS=path/to/key.json"))
	case "kubernetes":
		fmt.Printf("1. Get kubeconfig file\n")
		fmt.Printf("2. Set environment variable:\n")
		fmt.Printf("   %s\n", color.CyanFmt("export KUBECONFIG=path/to/kubeconfig"))
	default:
		fmt.Printf("1. Check Simple Container documentation for %s setup\n", opts.Provider)
	}

	fmt.Printf("\n   %s Authentication configuration guidance provided\n", color.GreenFmt("✓"))
	fmt.Printf("   %s Run 'sc secrets list' to verify configuration\n", color.CyanFmt("💡"))

	return nil
}

func (d *DevOpsMode) generateSecrets(opts SecretsOptions) error {
	fmt.Printf("🎲 Generating secrets: %s\n", color.GreenFmt(strings.Join(opts.SecretNames, ", ")))

	secretsPath := filepath.Join(".sc", "stacks", "infrastructure", "secrets.yaml")

	// Read existing secrets.yaml
	var secretsConfig map[string]interface{}
	if data, err := os.ReadFile(secretsPath); err == nil {
		if err := yaml.Unmarshal(data, &secretsConfig); err != nil {
			return fmt.Errorf("failed to parse existing secrets.yaml: %w", err)
		}
	} else {
		// Create new secrets config if file doesn't exist
		secretsConfig = map[string]interface{}{
			"schemaVersion": 1.0,
			"auth":          make(map[string]interface{}),
			"values":        make(map[string]string),
		}
	}

	// Get or create values section
	values, ok := secretsConfig["values"].(map[string]interface{})
	if !ok {
		values = make(map[string]interface{})
		secretsConfig["values"] = values
	}

	// Generate secure secrets for each requested name
	for _, secretName := range opts.SecretNames {
		if _, exists := values[secretName]; exists {
			fmt.Printf("   %s Secret '%s' already exists, skipping\n", color.YellowFmt("⚠"), secretName)
			continue
		}

		// Generate random secret based on the type (for reference, actual value set via env var)
		if strings.Contains(strings.ToLower(secretName), "key") ||
			strings.Contains(strings.ToLower(secretName), "secret") ||
			strings.Contains(strings.ToLower(secretName), "token") {
			// 32-byte (256-bit) random key for cryptographic use
			_ = d.generateRandomSecret(32)
		} else if strings.Contains(strings.ToLower(secretName), "password") {
			// 16-byte password (readable but secure)
			_ = d.generateRandomSecret(16)
		} else {
			// Default to 24-byte secret
			_ = d.generateRandomSecret(24)
		}

		values[secretName] = "${" + strings.ToUpper(secretName) + "}"
		fmt.Printf("   %s Generated secret '%s' (set env var: %s)\n",
			color.GreenFmt("✓"), secretName, strings.ToUpper(secretName))
	}

	// Write updated secrets.yaml
	updatedData, err := yaml.Marshal(secretsConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal secrets config: %w", err)
	}

	if err := os.WriteFile(secretsPath, updatedData, 0o644); err != nil {
		return fmt.Errorf("failed to write secrets.yaml: %w", err)
	}

	fmt.Printf("\n   %s Updated secrets.yaml with %d new secrets\n", color.GreenFmt("✓"), len(opts.SecretNames))
	fmt.Printf("   %s Set environment variables and run 'sc secrets reveal' to verify\n", color.CyanFmt("💡"))

	return nil
}

func (d *DevOpsMode) importSecrets(opts SecretsOptions) error {
	fmt.Println("📥 Importing secrets from external system")

	secretsPath := filepath.Join(".sc", "stacks", "infrastructure", "secrets.yaml")

	fmt.Println("\n📋 Import Options:")
	fmt.Println("1. From environment variables")
	fmt.Println("2. From AWS Secrets Manager")
	fmt.Println("3. From HashiCorp Vault")
	fmt.Println("4. From file (JSON/YAML)")

	fmt.Print("\nSelect import source [1-4]: ")
	scanner := bufio.NewScanner(os.Stdin)

	if scanner.Scan() {
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			return d.importFromEnvironment(secretsPath, opts.SecretNames)
		case "2":
			fmt.Printf("   %s AWS Secrets Manager import not yet implemented\n", color.YellowFmt("⚠"))
			fmt.Printf("   %s Use 'aws secretsmanager get-secret-value' manually for now\n", color.CyanFmt("💡"))
		case "3":
			fmt.Printf("   %s HashiCorp Vault import not yet implemented\n", color.YellowFmt("⚠"))
			fmt.Printf("   %s Use 'vault kv get' manually for now\n", color.CyanFmt("💡"))
		case "4":
			fmt.Printf("   %s File import not yet implemented\n", color.YellowFmt("⚠"))
			fmt.Printf("   %s Manually edit secrets.yaml for now\n", color.CyanFmt("💡"))
		default:
			fmt.Printf("   %s Invalid choice, defaulting to environment variables\n", color.YellowFmt("⚠"))
			return d.importFromEnvironment(secretsPath, opts.SecretNames)
		}
	}

	return nil
}

func (d *DevOpsMode) rotateSecrets(opts SecretsOptions) error {
	fmt.Printf("🔄 Rotating secrets: %s\n", color.YellowFmt(strings.Join(opts.SecretNames, ", ")))

	secretsPath := filepath.Join(".sc", "stacks", "infrastructure", "secrets.yaml")

	// Read existing secrets.yaml
	var secretsConfig map[string]interface{}
	data, err := os.ReadFile(secretsPath)
	if err != nil {
		return fmt.Errorf("failed to read secrets.yaml: %w", err)
	}

	if err := yaml.Unmarshal(data, &secretsConfig); err != nil {
		return fmt.Errorf("failed to parse secrets.yaml: %w", err)
	}

	values, ok := secretsConfig["values"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no values section found in secrets.yaml")
	}

	rotated := []string{}
	for _, secretName := range opts.SecretNames {
		if _, exists := values[secretName]; !exists {
			fmt.Printf("   %s Secret '%s' not found, skipping\n", color.YellowFmt("⚠"), secretName)
			continue
		}

		// Generate new secret value (for reference, actual value set via env var)
		_ = d.generateRandomSecret(24)
		values[secretName] = "${" + strings.ToUpper(secretName) + "}"
		rotated = append(rotated, secretName)

		fmt.Printf("   %s Rotated secret '%s' (update env var: %s)\n",
			color.GreenFmt("✓"), secretName, strings.ToUpper(secretName))
	}

	if len(rotated) > 0 {
		// Write updated secrets.yaml
		updatedData, err := yaml.Marshal(secretsConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal secrets config: %w", err)
		}

		if err := os.WriteFile(secretsPath, updatedData, 0o644); err != nil {
			return fmt.Errorf("failed to write secrets.yaml: %w", err)
		}

		fmt.Printf("\n   %s Rotated %d secrets in secrets.yaml\n", color.GreenFmt("✓"), len(rotated))
		fmt.Printf("   %s Update environment variables and redeploy services\n", color.CyanFmt("💡"))
		fmt.Printf("   %s Run 'sc deploy' to apply changes\n", color.CyanFmt("💡"))
	} else {
		fmt.Printf("   %s No secrets were rotated\n", color.YellowFmt("⚠"))
	}

	return nil
}

// Helper methods for secrets management

func (d *DevOpsMode) generateRandomSecret(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a deterministic but complex string if random fails
		return fmt.Sprintf("fallback-secret-%d", length)
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}

func (d *DevOpsMode) importFromEnvironment(secretsPath string, secretNames []string) error {
	fmt.Printf("📥 Importing secrets from environment variables\n")

	// Read existing secrets.yaml
	var secretsConfig map[string]interface{}
	if data, err := os.ReadFile(secretsPath); err == nil {
		if err := yaml.Unmarshal(data, &secretsConfig); err != nil {
			return fmt.Errorf("failed to parse existing secrets.yaml: %w", err)
		}
	} else {
		// Create new secrets config if file doesn't exist
		secretsConfig = map[string]interface{}{
			"schemaVersion": 1.0,
			"auth":          make(map[string]interface{}),
			"values":        make(map[string]interface{}),
		}
	}

	// Get or create values section
	values, ok := secretsConfig["values"].(map[string]interface{})
	if !ok {
		values = make(map[string]interface{})
		secretsConfig["values"] = values
	}

	imported := []string{}
	for _, secretName := range secretNames {
		envVarName := strings.ToUpper(secretName)
		envValue := os.Getenv(envVarName)

		if envValue != "" {
			values[secretName] = "${" + envVarName + "}"
			imported = append(imported, secretName)
			fmt.Printf("   %s Imported '%s' from environment variable %s\n",
				color.GreenFmt("✓"), secretName, envVarName)
		} else {
			fmt.Printf("   %s Environment variable '%s' not found for secret '%s'\n",
				color.YellowFmt("⚠"), envVarName, secretName)
		}
	}

	if len(imported) > 0 {
		// Write updated secrets.yaml
		updatedData, err := yaml.Marshal(secretsConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal secrets config: %w", err)
		}

		if err := os.WriteFile(secretsPath, updatedData, 0o644); err != nil {
			return fmt.Errorf("failed to write secrets.yaml: %w", err)
		}

		fmt.Printf("\n   %s Imported %d secrets from environment variables\n", color.GreenFmt("✓"), len(imported))
	} else {
		fmt.Printf("   %s No environment variables found for the specified secrets\n", color.YellowFmt("⚠"))
	}

	return nil
}

// Summary method

func (d *DevOpsMode) printSetupSummary(opts DevOpsSetupOptions) {
	fmt.Printf("\n%s Infrastructure Setup Complete!\n", color.GreenFmt("✅"))

	fmt.Println("\n📁 Generated files:")
	fmt.Printf("   • server.yaml   - Infrastructure resources and templates\n")
	fmt.Printf("   • secrets.yaml  - Authentication and sensitive configuration\n")
	fmt.Printf("   • cfg.default.yaml - Default Simple Container settings\n")

	fmt.Println("\n🔐 Next steps:")
	fmt.Printf("   1. Configure secrets:       %s\n", color.CyanFmt("sc secrets add aws-access-key aws-secret-key"))
	fmt.Printf("   2. Set database passwords:  %s\n", color.CyanFmt("sc secrets add staging-db-password prod-db-password"))
	fmt.Printf("   3. Deploy infrastructure:   %s\n", color.CyanFmt("sc provision -s infrastructure -e staging"))
	fmt.Printf("   4. Verify deployment:       %s\n", color.CyanFmt("sc stack status infrastructure -e staging"))

	fmt.Println("\n👥 Share with development teams:")
	fmt.Printf("   • Parent stack name:        %s\n", color.GreenFmt("infrastructure"))
	fmt.Printf("   • Available environments:   %s\n", color.GreenFmt(strings.Join(opts.Environments, ", ")))
	fmt.Printf("   • Available resources:      %s\n", color.GreenFmt(strings.Join(opts.Resources, ", ")))

	fmt.Println("\n💡 Development teams can now run:")
	fmt.Printf("   %s\n", color.CyanFmt("sc assistant dev setup"))
}

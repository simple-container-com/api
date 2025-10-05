package modes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api/logger/color"
)

// DevOpsMode handles infrastructure-focused workflows
type DevOpsMode struct{}

// NewDevOpsMode creates a new DevOps mode instance
func NewDevOpsMode() *DevOpsMode {
	return &DevOpsMode{}
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

	// TODO: Implement actual user input
	// For now, default to AWS
	fmt.Printf("\nChoice [1-5]: %s (auto-selected for demo)\n", color.YellowFmt("1"))
	return "aws", nil
}

func (d *DevOpsMode) selectEnvironments() ([]string, error) {
	fmt.Println("\n📊 Configure your environments:")
	fmt.Printf("%s Development (local docker-compose)\n", color.GreenFmt("✅"))
	fmt.Printf("%s Staging (cloud resources, cost-optimized)\n", color.GreenFmt("✅"))
	fmt.Printf("%s Production (cloud resources, high availability)\n", color.GreenFmt("✅"))
	fmt.Print("\nAdditional environments (preview, testing, etc.)? (y/n): ")

	// TODO: Implement actual user input
	// For now, use defaults
	fmt.Printf("%s (auto-selected for demo)\n", color.YellowFmt("n"))
	return []string{"staging", "production"}, nil
}

func (d *DevOpsMode) selectResources(cloudProvider string) ([]string, error) {
	fmt.Println("\n🎯 Select shared resources to provision:")

	fmt.Println("\nDatabases:")
	fmt.Printf("☑️ PostgreSQL (recommended for most apps)\n")
	fmt.Printf("☐ MongoDB (document database)\n")
	fmt.Printf("☐ MySQL (legacy compatibility)\n")
	fmt.Printf("☑️ Redis (caching & sessions)\n")

	fmt.Println("\nStorage:")
	fmt.Printf("☑️ S3-compatible bucket (file uploads)\n")
	fmt.Printf("☐ CDN (static asset distribution)\n")

	fmt.Println("\nCompute:")
	fmt.Printf("☑️ Container platform (ECS/GKE/K8s)\n")
	fmt.Printf("☐ Serverless functions\n")
	fmt.Printf("☐ Static site hosting\n")

	fmt.Println("\nMonitoring:")
	fmt.Printf("☐ Application monitoring\n")
	fmt.Printf("☐ Log aggregation\n")
	fmt.Printf("☐ Alerting (Slack/Email)\n")

	// TODO: Implement actual user input
	// For now, use selected defaults
	fmt.Printf("\nUsing selected resources: %s\n", color.GreenFmt("PostgreSQL, Redis, S3 Bucket"))
	return []string{"postgres", "redis", "s3-bucket"}, nil
}

// File generation methods

func (d *DevOpsMode) generateInfrastructureFiles(opts DevOpsSetupOptions) error {
	// Create output directory structure
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = ".sc/stacks/infrastructure"
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate server.yaml
	fmt.Printf("   📄 Generating server.yaml...")
	serverYaml := d.generateServerYAML(opts)
	serverPath := filepath.Join(outputDir, "server.yaml")
	if err := os.WriteFile(serverPath, []byte(serverYaml), 0644); err != nil {
		return fmt.Errorf("failed to write server.yaml: %w", err)
	}
	fmt.Printf(" %s\n", color.GreenFmt("✓"))

	// Generate secrets.yaml
	fmt.Printf("   📄 Generating secrets.yaml...")
	secretsYaml := d.generateSecretsYAML(opts)
	secretsPath := filepath.Join(outputDir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte(secretsYaml), 0644); err != nil {
		return fmt.Errorf("failed to write secrets.yaml: %w", err)
	}
	fmt.Printf(" %s\n", color.GreenFmt("✓"))

	// Generate cfg.default.yaml
	fmt.Printf("   📄 Generating cfg.default.yaml...")
	configYaml := d.generateDefaultConfig(opts)
	configPath := filepath.Join(".sc", "cfg.default.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create .sc directory: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		return fmt.Errorf("failed to write cfg.default.yaml: %w", err)
	}
	fmt.Printf(" %s\n", color.GreenFmt("✓"))

	return nil
}

func (d *DevOpsMode) generateServerYAML(opts DevOpsSetupOptions) string {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "mycompany"
	}

	yaml := fmt.Sprintf(`schemaVersion: 1.0

# Provisioner configuration
provisioner:
  pulumi:
    backend: s3
    state-storage:
      type: s3-bucket
      bucketName: %s-sc-state
      region: %s
    secrets-provider:
      type: aws-kms
      kmsKeyId: "alias/simple-container"
  auth:
    %s: "${auth:%s}"

# Reusable templates for application teams
templates:`, prefix, d.getDefaultRegion(opts.CloudProvider), opts.CloudProvider, opts.CloudProvider)

	// Add templates
	for _, template := range opts.Templates {
		switch template {
		case "web-app":
			yaml += fmt.Sprintf(`
  web-app:
    type: %s
    ecsClusterResource: ecs-cluster
    ecrRepositoryResource: web-registry`, d.getComputeTemplate(opts.CloudProvider))
		case "api-service":
			yaml += fmt.Sprintf(`
  api-service:
    type: %s
    ecsClusterResource: ecs-cluster
    ecrRepositoryResource: api-registry`, d.getComputeTemplate(opts.CloudProvider))
		}
	}

	yaml += "\n\n# Shared infrastructure resources\nresources:"

	// Add resources for each environment
	for _, env := range opts.Environments {
		yaml += fmt.Sprintf("\n  %s:", env)

		// Add compute cluster
		yaml += fmt.Sprintf(`
    ecs-cluster:
      type: %s
      name: %s-%s-cluster`, d.getClusterType(opts.CloudProvider), prefix, env)

		// Add container registries
		for _, template := range opts.Templates {
			registryName := strings.Replace(template, "-", "", -1)
			yaml += fmt.Sprintf(`
    %s-registry:
      type: %s
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
      name: %s-%s-cache
      nodeType: %s
      numCacheNodes: %s`, d.getCacheType(opts.CloudProvider), prefix, env, nodeType, nodes)

			case "s3-bucket":
				yaml += fmt.Sprintf(`
    uploads-bucket:
      type: s3-bucket
      name: %s-%s-uploads
      allowOnlyHttps: true`, prefix, env)
			}
		}
	}

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

	if err := os.WriteFile(serverYamlPath, updatedData, 0644); err != nil {
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

	if err := os.WriteFile(serverYamlPath, updatedData, 0644); err != nil {
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

	if err := os.WriteFile(serverYamlPath, updatedData, 0644); err != nil {
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

// Secrets management methods

func (d *DevOpsMode) initSecrets(opts SecretsOptions) error {
	fmt.Printf("🔐 Initializing secrets for %s\n", color.CyanFmt(opts.Provider))

	// TODO: Implement secrets initialization
	fmt.Println("Secrets initialization will be implemented in Phase 2")
	return nil
}

func (d *DevOpsMode) configureAuth(opts SecretsOptions) error {
	fmt.Printf("🔑 Configuring %s authentication\n", color.CyanFmt(opts.Provider))

	// TODO: Implement authentication configuration
	fmt.Println("Authentication configuration will be implemented in Phase 2")
	return nil
}

func (d *DevOpsMode) generateSecrets(opts SecretsOptions) error {
	fmt.Printf("🎲 Generating secrets: %s\n", color.GreenFmt(strings.Join(opts.SecretNames, ", ")))

	// TODO: Implement secret generation
	fmt.Println("Secret generation will be implemented in Phase 2")
	return nil
}

func (d *DevOpsMode) importSecrets(opts SecretsOptions) error {
	fmt.Println("📥 Importing secrets from external system")

	// TODO: Implement secret import
	fmt.Println("Secret import will be implemented in Phase 2")
	return nil
}

func (d *DevOpsMode) rotateSecrets(opts SecretsOptions) error {
	fmt.Printf("🔄 Rotating secrets: %s\n", color.YellowFmt(strings.Join(opts.SecretNames, ", ")))

	// TODO: Implement secret rotation
	fmt.Println("Secret rotation will be implemented in Phase 2")
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

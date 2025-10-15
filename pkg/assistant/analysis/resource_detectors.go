package analysis

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// ResourceDetector interface for implementing different resource detection strategies
type ResourceDetector interface {
	Detect(projectPath string) (*ResourceAnalysis, error)
	Name() string
	Priority() int
}

// EnvironmentVariableDetector scans for environment variable usage
type EnvironmentVariableDetector struct{}

func (d *EnvironmentVariableDetector) Name() string  { return "environment-variables" }
func (d *EnvironmentVariableDetector) Priority() int { return 100 }

func (d *EnvironmentVariableDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	var envVars []EnvironmentVariable
	envVarMap := make(map[string]*EnvironmentVariable)

	// Patterns for detecting environment variables
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]+)`),               // Node.js
		regexp.MustCompile(`os\.environ\.get\(['"]([A-Z][A-Z0-9_]+)['"]\)`), // Python
		regexp.MustCompile(`os\.environ\[['"]([A-Z][A-Z0-9_]+)['"]\]`),      // Python
		regexp.MustCompile(`os\.Getenv\(['"]([A-Z][A-Z0-9_]+)['"]\)`),       // Go
		regexp.MustCompile(`System\.getenv\(['"]([A-Z][A-Z0-9_]+)['"]\)`),   // Java
		regexp.MustCompile(`ENV\[['"]([A-Z][A-Z0-9_]+)['"]\]`),              // Ruby
		regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]+)\}`),                       // Shell/Docker
		regexp.MustCompile(`\$([A-Z][A-Z0-9_]+)`),                           // Shell/Docker
	}

	// .env file patterns
	envFilePattern := regexp.MustCompile(`^([A-Z][A-Z0-9_]+)=(.*)$`)

	// Collect all files first for parallel processing
	var filesToScan []string
	err := filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Skip paths with known dependency directories (faster than checking each dir)
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip common ignore directories
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only collect relevant files
		if shouldScanForEnvVars(entry.Name()) {
			filesToScan = append(filesToScan, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process files in parallel using errgroup
	var mu sync.Mutex
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(4) // Limit concurrent file processing

	for _, filePath := range filesToScan {
		filePath := filePath // capture loop variable
		g.Go(func() error {
			relPath, _ := filepath.Rel(projectPath, filePath)
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil // Continue on errors
			}

			contentStr := string(content)
			fileName := filepath.Base(filePath)
			localEnvVars := make(map[string]*EnvironmentVariable)

			// Handle .env files specially
			if strings.Contains(fileName, ".env") {
				scanner := bufio.NewScanner(strings.NewReader(contentStr))
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if matches := envFilePattern.FindStringSubmatch(line); matches != nil {
						envName := matches[1]
						defaultVal := matches[2]

						localEnvVars[envName] = &EnvironmentVariable{
							Name:       envName,
							Sources:    []string{relPath},
							UsageType:  d.determineUsageType(envName),
							Required:   true,
							DefaultVal: defaultVal,
						}
					}
				}
			} else {
				// Scan for environment variable usage in code files
				for _, pattern := range patterns {
					matches := pattern.FindAllStringSubmatch(contentStr, -1)
					for _, match := range matches {
						if len(match) > 1 {
							envName := match[1]
							if _, exists := localEnvVars[envName]; !exists {
								localEnvVars[envName] = &EnvironmentVariable{
									Name:      envName,
									Sources:   []string{relPath},
									UsageType: d.determineUsageType(envName),
									Required:  true, // Assume required if used in code
								}
							}
						}
					}
				}
			}

			// Merge local results with global map (thread-safe)
			mu.Lock()
			for envName, envVar := range localEnvVars {
				if existingVar, exists := envVarMap[envName]; exists {
					// Merge sources
					for _, source := range envVar.Sources {
						found := false
						for _, existingSource := range existingVar.Sources {
							if existingSource == source {
								found = true
								break
							}
						}
						if !found {
							existingVar.Sources = append(existingVar.Sources, source)
						}
					}
					if existingVar.DefaultVal == "" && envVar.DefaultVal != "" {
						existingVar.DefaultVal = envVar.DefaultVal
					}
				} else {
					envVarMap[envName] = envVar
				}
			}
			mu.Unlock()

			return nil
		})
	}

	// Wait for all file processing to complete
	_ = g.Wait() // Ignore errors

	// Convert map to slice
	for _, envVar := range envVarMap {
		envVar.Description = d.generateDescription(envVar.Name, envVar.UsageType)
		envVars = append(envVars, *envVar)
	}

	return &ResourceAnalysis{
		EnvironmentVars: envVars,
	}, nil
}

func (d *EnvironmentVariableDetector) determineUsageType(envName string) string {
	envName = strings.ToUpper(envName)

	// Database-related
	if strings.Contains(envName, "DATABASE") || strings.Contains(envName, "DB_") ||
		strings.Contains(envName, "POSTGRES") || strings.Contains(envName, "MYSQL") ||
		strings.Contains(envName, "MONGO") || strings.Contains(envName, "REDIS") {
		return "database_config"
	}

	// API keys and secrets
	if strings.Contains(envName, "KEY") || strings.Contains(envName, "SECRET") ||
		strings.Contains(envName, "TOKEN") || strings.Contains(envName, "PASSWORD") ||
		strings.Contains(envName, "PRIVATE") {
		return "secret"
	}

	// URLs and endpoints
	if strings.Contains(envName, "URL") || strings.Contains(envName, "ENDPOINT") ||
		strings.Contains(envName, "HOST") || strings.Contains(envName, "ADDRESS") {
		return "url"
	}

	// Port numbers
	if strings.Contains(envName, "PORT") {
		return "port"
	}

	// Feature flags
	if strings.Contains(envName, "ENABLE") || strings.Contains(envName, "DISABLE") ||
		strings.Contains(envName, "FEATURE") {
		return "feature_flag"
	}

	// Environment/mode
	if envName == "NODE_ENV" || envName == "ENVIRONMENT" || envName == "ENV" ||
		strings.Contains(envName, "MODE") {
		return "environment"
	}

	return "config"
}

func (d *EnvironmentVariableDetector) generateDescription(name, usageType string) string {
	switch usageType {
	case "database_config":
		return "Database connection configuration"
	case "secret":
		return "Sensitive credential or secret value"
	case "url":
		return "Service URL or endpoint configuration"
	case "port":
		return "Port number configuration"
	case "feature_flag":
		return "Feature toggle or flag"
	case "environment":
		return "Environment or mode specification"
	default:
		return "Application configuration parameter"
	}
}

// SecretDetector scans for hardcoded secrets and sensitive data patterns
type SecretDetector struct{}

func (d *SecretDetector) Name() string  { return "secrets" }
func (d *SecretDetector) Priority() int { return 95 }

func (d *SecretDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	var secrets []Secret

	// Patterns for detecting secrets (be careful not to match template placeholders)
	secretPatterns := []struct {
		pattern    *regexp.Regexp
		name       string
		secretType string
		confidence float32
	}{
		{regexp.MustCompile(`['"]sk-[a-zA-Z0-9]{32,}['"]`), "OpenAI API Key", "api_key", 0.9},
		{regexp.MustCompile(`['"]pk_live_[a-zA-Z0-9]{24,}['"]`), "Stripe Live Key", "api_key", 0.95},
		{regexp.MustCompile(`['"]pk_test_[a-zA-Z0-9]{24,}['"]`), "Stripe Test Key", "api_key", 0.95},
		{regexp.MustCompile(`['"]sk_live_[a-zA-Z0-9]{24,}['"]`), "Stripe Secret Live Key", "api_key", 0.95},
		{regexp.MustCompile(`['"]sk_test_[a-zA-Z0-9]{24,}['"]`), "Stripe Secret Test Key", "api_key", 0.95},
		{regexp.MustCompile(`['"]AKIA[0-9A-Z]{16}['"]`), "AWS Access Key", "aws_key", 0.9},
		{regexp.MustCompile(`['"]AIza[0-9A-Za-z-_]{35}['"]`), "Google API Key", "api_key", 0.85},
		{regexp.MustCompile(`['"][0-9a-f]{32}['"]`), "Generic 32-char hex key", "api_key", 0.6},
		{regexp.MustCompile(`jwt_secret\s*=\s*['"][^'"]+['"]`), "JWT Secret", "jwt_secret", 0.8},
		{regexp.MustCompile(`database.*://[^:]+:[^@]+@`), "Database URL with credentials", "database_url", 0.8},
		{regexp.MustCompile(`mongodb://[^:]+:[^@]+@`), "MongoDB URL with credentials", "database_url", 0.85},
		{regexp.MustCompile(`postgres://[^:]+:[^@]+@`), "PostgreSQL URL with credentials", "database_url", 0.85},
		{regexp.MustCompile(`mysql://[^:]+:[^@]+@`), "MySQL URL with credentials", "database_url", 0.85},
	}

	// Collect all files first for parallel processing
	var filesToScan []string
	err := filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip paths with known dependency directories (faster than checking each dir)
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only collect relevant files
		if shouldScanForSecrets(entry.Name()) {
			filesToScan = append(filesToScan, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process files in parallel using errgroup
	var mu sync.Mutex
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(4) // Limit concurrent file processing

	for _, filePath := range filesToScan {
		filePath := filePath // capture loop variable
		g.Go(func() error {
			relPath, _ := filepath.Rel(projectPath, filePath)
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil // Continue on errors
			}

			contentStr := string(content)

			// Skip if file contains template placeholder patterns (Simple Container specific)
			if strings.Contains(contentStr, "${resource:") || strings.Contains(contentStr, "${secret:") ||
				strings.Contains(contentStr, "${auth:") || strings.Contains(contentStr, "${dependency:") {
				return nil
			}

			var localSecrets []Secret

			// Scan for secret patterns
			for _, secretPattern := range secretPatterns {
				matches := secretPattern.pattern.FindAllString(contentStr, -1)
				for range matches {
					secret := Secret{
						Type:        secretPattern.secretType,
						Name:        secretPattern.name,
						Sources:     []string{relPath},
						Pattern:     secretPattern.pattern.String(),
						Confidence:  secretPattern.confidence,
						Recommended: d.getRecommendedResource(secretPattern.secretType),
					}
					localSecrets = append(localSecrets, secret)
				}
			}

			// Merge local results with global slice (thread-safe)
			if len(localSecrets) > 0 {
				mu.Lock()
				secrets = append(secrets, localSecrets...)
				mu.Unlock()
			}

			return nil
		})
	}

	// Wait for all file processing to complete
	_ = g.Wait() // Ignore errors

	return &ResourceAnalysis{
		Secrets: secrets,
	}, nil
}

func (d *SecretDetector) getRecommendedResource(secretType string) string {
	switch secretType {
	case "database_url":
		return "Use Simple Container secrets.yaml and ${secret:database-url} placeholder"
	case "api_key":
		return "Use Simple Container secrets.yaml and ${secret:api-key} placeholder"
	case "jwt_secret":
		return "Use Simple Container secrets.yaml and ${secret:jwt-secret} placeholder"
	case "aws_key":
		return "Use Simple Container ${auth:aws} authentication"
	default:
		return "Use Simple Container secrets.yaml"
	}
}

// DatabaseDetector enhanced to detect databases beyond just dependencies
type DatabaseDetector struct{}

func (d *DatabaseDetector) Name() string  { return "databases" }
func (d *DatabaseDetector) Priority() int { return 90 }

func (d *DatabaseDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	var databases []Database
	dbMap := make(map[string]*Database)

	// Configuration file patterns for different databases
	configPatterns := []struct {
		pattern    *regexp.Regexp
		dbType     string
		confidence float32
	}{
		// PostgreSQL
		{regexp.MustCompile(`(?i)postgres|postgresql`), "postgresql", 0.8},
		{regexp.MustCompile(`pg_[a-z]+|psycopg`), "postgresql", 0.85},
		{regexp.MustCompile(`port\s*:\s*5432`), "postgresql", 0.7},

		// MySQL
		{regexp.MustCompile(`(?i)mysql`), "mysql", 0.8},
		{regexp.MustCompile(`port\s*:\s*3306`), "mysql", 0.7},

		// MongoDB
		{regexp.MustCompile(`(?i)mongodb|mongo`), "mongodb", 0.8},
		{regexp.MustCompile(`port\s*:\s*27017`), "mongodb", 0.7},

		// Redis
		{regexp.MustCompile(`(?i)redis`), "redis", 0.8},
		{regexp.MustCompile(`port\s*:\s*6379`), "redis", 0.7},

		// SQLite
		{regexp.MustCompile(`\.db$|\.sqlite$|sqlite`), "sqlite", 0.8},

		// ElasticSearch (only match actual Elasticsearch, not ElastiCache/ElasticBeanstalk)
		{regexp.MustCompile(`(?i)\belasticsearch\b(?![\w])|elasticsearch[\.\-_]|elastic[\s]+search|elastic\.co`), "elasticsearch", 0.8},
		{regexp.MustCompile(`port\s*:\s*9200`), "elasticsearch", 0.7},
	}

	// Collect all files first for parallel processing
	var filesToScan []string
	err := filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip paths with known dependency directories (faster than checking each dir)
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only collect relevant files
		if shouldScanForDatabases(entry.Name()) {
			filesToScan = append(filesToScan, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process files in parallel using errgroup
	var mu sync.Mutex
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(4) // Limit concurrent file processing

	for _, filePath := range filesToScan {
		filePath := filePath // capture loop variable
		g.Go(func() error {
			relPath, _ := filepath.Rel(projectPath, filePath)
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil // Continue on errors
			}

			contentStr := string(content)
			localDBMap := make(map[string]*Database)

			// Scan for database patterns
			for _, dbPattern := range configPatterns {
				if dbPattern.pattern.MatchString(contentStr) {
					dbType := dbPattern.dbType

					// Skip false positives for Elasticsearch
					if dbType == "elasticsearch" {
						// Skip if this is AWS ElastiCache (Redis cache service) or services.json
						if strings.Contains(strings.ToLower(contentStr), "elasticache") ||
							strings.Contains(strings.ToLower(contentStr), "elastic cache") ||
							strings.Contains(strings.ToLower(relPath), "services.json") {
							continue
						}
					}

					if existingDB, exists := localDBMap[dbType]; exists {
						// Add source if not already present in local map
						found := false
						for _, source := range existingDB.Sources {
							if source == relPath {
								found = true
								break
							}
						}
						if !found {
							existingDB.Sources = append(existingDB.Sources, relPath)
						}
						// Increase confidence if found in multiple places
						existingDB.Confidence = minFloat32(existingDB.Confidence+0.1, 1.0)
					} else {
						connection := d.detectConnection(contentStr, dbType)
						version := d.detectVersion(contentStr, dbType)

						db := &Database{
							Type:        dbType,
							Sources:     []string{relPath},
							Connection:  connection,
							Version:     version,
							Config:      make(map[string]string),
							Confidence:  dbPattern.confidence,
							Recommended: d.getRecommendedResource(dbType),
						}

						// Extract additional config
						d.extractConfig(contentStr, db)

						localDBMap[dbType] = db
					}
				}
			}

			// Merge local results with global map (thread-safe)
			mu.Lock()
			for dbType, db := range localDBMap {
				if existingDB, exists := dbMap[dbType]; exists {
					// Merge sources
					for _, source := range db.Sources {
						found := false
						for _, existingSource := range existingDB.Sources {
							if existingSource == source {
								found = true
								break
							}
						}
						if !found {
							existingDB.Sources = append(existingDB.Sources, source)
						}
					}
					// Update confidence (take higher value)
					if db.Confidence > existingDB.Confidence {
						existingDB.Confidence = db.Confidence
					}
					// Merge config
					for key, value := range db.Config {
						existingDB.Config[key] = value
					}
				} else {
					dbMap[dbType] = db
				}
			}
			mu.Unlock()

			return nil
		})
	}

	// Wait for all file processing to complete
	_ = g.Wait() // Ignore errors

	// Convert map to slice
	for _, db := range dbMap {
		databases = append(databases, *db)
	}

	return &ResourceAnalysis{
		Databases: databases,
	}, nil
}

func (d *DatabaseDetector) detectConnection(content, dbType string) string {
	connectionLibs := map[string][]string{
		"postgresql": {"pg", "psycopg2", "pgx", "database/sql", "gorm"},
		"mysql":      {"mysql2", "mysql", "PyMySQL", "database/sql", "gorm"},
		"mongodb":    {"mongoose", "pymongo", "mongo-driver", "mongodb"},
		"redis":      {"redis", "ioredis", "redis-py", "redigo", "go-redis"},
		"sqlite":     {"sqlite3", "better-sqlite3", "sqlite", "database/sql"},
	}

	if libs, exists := connectionLibs[dbType]; exists {
		for _, lib := range libs {
			if strings.Contains(content, lib) {
				return lib
			}
		}
	}

	return ""
}

func (d *DatabaseDetector) detectVersion(content, dbType string) string {
	// Simple version detection - could be enhanced
	versionPatterns := map[string]*regexp.Regexp{
		"postgresql": regexp.MustCompile(`postgres(?:ql)?[:\s]+(\d+(?:\.\d+)*)`),
		"mysql":      regexp.MustCompile(`mysql[:\s]+(\d+(?:\.\d+)*)`),
		"mongodb":    regexp.MustCompile(`mongo(?:db)?[:\s]+(\d+(?:\.\d+)*)`),
		"redis":      regexp.MustCompile(`redis[:\s]+(\d+(?:\.\d+)*)`),
	}

	if pattern, exists := versionPatterns[dbType]; exists {
		if matches := pattern.FindStringSubmatch(content); len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

func (d *DatabaseDetector) extractConfig(content string, db *Database) {
	// Extract database-specific configuration
	switch db.Type {
	case "postgresql", "mysql":
		if matches := regexp.MustCompile(`host[:\s]+([^\s,;]+)`).FindStringSubmatch(content); len(matches) > 1 {
			db.Config["host"] = matches[1]
		}
		if matches := regexp.MustCompile(`port[:\s]+(\d+)`).FindStringSubmatch(content); len(matches) > 1 {
			db.Config["port"] = matches[1]
		}
		if matches := regexp.MustCompile(`database[:\s]+([^\s,;]+)`).FindStringSubmatch(content); len(matches) > 1 {
			db.Config["database"] = matches[1]
		}
	case "mongodb":
		if matches := regexp.MustCompile(`collection[:\s]+([^\s,;]+)`).FindStringSubmatch(content); len(matches) > 1 {
			db.Config["collection"] = matches[1]
		}
	case "redis":
		if matches := regexp.MustCompile(`db[:\s]+(\d+)`).FindStringSubmatch(content); len(matches) > 1 {
			db.Config["db"] = matches[1]
		}
	}
}

func (d *DatabaseDetector) getRecommendedResource(dbType string) string {
	recommendations := map[string]string{
		"postgresql":    "aws-rds-postgres or gcp-cloudsql-postgres or kubernetes-helm-postgres-operator",
		"mysql":         "aws-rds-mysql",
		"mongodb":       "mongodb-atlas",
		"redis":         "gcp-redis or kubernetes-helm-redis-operator",
		"sqlite":        "Consider upgrading to managed database for production",
		"elasticsearch": "Consider managed Elasticsearch service",
	}

	if recommendation, exists := recommendations[dbType]; exists {
		return recommendation
	}

	return "Consider managed database service"
}

// Helper functions
func shouldSkipDir(name string) bool {
	skipDirs := []string{
		"node_modules", "__pycache__", "vendor", "target", "build", "dist",
		".git", ".svn", ".hg", "coverage", "logs", "tmp", "temp",
		// JavaScript/Node.js
		".npm", ".yarn", "bower_components", ".next", ".nuxt", ".cache",
		// Meteor
		".meteor", "packages",
		// IDE and tools
		".idea", ".vscode", ".vs", ".DS_Store",
		// Testing and coverage
		"cypress", "jest_cache", "nyc_output", ".nyc_output",
		// Build outputs
		"out", "output", "public/build", "static/build",
		// Dependencies and caches
		"deps", "lib", ".tsbuildinfo",
	}

	for _, skip := range skipDirs {
		if name == skip {
			return true
		}
	}

	return false
}

// ShouldSkipPath checks if a path should be skipped (for nested dependencies)
func ShouldSkipPath(path string) bool {
	skipPatterns := []string{
		"/node_modules/",
		"/.meteor/",
		"/.git/",
		"/vendor/",
		"/build/",
		"/dist/",
		"/out/",
		"/coverage/",
		"/cypress/",
		"/.cache/",
		"/.npm/",
		"/.yarn/",
	}

	for _, pattern := range skipPatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}

	return false
}

func shouldScanForEnvVars(filename string) bool {
	// Scan code files and config files
	extensions := []string{
		".js", ".ts", ".jsx", ".tsx", ".mjs", // JavaScript/TypeScript
		".py", ".pyx", // Python
		".go",          // Go
		".java", ".kt", // Java/Kotlin
		".rb",                       // Ruby
		".php",                      // PHP
		".cs",                       // C#
		".cpp", ".cc", ".cxx", ".c", // C/C++
		".rs",           // Rust
		".yml", ".yaml", // YAML
		".json",                // JSON
		".toml",                // TOML
		".ini",                 // INI
		".conf",                // Config
		".env",                 // Env files
		".sh", ".bash", ".zsh", // Shell scripts
		"Dockerfile", "docker-compose", // Docker files
		"Makefile", // Makefiles
	}

	filename = strings.ToLower(filename)

	// Check for .env files
	if strings.Contains(filename, ".env") {
		return true
	}

	// Check extensions
	for _, ext := range extensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}

	// Special cases
	if filename == "dockerfile" || strings.HasPrefix(filename, "docker-compose") ||
		filename == "makefile" {
		return true
	}

	return false
}

func shouldScanForSecrets(filename string) bool {
	// Don't scan binary files, images, etc.
	skipExtensions := []string{
		".exe", ".bin", ".dll", ".so", ".dylib",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".webp",
		".mp3", ".mp4", ".avi", ".mov", ".wmv",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
		".zip", ".tar", ".gz", ".bz2", ".7z",
	}

	filename = strings.ToLower(filename)

	for _, ext := range skipExtensions {
		if strings.HasSuffix(filename, ext) {
			return false
		}
	}

	return true
}

func shouldScanForDatabases(filename string) bool {
	// Focus on configuration files and code files
	return shouldScanForEnvVars(filename) ||
		strings.Contains(filename, "config") ||
		strings.Contains(filename, "database") ||
		strings.Contains(filename, "db")
}

// QueueDetector detects messaging queues and pub/sub systems
type QueueDetector struct{}

func (d *QueueDetector) Name() string  { return "queues" }
func (d *QueueDetector) Priority() int { return 85 }

func (d *QueueDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	var queues []Queue
	queueMap := make(map[string]*Queue)

	// Queue system patterns
	queuePatterns := []struct {
		pattern    *regexp.Regexp
		queueType  string
		confidence float32
	}{
		// RabbitMQ
		{regexp.MustCompile(`(?i)rabbitmq|amqp`), "rabbitmq", 0.85},
		{regexp.MustCompile(`port\s*:\s*5672`), "rabbitmq", 0.7},

		// Apache Kafka
		{regexp.MustCompile(`(?i)kafka`), "kafka", 0.85},
		{regexp.MustCompile(`port\s*:\s*9092`), "kafka", 0.7},

		// AWS SQS (only AWS-specific patterns, exclude package dependency names)
		{regexp.MustCompile(`(?i)sqs\..*\.amazonaws\.com|aws[-_\s]*sqs|sqs[-_\s]*queue|queue[-_\s]*sqs`), "aws_sqs", 0.9},
		{regexp.MustCompile(`(?i)@aws-sdk\/client-sqs|aws-sdk.*sqs`), "aws_sqs", 0.6}, // Lower confidence for SDK deps

		// Redis Pub/Sub
		{regexp.MustCompile(`(?i)redis.*pub|redis.*sub|pub.*redis|sub.*redis`), "redis_pubsub", 0.8},
		{regexp.MustCompile(`\.publish\(|\.subscribe\(`), "redis_pubsub", 0.6},

		// Google Pub/Sub
		{regexp.MustCompile(`(?i)pubsub\.googleapis\.com|gcloud.*pubsub`), "gcp_pubsub", 0.9},
		{regexp.MustCompile(`(?i)google.*pubsub`), "gcp_pubsub", 0.7},

		// Azure Service Bus
		{regexp.MustCompile(`(?i)servicebus\.windows\.net|azure.*servicebus`), "azure_servicebus", 0.9},
		{regexp.MustCompile(`(?i)servicebus`), "azure_servicebus", 0.7},

		// NATS
		{regexp.MustCompile(`(?i)nats`), "nats", 0.8},
		{regexp.MustCompile(`port\s*:\s*4222`), "nats", 0.7},
	}

	// Topic/Queue name patterns
	topicPatterns := []*regexp.Regexp{
		regexp.MustCompile(`topic[:\s]*['"]([a-zA-Z0-9_.-]+)['"]`),
		regexp.MustCompile(`queue[:\s]*['"]([a-zA-Z0-9_.-]+)['"]`),
		regexp.MustCompile(`channel[:\s]*['"]([a-zA-Z0-9_.-]+)['"]`),
		regexp.MustCompile(`exchange[:\s]*['"]([a-zA-Z0-9_.-]+)['"]`),
	}

	// Collect all files first for parallel processing
	var filesToScan []string
	err := filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip paths with known dependency directories (faster than checking each dir)
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only collect relevant files
		if shouldScanForQueues(entry.Name()) {
			filesToScan = append(filesToScan, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process files in parallel using errgroup
	var mu sync.Mutex
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(4) // Limit concurrent file processing

	for _, filePath := range filesToScan {
		filePath := filePath // capture loop variable
		g.Go(func() error {
			relPath, _ := filepath.Rel(projectPath, filePath)
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil // Continue on errors
			}

			contentStr := string(content)
			localQueueMap := make(map[string]*Queue)

			// Detect queue systems
			for _, queuePattern := range queuePatterns {
				if queuePattern.pattern.MatchString(contentStr) {
					queueType := queuePattern.queueType

					// Skip false positives for AWS SQS
					if queueType == "aws_sqs" {
						pathLower := strings.ToLower(relPath)

						// Skip all package lock files entirely for queue detection
						if strings.Contains(pathLower, "package-lock.json") ||
							strings.Contains(pathLower, "yarn.lock") ||
							strings.Contains(pathLower, "pnpm-lock.yaml") {
							continue
						}
					}

					if existingQueue, exists := localQueueMap[queueType]; exists {
						// Add source if not already present in local map
						found := false
						for _, source := range existingQueue.Sources {
							if source == relPath {
								found = true
								break
							}
						}
						if !found {
							existingQueue.Sources = append(existingQueue.Sources, relPath)
						}
						existingQueue.Confidence = minFloat32(existingQueue.Confidence+0.1, 1.0)
					} else {
						queue := &Queue{
							Type:        queueType,
							Sources:     []string{relPath},
							Topics:      []string{},
							Confidence:  queuePattern.confidence,
							Recommended: d.getRecommendedResource(queueType),
						}

						// Extract topics/queues
						for _, topicPattern := range topicPatterns {
							matches := topicPattern.FindAllStringSubmatch(contentStr, -1)
							for _, match := range matches {
								if len(match) > 1 {
									topic := match[1]
									found := false
									for _, existing := range queue.Topics {
										if existing == topic {
											found = true
											break
										}
									}
									if !found {
										queue.Topics = append(queue.Topics, topic)
									}
								}
							}
						}

						localQueueMap[queueType] = queue
					}
				}
			}

			// Merge local results with global map (thread-safe)
			mu.Lock()
			for queueType, queue := range localQueueMap {
				if existingQueue, exists := queueMap[queueType]; exists {
					// Merge sources
					for _, source := range queue.Sources {
						found := false
						for _, existingSource := range existingQueue.Sources {
							if existingSource == source {
								found = true
								break
							}
						}
						if !found {
							existingQueue.Sources = append(existingQueue.Sources, source)
						}
					}
					// Merge topics
					for _, topic := range queue.Topics {
						found := false
						for _, existingTopic := range existingQueue.Topics {
							if existingTopic == topic {
								found = true
								break
							}
						}
						if !found {
							existingQueue.Topics = append(existingQueue.Topics, topic)
						}
					}
					// Update confidence (take higher value)
					if queue.Confidence > existingQueue.Confidence {
						existingQueue.Confidence = queue.Confidence
					}
				} else {
					queueMap[queueType] = queue
				}
			}
			mu.Unlock()

			return nil
		})
	}

	// Wait for all file processing to complete
	_ = g.Wait() // Ignore errors

	// Convert map to slice
	for _, queue := range queueMap {
		queues = append(queues, *queue)
	}

	return &ResourceAnalysis{
		Queues: queues,
	}, nil
}

func (d *QueueDetector) getRecommendedResource(queueType string) string {
	recommendations := map[string]string{
		"rabbitmq":         "kubernetes-helm-rabbitmq-operator",
		"kafka":            "Consider managed Kafka service",
		"aws_sqs":          "Use AWS SQS with ${auth:aws}",
		"redis_pubsub":     "gcp-redis or kubernetes-helm-redis-operator",
		"gcp_pubsub":       "Use GCP Pub/Sub with ${auth:gcloud}",
		"azure_servicebus": "Use Azure Service Bus",
		"nats":             "Consider managed NATS service",
	}

	if recommendation, exists := recommendations[queueType]; exists {
		return recommendation
	}

	return "Consider managed messaging service"
}

// StorageDetector detects cloud storage and file upload systems
type StorageDetector struct{}

func (d *StorageDetector) Name() string  { return "storage" }
func (d *StorageDetector) Priority() int { return 80 }

func (d *StorageDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	var storages []Storage
	storageMap := make(map[string]*Storage)

	// Storage service patterns
	storagePatterns := []struct {
		pattern     *regexp.Regexp
		storageType string
		confidence  float32
		purpose     string
	}{
		// AWS S3
		{regexp.MustCompile(`(?i)s3\.amazonaws\.com|aws.*s3`), "s3", 0.9, "cloud_storage"},
		{regexp.MustCompile(`(?i)s3`), "s3", 0.7, "cloud_storage"},

		// Google Cloud Storage
		{regexp.MustCompile(`(?i)storage\.googleapis\.com|gcs|google.*storage`), "gcs", 0.9, "cloud_storage"},
		{regexp.MustCompile(`(?i)gs://`), "gcs", 0.85, "cloud_storage"},

		// Azure Blob Storage
		{regexp.MustCompile(`(?i)blob\.core\.windows\.net|azure.*blob`), "azure_blob", 0.9, "cloud_storage"},
		{regexp.MustCompile(`(?i)azure.*storage`), "azure_blob", 0.7, "cloud_storage"},

		// File upload patterns
		{regexp.MustCompile(`(?i)multer|upload|file.*upload`), "file_upload", 0.6, "uploads"},
		{regexp.MustCompile(`(?i)multipart/form-data`), "file_upload", 0.7, "uploads"},

		// CDN patterns
		{regexp.MustCompile(`(?i)cloudfront|cdn`), "cdn", 0.8, "static"},
		{regexp.MustCompile(`(?i)static.*files|public.*assets`), "static_assets", 0.5, "static"},
	}

	// Bucket name patterns
	bucketPatterns := []*regexp.Regexp{
		regexp.MustCompile(`bucket[:\s]*['"]([a-zA-Z0-9_.-]+)['"]`),
		regexp.MustCompile(`s3://([a-zA-Z0-9_.-]+)`),
		regexp.MustCompile(`gs://([a-zA-Z0-9_.-]+)`),
		regexp.MustCompile(`container[:\s]*['"]([a-zA-Z0-9_.-]+)['"]`),
	}

	// Collect all files first for parallel processing
	var filesToScan []string
	err := filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip paths with known dependency directories (faster than checking each dir)
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only collect relevant files
		if shouldScanForStorage(entry.Name()) {
			filesToScan = append(filesToScan, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process files in parallel using errgroup
	var mu sync.Mutex
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(4) // Limit concurrent file processing

	for _, filePath := range filesToScan {
		filePath := filePath // capture loop variable
		g.Go(func() error {
			relPath, _ := filepath.Rel(projectPath, filePath)
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil // Continue on errors
			}

			contentStr := string(content)
			localStorageMap := make(map[string]*Storage)

			// Scan for storage patterns
			for _, storagePattern := range storagePatterns {
				if storagePattern.pattern.MatchString(contentStr) {
					storageType := storagePattern.storageType

					if existingStorage, exists := localStorageMap[storageType]; exists {
						// Add source if not already present in local map
						found := false
						for _, source := range existingStorage.Sources {
							if source == relPath {
								found = true
								break
							}
						}
						if !found {
							existingStorage.Sources = append(existingStorage.Sources, relPath)
						}
						existingStorage.Confidence = minFloat32(existingStorage.Confidence+0.1, 1.0)
					} else {
						storage := &Storage{
							Type:        storageType,
							Sources:     []string{relPath},
							Buckets:     []string{},
							Purpose:     storagePattern.purpose,
							Confidence:  storagePattern.confidence,
							Recommended: d.getRecommendedResource(storageType),
						}

						// Extract bucket names
						for _, bucketPattern := range bucketPatterns {
							matches := bucketPattern.FindAllStringSubmatch(contentStr, -1)
							for _, match := range matches {
								if len(match) > 1 {
									bucket := match[1]
									found := false
									for _, existing := range storage.Buckets {
										if existing == bucket {
											found = true
											break
										}
									}
									if !found {
										storage.Buckets = append(storage.Buckets, bucket)
									}
								}
							}
						}

						localStorageMap[storageType] = storage
					}
				}
			}

			// Merge local results with global map (thread-safe)
			mu.Lock()
			for storageType, storage := range localStorageMap {
				if existingStorage, exists := storageMap[storageType]; exists {
					// Merge sources
					for _, source := range storage.Sources {
						found := false
						for _, existingSource := range existingStorage.Sources {
							if existingSource == source {
								found = true
								break
							}
						}
						if !found {
							existingStorage.Sources = append(existingStorage.Sources, source)
						}
					}
					// Merge buckets
					for _, bucket := range storage.Buckets {
						found := false
						for _, existingBucket := range existingStorage.Buckets {
							if existingBucket == bucket {
								found = true
								break
							}
						}
						if !found {
							existingStorage.Buckets = append(existingStorage.Buckets, bucket)
						}
					}
					// Update confidence (take higher value)
					if storage.Confidence > existingStorage.Confidence {
						existingStorage.Confidence = storage.Confidence
					}
				} else {
					storageMap[storageType] = storage
				}
			}
			mu.Unlock()

			return nil
		})
	}

	// Wait for all file processing to complete
	_ = g.Wait() // Ignore errors

	// Convert map to slice
	for _, storage := range storageMap {
		storages = append(storages, *storage)
	}

	return &ResourceAnalysis{
		Storage: storages,
	}, nil
}

func (d *StorageDetector) getRecommendedResource(storageType string) string {
	recommendations := map[string]string{
		"s3":            "s3-bucket",
		"gcs":           "gcp-bucket",
		"azure_blob":    "Consider Azure Blob Storage resource",
		"file_upload":   "s3-bucket or gcp-bucket for scalable file storage",
		"cdn":           "s3-bucket with CloudFront or gcp-bucket with CDN",
		"static_assets": "s3-bucket for static website hosting",
	}

	if recommendation, exists := recommendations[storageType]; exists {
		return recommendation
	}

	return "Consider managed storage service"
}

// ExternalAPIDetector detects external API services
type ExternalAPIDetector struct{}

func (d *ExternalAPIDetector) Name() string  { return "external-apis" }
func (d *ExternalAPIDetector) Priority() int { return 75 }

func (d *ExternalAPIDetector) Detect(projectPath string) (*ResourceAnalysis, error) {
	var apis []ExternalAPI
	apiMap := make(map[string]*ExternalAPI)

	// External API patterns
	apiPatterns := []struct {
		pattern    *regexp.Regexp
		name       string
		purpose    string
		confidence float32
	}{
		// Payment services
		{regexp.MustCompile(`(?i)stripe\.com|stripe`), "stripe", "payment", 0.9},
		{regexp.MustCompile(`(?i)paypal\.com|paypal`), "paypal", "payment", 0.9},
		{regexp.MustCompile(`(?i)square\.com|squareup`), "square", "payment", 0.8},

		// Email services
		{regexp.MustCompile(`(?i)sendgrid\.com|sendgrid`), "sendgrid", "email", 0.9},
		{regexp.MustCompile(`(?i)mailgun\.com|mailgun`), "mailgun", "email", 0.9},
		{regexp.MustCompile(`(?i)ses\.amazonaws\.com|aws.*ses`), "aws_ses", "email", 0.9},

		// AI/ML services
		{regexp.MustCompile(`(?i)openai\.com|openai`), "openai", "ai", 0.9},
		{regexp.MustCompile(`(?i)anthropic\.com|claude`), "anthropic", "ai", 0.9},
		{regexp.MustCompile(`(?i)huggingface\.co|hugging.*face`), "huggingface", "ai", 0.8},

		// Communication
		{regexp.MustCompile(`(?i)twilio\.com|twilio`), "twilio", "communication", 0.9},
		{regexp.MustCompile(`(?i)slack\.com/api|slack.*api`), "slack", "communication", 0.8},
		{regexp.MustCompile(`(?i)discord\.com/api|discord.*api`), "discord", "communication", 0.8},

		// Analytics
		{regexp.MustCompile(`(?i)google-analytics|analytics\.google`), "google_analytics", "analytics", 0.9},
		{regexp.MustCompile(`(?i)mixpanel\.com|mixpanel`), "mixpanel", "analytics", 0.9},
		{regexp.MustCompile(`(?i)amplitude\.com|amplitude`), "amplitude", "analytics", 0.8},

		// Auth services
		{regexp.MustCompile(`(?i)auth0\.com|auth0`), "auth0", "authentication", 0.9},
		{regexp.MustCompile(`(?i)firebase\.google\.com|firebase`), "firebase", "backend_service", 0.9},
		{regexp.MustCompile(`(?i)supabase\.com|supabase`), "supabase", "backend_service", 0.9},

		// Maps and location
		{regexp.MustCompile(`(?i)maps\.googleapis\.com|google.*maps`), "google_maps", "maps", 0.9},
		{regexp.MustCompile(`(?i)mapbox\.com|mapbox`), "mapbox", "maps", 0.9},

		// Search
		{regexp.MustCompile(`(?i)algolia\.com|algolia`), "algolia", "search", 0.9},
		{regexp.MustCompile(`(?i)elasticsearch\.com`), "elastic_cloud", "search", 0.8},
	}

	// Endpoint patterns to extract API endpoints
	endpointPatterns := []*regexp.Regexp{
		regexp.MustCompile(`https?://[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/[^\s'"]*`),
		regexp.MustCompile(`api[_-]?endpoint[:\s]*['"]([^'"]+)['"]`),
		regexp.MustCompile(`base[_-]?url[:\s]*['"]([^'"]+)['"]`),
	}

	// Collect all files first for parallel processing
	var filesToScan []string
	err := filepath.WalkDir(projectPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip paths with known dependency directories (faster than checking each dir)
		if ShouldSkipPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only collect relevant files
		if shouldScanForAPIs(entry.Name()) {
			filesToScan = append(filesToScan, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process files in parallel using errgroup
	var mu sync.Mutex
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(4) // Limit concurrent file processing

	for _, filePath := range filesToScan {
		filePath := filePath // capture loop variable
		g.Go(func() error {
			relPath, _ := filepath.Rel(projectPath, filePath)
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil // Continue on errors
			}

			contentStr := string(content)
			localAPIMap := make(map[string]*ExternalAPI)

			// Scan for API patterns
			for _, apiPattern := range apiPatterns {
				if apiPattern.pattern.MatchString(contentStr) {
					apiName := apiPattern.name

					if existingAPI, exists := localAPIMap[apiName]; exists {
						// Add source if not already present in local map
						found := false
						for _, source := range existingAPI.Sources {
							if source == relPath {
								found = true
								break
							}
						}
						if !found {
							existingAPI.Sources = append(existingAPI.Sources, relPath)
						}
						existingAPI.Confidence = minFloat32(existingAPI.Confidence+0.1, 1.0)
					} else {
						api := &ExternalAPI{
							Name:       apiName,
							Sources:    []string{relPath},
							Endpoints:  []string{},
							Purpose:    apiPattern.purpose,
							Confidence: apiPattern.confidence,
						}

						// Extract endpoints
						for _, endpointPattern := range endpointPatterns {
							matches := endpointPattern.FindAllString(contentStr, -1)
							for _, match := range matches {
								// Filter to only relevant endpoints
								if strings.Contains(strings.ToLower(match), strings.ToLower(apiName)) {
									found := false
									for _, existing := range api.Endpoints {
										if existing == match {
											found = true
											break
										}
									}
									if !found {
										api.Endpoints = append(api.Endpoints, match)
									}
								}
							}
						}

						localAPIMap[apiName] = api
					}
				}
			}

			// Merge local results with global map (thread-safe)
			mu.Lock()
			for apiName, api := range localAPIMap {
				if existingAPI, exists := apiMap[apiName]; exists {
					// Merge sources
					for _, source := range api.Sources {
						found := false
						for _, existingSource := range existingAPI.Sources {
							if existingSource == source {
								found = true
								break
							}
						}
						if !found {
							existingAPI.Sources = append(existingAPI.Sources, source)
						}
					}
					// Merge endpoints
					for _, endpoint := range api.Endpoints {
						found := false
						for _, existingEndpoint := range existingAPI.Endpoints {
							if existingEndpoint == endpoint {
								found = true
								break
							}
						}
						if !found {
							existingAPI.Endpoints = append(existingAPI.Endpoints, endpoint)
						}
					}
					// Update confidence (take higher value)
					if api.Confidence > existingAPI.Confidence {
						existingAPI.Confidence = api.Confidence
					}
				} else {
					apiMap[apiName] = api
				}
			}
			mu.Unlock()

			return nil
		})
	}

	// Wait for all file processing to complete
	_ = g.Wait() // Ignore errors

	// Convert map to slice
	for _, api := range apiMap {
		apis = append(apis, *api)
	}

	return &ResourceAnalysis{
		ExternalAPIs: apis,
	}, nil
}

// Helper functions for new detectors
func shouldScanForQueues(filename string) bool {
	// Reduce false positives by being more selective with lock files
	if strings.Contains(filename, "package-lock.json") || strings.Contains(filename, "yarn.lock") {
		return false // Skip dependency lock files for queue detection
	}

	return shouldScanForEnvVars(filename) ||
		strings.Contains(filename, "queue") ||
		strings.Contains(filename, "messaging") ||
		strings.Contains(filename, "pub") ||
		strings.Contains(filename, "sub")
}

func shouldScanForStorage(filename string) bool {
	return shouldScanForEnvVars(filename) ||
		strings.Contains(filename, "storage") ||
		strings.Contains(filename, "upload") ||
		strings.Contains(filename, "file") ||
		strings.Contains(filename, "bucket") ||
		strings.Contains(filename, "assets")
}

func shouldScanForAPIs(filename string) bool {
	return shouldScanForEnvVars(filename) ||
		strings.Contains(filename, "api") ||
		strings.Contains(filename, "client") ||
		strings.Contains(filename, "service") ||
		strings.Contains(filename, "integration")
}

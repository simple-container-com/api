package plugins

import (
	"context"
	"fmt"
	"sync"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/assistant/analysis"
)

// PluginType defines the type of plugin
type PluginType string

const (
	PluginTypeAnalyzer    PluginType = "analyzer"
	PluginTypeGenerator   PluginType = "generator"
	PluginTypeProvider    PluginType = "provider"
	PluginTypeIntegration PluginType = "integration"
)

// PluginInfo contains metadata about a plugin
type PluginInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Type        PluginType        `json:"type"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Tags        []string          `json:"tags"`
	Config      map[string]string `json:"config"`
	Enabled     bool              `json:"enabled"`
}

// Plugin defines the interface that all plugins must implement
type Plugin interface {
	// GetInfo returns plugin metadata
	GetInfo() PluginInfo

	// Initialize prepares the plugin for use
	Initialize(ctx context.Context, config map[string]string) error

	// Shutdown cleans up plugin resources
	Shutdown(ctx context.Context) error

	// IsHealthy checks if the plugin is functioning correctly
	IsHealthy(ctx context.Context) error
}

// AnalyzerPlugin extends Plugin for project analysis capabilities
type AnalyzerPlugin interface {
	Plugin

	// AnalyzeProject performs custom project analysis
	AnalyzeProject(ctx context.Context, projectPath string) (*analysis.ProjectAnalysis, error)

	// GetSupportedLanguages returns languages this analyzer supports
	GetSupportedLanguages() []string

	// GetConfidenceScore returns how confident this analyzer is for a project
	GetConfidenceScore(ctx context.Context, projectPath string) (float64, error)
}

// GeneratorPlugin extends Plugin for file generation capabilities
type GeneratorPlugin interface {
	Plugin

	// GenerateFile creates a file based on analysis and templates
	GenerateFile(ctx context.Context, analysis *analysis.ProjectAnalysis, template string) (string, error)

	// GetSupportedTemplates returns templates this generator supports
	GetSupportedTemplates() []string

	// ValidateTemplate checks if a template is valid
	ValidateTemplate(ctx context.Context, template string) error
}

// ProviderPlugin extends Plugin for cloud provider integrations
type ProviderPlugin interface {
	Plugin

	// GetProviderName returns the cloud provider name
	GetProviderName() string

	// GetSupportedResources returns available resources for this provider
	GetSupportedResources() []string

	// ValidateCredentials checks if provider credentials are valid
	ValidateCredentials(ctx context.Context) error
}

// IntegrationPlugin extends Plugin for external service integrations
type IntegrationPlugin interface {
	Plugin

	// GetServiceName returns the integrated service name
	GetServiceName() string

	// TestConnection verifies connectivity to the external service
	TestConnection(ctx context.Context) error

	// GetCapabilities returns what this integration can do
	GetCapabilities() []string
}

// Registry manages all registered plugins
type Registry struct {
	plugins map[string]Plugin
	mu      sync.RWMutex
	logger  logger.Logger
}

// NewRegistry creates a new plugin registry
func NewRegistry(logger logger.Logger) *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
		logger:  logger,
	}
}

// Register adds a plugin to the registry
func (r *Registry) Register(ctx context.Context, plugin Plugin) error {
	info := plugin.GetInfo()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[info.ID]; exists {
		return fmt.Errorf("plugin with ID %s already registered", info.ID)
	}

	r.plugins[info.ID] = plugin

	r.logger.Info(ctx, "Plugin registered: plugin_id=%s, plugin_name=%s, plugin_type=%s, version=%s",
		info.ID, info.Name, string(info.Type), info.Version)

	return nil
}

// Unregister removes a plugin from the registry
func (r *Registry) Unregister(pluginID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	// Shutdown the plugin
	if err := plugin.Shutdown(context.Background()); err != nil {
		ctx := context.Background()
		r.logger.Warn(ctx, "Plugin shutdown error: plugin_id=%s, error=%s", pluginID, err.Error())
	}

	delete(r.plugins, pluginID)

	ctx := context.Background()
	r.logger.Info(ctx, "Plugin unregistered: plugin_id=%s", pluginID)

	return nil
}

// GetPlugin retrieves a specific plugin
func (r *Registry) GetPlugin(pluginID string) (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.plugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", pluginID)
	}

	return plugin, nil
}

// GetPluginsByType returns all plugins of a specific type
func (r *Registry) GetPluginsByType(pluginType PluginType) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Plugin
	for _, plugin := range r.plugins {
		if plugin.GetInfo().Type == pluginType {
			result = append(result, plugin)
		}
	}

	return result
}

// GetAllPlugins returns all registered plugins
func (r *Registry) GetAllPlugins() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Plugin
	for _, plugin := range r.plugins {
		result = append(result, plugin)
	}

	return result
}

// InitializeAll initializes all registered plugins
func (r *Registry) InitializeAll(ctx context.Context) error {
	r.mu.RLock()
	plugins := make([]Plugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}
	r.mu.RUnlock()

	var errors []error
	for _, plugin := range plugins {
		info := plugin.GetInfo()
		if !info.Enabled {
			continue
		}

		if err := plugin.Initialize(ctx, info.Config); err != nil {
			errors = append(errors, fmt.Errorf("failed to initialize plugin %s: %w", info.ID, err))
			r.logger.Error(ctx, "Plugin initialization failed: plugin_id=%s, error=%s", info.ID, err.Error())
		} else {
			r.logger.Info(ctx, "Plugin initialized: plugin_id=%s", info.ID)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to initialize %d plugins: %v", len(errors), errors)
	}

	return nil
}

// HealthCheck checks the health of all plugins
func (r *Registry) HealthCheck(ctx context.Context) map[string]error {
	r.mu.RLock()
	plugins := make(map[string]Plugin, len(r.plugins))
	for id, plugin := range r.plugins {
		plugins[id] = plugin
	}
	r.mu.RUnlock()

	results := make(map[string]error)

	for id, plugin := range plugins {
		if !plugin.GetInfo().Enabled {
			continue
		}

		err := plugin.IsHealthy(ctx)
		results[id] = err

		if err != nil {
			r.logger.Warn(ctx, "Plugin health check failed: plugin_id=%s, error=%s", id, err.Error())
		}
	}

	return results
}

// ShutdownAll shuts down all registered plugins
func (r *Registry) ShutdownAll(ctx context.Context) {
	r.mu.RLock()
	plugins := make([]Plugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}
	r.mu.RUnlock()

	for _, plugin := range plugins {
		info := plugin.GetInfo()
		if err := plugin.Shutdown(ctx); err != nil {
			r.logger.Error(ctx, "Plugin shutdown error: plugin_id=%s, error=%s", info.ID, err.Error())
		}
	}
}

// GetPluginInfo returns information about all registered plugins
func (r *Registry) GetPluginInfo() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var info []PluginInfo
	for _, plugin := range r.plugins {
		info = append(info, plugin.GetInfo())
	}

	return info
}

// EnablePlugin enables a specific plugin
func (r *Registry) EnablePlugin(ctx context.Context, pluginID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	info := plugin.GetInfo()
	if info.Enabled {
		return nil // Already enabled
	}

	// Initialize the plugin
	if err := plugin.Initialize(ctx, info.Config); err != nil {
		return fmt.Errorf("failed to initialize plugin %s: %w", pluginID, err)
	}

	// Update enabled status (this is simplified - in practice you'd need to modify the plugin)
	r.logger.Info(ctx, "Plugin enabled: plugin_id=%s", pluginID)

	return nil
}

// DisablePlugin disables a specific plugin
func (r *Registry) DisablePlugin(ctx context.Context, pluginID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	info := plugin.GetInfo()
	if !info.Enabled {
		return nil // Already disabled
	}

	// Shutdown the plugin
	if err := plugin.Shutdown(ctx); err != nil {
		r.logger.Warn(ctx, "Plugin shutdown error during disable: plugin_id=%s, error=%s", pluginID, err.Error())
	}

	r.logger.Info(ctx, "Plugin disabled: plugin_id=%s", pluginID)

	return nil
}

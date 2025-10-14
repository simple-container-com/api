package configdiff

import (
	"time"

	"github.com/simple-container-com/api/pkg/api"
)

// DiffFormat represents the output format for configuration diffs
type DiffFormat string

const (
	FormatUnified DiffFormat = "unified" // Git diff style with +/-
	FormatSplit   DiffFormat = "split"   // GitHub style, one line per change
	FormatInline  DiffFormat = "inline"  // Compact path: old → new
	FormatCompact DiffFormat = "compact" // Shortest, without stacks prefix
)

// ConfigDiff represents the differences between two configurations
type ConfigDiff struct {
	StackName   string      `json:"stack_name"`
	ConfigType  string      `json:"config_type"`
	CompareFrom string      `json:"compare_from"`
	CompareTo   string      `json:"compare_to"`
	Changes     []DiffLine  `json:"changes"`
	Summary     DiffSummary `json:"summary"`
	Warnings    []string    `json:"warnings"`
	GeneratedAt time.Time   `json:"generated_at"`
}

// DiffLine represents a single line change in the configuration
type DiffLine struct {
	Type        DiffLineType `json:"type"`
	Path        string       `json:"path"` // YAML path (e.g., "stacks.prod.config.scale.min")
	OldValue    string       `json:"old_value"`
	NewValue    string       `json:"new_value"`
	LineNumber  int          `json:"line_number"`
	Context     []string     `json:"context"`     // Surrounding lines for context
	Description string       `json:"description"` // Human-readable description of change
	Warning     string       `json:"warning"`     // Optional warning about this change
}

// DiffLineType represents the type of change
type DiffLineType string

const (
	DiffLineAdded     DiffLineType = "added"
	DiffLineRemoved   DiffLineType = "removed"
	DiffLineModified  DiffLineType = "modified"
	DiffLineUnchanged DiffLineType = "unchanged"
)

// DiffSummary provides statistics about the changes
type DiffSummary struct {
	TotalChanges         int      `json:"total_changes"`
	Additions            int      `json:"additions"`
	Deletions            int      `json:"deletions"`
	Modifications        int      `json:"modifications"`
	EnvironmentsAffected []string `json:"environments_affected"`
	HasWarnings          bool     `json:"has_warnings"`
}

// ResolvedConfig represents a configuration after all inheritance is resolved
type ResolvedConfig struct {
	StackName    string                 `json:"stack_name"`
	ConfigType   string                 `json:"config_type"`
	Content      string                 `json:"content"`       // YAML content
	ParsedConfig map[string]interface{} `json:"parsed_config"` // Parsed YAML structure
	ResolvedAt   time.Time              `json:"resolved_at"`
	GitRef       string                 `json:"git_ref"`   // Git reference if from git
	FilePath     string                 `json:"file_path"` // Original file path
	Metadata     map[string]interface{} `json:"metadata"`
}

// ConfigVersionProvider interface for getting configuration versions
type ConfigVersionProvider interface {
	GetCurrent(stackName, configType string) (*ResolvedConfig, error)
	GetFromGit(stackName, configType, gitRef string) (*ResolvedConfig, error)
	GetFromLocal(stackName, configType, filePath string) (*ResolvedConfig, error)
}

// ConfigResolver handles inheritance resolution
type ConfigResolver struct {
	stacksMap       api.StacksMap
	versionProvider ConfigVersionProvider
}

// DiffOptions configures how the diff is generated and formatted
type DiffOptions struct {
	Format           DiffFormat `json:"format"`
	ShowInheritance  bool       `json:"show_inheritance"`
	ContextLines     int        `json:"context_lines"`
	ObfuscateSecrets bool       `json:"obfuscate_secrets"`
	MaxChanges       int        `json:"max_changes"` // Limit output for large diffs
}

// DefaultDiffOptions returns sensible defaults for diff options
func DefaultDiffOptions() DiffOptions {
	return DiffOptions{
		Format:           FormatSplit, // Recommended format from examples
		ShowInheritance:  false,
		ContextLines:     3,
		ObfuscateSecrets: true,
		MaxChanges:       100,
	}
}

// ConfigDiffParams represents parameters for generating a config diff
type ConfigDiffParams struct {
	StackName   string      `json:"stack_name"`
	ConfigType  string      `json:"config_type"`
	CompareWith string      `json:"compare_with"` // Git ref to compare with
	Options     DiffOptions `json:"options"`
}

// ConfigDiffResult represents the result of a config diff operation
type ConfigDiffResult struct {
	Diff    *ConfigDiff `json:"diff"`
	Message string      `json:"message"` // Formatted output message
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
}

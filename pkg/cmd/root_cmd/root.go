package root_cmd

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.uber.org/atomic"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/provisioner"
)

type Params struct {
	Verbose bool
	Silent  bool
	Profile string

	PreviewTimeout *string
	DeployTimeout  *string

	api.InitParams
	IsCanceled *atomic.Bool
	CancelFunc func()
}

type RootCmd struct {
	*Params

	GitRepo     git.Repo
	Logger      logger.Logger
	Provisioner provisioner.Provisioner
}

type InitOpts struct {
	SkipScDirCreation    bool
	IgnoreConfigDirError bool
	ReturnOnGitError     bool
}

var IgnoreAllErrors = InitOpts{
	SkipScDirCreation:    true,
	IgnoreConfigDirError: true,
	ReturnOnGitError:     true,
}

func (c *RootCmd) Init(opts InitOpts) error {
	ctx := context.Background()

	c.Logger = logger.New()
	gitRepo, err := git.New(
		git.WithDetectRootDir(),
	)
	if err != nil && !opts.ReturnOnGitError {
		return err
	} else if err != nil && opts.ReturnOnGitError {
		return nil
	}

	c.Provisioner, err = provisioner.New(
		provisioner.WithGitRepo(gitRepo),
		provisioner.WithLogger(c.Logger),
	)
	if err != nil {
		return err
	}

	if err := c.Provisioner.Init(ctx, api.InitParams{
		ProjectName:         path.Base(gitRepo.Workdir()),
		RootDir:             gitRepo.Workdir(),
		SkipInitialCommit:   true,
		SkipProfileCreation: true,
		SkipScDirCreation:   opts.SkipScDirCreation,
		IgnoreWorkdirErrors: opts.SkipScDirCreation,
		Profile:             c.Params.Profile,
		GenerateKeyPair:     c.Params.GenerateKeyPair,
	}); err != nil {
		return err
	}

	if err := c.Provisioner.Cryptor().ReadProfileConfig(); err != nil && !opts.IgnoreConfigDirError {
		return errors.Wrapf(err, "failed to read profile config, did you run `init`?")
	}
	if err := c.Provisioner.Cryptor().ReadSecretFiles(); err != nil && !opts.IgnoreConfigDirError {
		return errors.Wrapf(err, "failed to read secrets file, did you run `init`?")
	}

	return nil
}

func RegisterDeployFlags(cmd *cobra.Command, p *api.DeployParams) {
	RegisterStackFlags(cmd, &p.StackParams, false)
	_ = cmd.MarkFlagRequired("env")
	cmd.Flags().StringVarP(&p.Version, "deploy-version", "V", os.Getenv("VERSION"), "Deploy version (default: `latest`)")

	cmd.Flags().StringVarP(&p.Timeouts.PreviewTimeout, "preview-timeout", "M", p.Timeouts.PreviewTimeout, "Timeout on preview operations (in Go's duration format, e.g. `20m`)")
	cmd.Flags().StringVarP(&p.Timeouts.ExecutionTimeout, "execution-timeout", "O", p.Timeouts.ExecutionTimeout, "Timeout on whole command execution (in Go's duration format, e.g. `20m`)")
	cmd.Flags().StringVarP(&p.Timeouts.DeployTimeout, "timeout", "T", p.Timeouts.DeployTimeout, "Timeout on deploy/provision operations (in Go's duration format, e.g. `20m`)")
}

func RegisterStackFlags(cmd *cobra.Command, p *api.StackParams, persistent bool) {
	flags := cmd.Flags()
	if persistent {
		flags = cmd.PersistentFlags()
	}
	flags.StringVarP(&p.Profile, "profile", "p", p.Profile, "Use profile (default: `default`)")
	flags.StringVarP(&p.StackName, "stack", "s", p.StackName, "Stack name to deploy (required)")
	_ = cmd.MarkFlagRequired("stack")
	flags.StringVarP(&p.Environment, "env", "e", p.Environment, "Environment to deploy")
	flags.StringVarP(&p.StacksDir, "dir", "d", p.StacksDir, "Root directory for stack configurations (default: .sc/stacks)")
	cmd.Flags().BoolVarP(&p.SkipRefresh, "skip-refresh", "R", p.SkipRefresh, "Skip refresh before deploy")
	cmd.Flags().BoolVarP(&p.SkipPreview, "skip-preview", "S", p.SkipPreview, "Skip preview before deploy")
	cmd.Flags().BoolVarP(&p.DetailedDiff, "diff", "D", p.DetailedDiff, "Show detailed diff with granular changes for nested properties (e.g., redisConfigs)")
	cmd.Flags().BoolVar(&p.DetailedDiff, "detailed-diff", p.DetailedDiff, "Alias for --diff")
	_ = cmd.Flags().MarkHidden("detailed-diff") // Hide the alias from help output

	// Register auto-completion for stack and environment parameters
	registerStackCompletion(cmd, "stack")
	registerEnvironmentCompletion(cmd, "env")
}

// completeStackNames provides auto-completion for stack names from .sc/stacks directory
func completeStackNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	stacksDir := ".sc/stacks"

	// Check if .sc/stacks directory exists
	if _, err := os.Stat(stacksDir); os.IsNotExist(err) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Read all directories in .sc/stacks
	entries, err := os.ReadDir(stacksDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var stackNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			stackName := entry.Name()
			// Filter based on the current input
			if strings.HasPrefix(stackName, toComplete) {
				stackNames = append(stackNames, stackName)
			}
		}
	}

	return stackNames, cobra.ShellCompDirectiveNoFileComp
}

// completeEnvironmentNames provides auto-completion for environment names from stack configurations
func completeEnvironmentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	environmentNames := make(map[string]bool) // Use map to avoid duplicates

	// Get stack name from flags if available
	stackName, _ := cmd.Flags().GetString("stack")

	if stackName != "" {
		// Look for environments in the specific stack's client.yaml
		clientPath := filepath.Join(".sc/stacks", stackName, "client.yaml")
		if envs := getEnvironmentsFromClientFile(clientPath); len(envs) > 0 {
			for _, env := range envs {
				if strings.HasPrefix(env, toComplete) {
					environmentNames[env] = true
				}
			}
		}

		// Also look for environments in the stack's server.yaml if it exists
		serverPath := filepath.Join(".sc/stacks", stackName, "server.yaml")
		if envs := getEnvironmentsFromServerFile(serverPath); len(envs) > 0 {
			for _, env := range envs {
				if strings.HasPrefix(env, toComplete) {
					environmentNames[env] = true
				}
			}
		}
	} else {
		// If no specific stack, scan all stacks for environment names
		stacksDir := ".sc/stacks"
		if entries, err := os.ReadDir(stacksDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					stackDir := entry.Name()

					// Check client.yaml
					clientPath := filepath.Join(stacksDir, stackDir, "client.yaml")
					if envs := getEnvironmentsFromClientFile(clientPath); len(envs) > 0 {
						for _, env := range envs {
							if strings.HasPrefix(env, toComplete) {
								environmentNames[env] = true
							}
						}
					}

					// Check server.yaml
					serverPath := filepath.Join(stacksDir, stackDir, "server.yaml")
					if envs := getEnvironmentsFromServerFile(serverPath); len(envs) > 0 {
						for _, env := range envs {
							if strings.HasPrefix(env, toComplete) {
								environmentNames[env] = true
							}
						}
					}
				}
			}
		}
	}

	// Convert map to slice
	var result []string
	for env := range environmentNames {
		result = append(result, env)
	}

	return result, cobra.ShellCompDirectiveNoFileComp
}

// getEnvironmentsFromClientFile extracts environment names from a client.yaml file
func getEnvironmentsFromClientFile(clientPath string) []string {
	var environments []string

	if _, err := os.Stat(clientPath); err != nil {
		return environments
	}

	// Try to read the client configuration
	var clientDesc api.ClientDescriptor
	if _, err := api.ReadDescriptor(clientPath, &clientDesc); err != nil {
		return environments
	}

	// Environment names are the keys in the stacks map
	for envName := range clientDesc.Stacks {
		environments = append(environments, envName)
	}

	return environments
}

// getEnvironmentsFromServerFile extracts environment names from a server.yaml file
func getEnvironmentsFromServerFile(serverPath string) []string {
	var environments []string

	if _, err := os.Stat(serverPath); err != nil {
		return environments
	}

	// Try to read the server configuration
	var serverDesc api.ServerDescriptor
	if _, err := api.ReadDescriptor(serverPath, &serverDesc); err != nil {
		return environments
	}

	// Extract environment names from resources
	for envName := range serverDesc.Resources.Resources {
		environments = append(environments, envName)
	}

	return environments
}

// registerStackCompletion adds stack name completion to a flag
func registerStackCompletion(cmd *cobra.Command, flagName string) {
	_ = cmd.RegisterFlagCompletionFunc(flagName, completeStackNames)
}

// registerEnvironmentCompletion adds environment name completion to a flag
func registerEnvironmentCompletion(cmd *cobra.Command, flagName string) {
	_ = cmd.RegisterFlagCompletionFunc(flagName, completeEnvironmentNames)
}

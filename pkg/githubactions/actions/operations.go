package actions

import (
	"context"
	"os"
	"strings"
)

// DeployClientStack deploys a client stack using SC's internal APIs
func (e *Executor) DeployClientStack(ctx context.Context) error {
	// Generate CalVer version if not provided or empty
	version := strings.TrimSpace(os.Getenv("VERSION"))
	if version == "" {
		var err error
		version, err = e.generateCalVerVersion(ctx)
		if err != nil {
			e.logger.Warn(ctx, "Failed to generate CalVer version: %v, using 'latest'", err)
			version = "latest"
		} else {
			// Tag the repository with the generated version
			if err := e.tagRepository(ctx, version); err != nil {
				e.logger.Warn(ctx, "Failed to tag repository with version %s: %v", version, err)
			}
		}
	}

	return e.executeOperation(ctx, OperationConfig{
		Type:      OperationDeploy,
		Scope:     ScopeClient,
		StackName: os.Getenv("STACK_NAME"),
		Env:       os.Getenv("ENVIRONMENT"),
		Version:   version,
	})
}

// ProvisionParentStack provisions a parent stack using SC's internal APIs
func (e *Executor) ProvisionParentStack(ctx context.Context) error {
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		stackName = "infrastructure" // Default for parent stacks
	}

	return e.executeOperation(ctx, OperationConfig{
		Type:      OperationProvision,
		Scope:     ScopeParent,
		StackName: stackName,
	})
}

// DestroyClientStack destroys a client stack using SC's internal APIs
func (e *Executor) DestroyClientStack(ctx context.Context) error {
	return e.executeOperation(ctx, OperationConfig{
		Type:      OperationDestroy,
		Scope:     ScopeClient,
		StackName: os.Getenv("STACK_NAME"),
		Env:       os.Getenv("ENVIRONMENT"),
	})
}

// DestroyParentStack destroys a parent stack using SC's internal APIs
func (e *Executor) DestroyParentStack(ctx context.Context) error {
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		stackName = "infrastructure" // Default for parent stacks
	}

	return e.executeOperation(ctx, OperationConfig{
		Type:      OperationDestroy,
		Scope:     ScopeParent,
		StackName: stackName,
	})
}

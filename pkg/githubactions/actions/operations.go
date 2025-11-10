package actions

import (
	"context"
	"os"
	"strings"

	"github.com/simple-container-com/api/pkg/api"
)

// DeployClientStack deploys a client stack using SC's internal APIs
func (e *Executor) DeployClientStack(ctx context.Context) error {
	// Generate CalVer version if not provided or empty
	version := strings.TrimSpace(os.Getenv("VERSION"))
	generatedVersion := false
	if version == "" {
		var err error
		version, err = e.generateCalVerVersion(ctx)
		if err != nil {
			e.logger.Warn(ctx, "Failed to generate CalVer version: %v, using 'latest'", err)
			version = "latest"
		} else {
			generatedVersion = true
		}
	}

	// Create deployment parameters
	deployParams := api.DeployParams{
		StackParams: api.StackParams{
			StackName:   os.Getenv("STACK_NAME"),
			Environment: os.Getenv("ENVIRONMENT"),
		},
	}

	// Wrap the deployment with signal handling and panic recovery
	err := e.signalHandler.WithSignalHandling(ctx, opTypeDeploy, deployParams, func(opCtx context.Context) error {
		return e.executeOperation(opCtx, OperationConfig{
			Type:      OperationDeploy,
			Scope:     ScopeClient,
			StackName: deployParams.StackName,
			Env:       deployParams.Environment,
			Version:   version,
		})
	})

	// Only tag the repository if deployment succeeded and we generated a version
	if err == nil && generatedVersion {
		if tagErr := e.tagRepository(ctx, version); tagErr != nil {
			e.logger.Warn(ctx, "Failed to tag repository with version %s: %v", version, tagErr)
		}
	}

	return err
}

// ProvisionParentStack provisions a parent stack using SC's internal APIs
func (e *Executor) ProvisionParentStack(ctx context.Context) error {
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		stackName = "infrastructure" // Default for parent stacks
	}

	// Create provision parameters
	provisionParams := api.ProvisionParams{
		Stacks: []string{stackName},
	}

	// Wrap the provision with signal handling and panic recovery
	return e.signalHandler.WithSignalHandling(ctx, opTypeProvision, provisionParams, func(opCtx context.Context) error {
		return e.executeOperation(opCtx, OperationConfig{
			Type:      OperationProvision,
			Scope:     ScopeParent,
			StackName: stackName,
		})
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

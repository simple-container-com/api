package actions

import (
	"context"
	"os"
)

// DeployClientStack deploys a client stack using SC's internal APIs
func (e *Executor) DeployClientStack(ctx context.Context) error {
	return e.executeOperation(ctx, OperationConfig{
		Type:      OperationDeploy,
		Scope:     ScopeClient,
		StackName: os.Getenv("STACK_NAME"),
		Env:       os.Getenv("ENVIRONMENT"),
		Version:   os.Getenv("VERSION"),
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

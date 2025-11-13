package actions

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/provisioner"
)

// SignalHandler manages graceful shutdown and panic recovery for GitHub Actions operations
type SignalHandler struct {
	logger      logger.Logger
	provisioner provisioner.Provisioner
	mu          sync.RWMutex
	activeOps   map[string]*activeOperation
}

type activeOperation struct {
	ctx        context.Context
	cancel     context.CancelFunc
	opType     operationType
	params     interface{} // Can be api.DeployParams or api.ProvisionParams
	cancelFunc func(context.Context) error
}

type operationType int

const (
	opTypeDeploy operationType = iota
	opTypeProvision
	opTypeDestroy
)

// NewSignalHandler creates a new signal handler for GitHub Actions
func NewSignalHandler(logger logger.Logger, prov provisioner.Provisioner) *SignalHandler {
	return &SignalHandler{
		logger:      logger,
		provisioner: prov,
		activeOps:   make(map[string]*activeOperation),
	}
}

// WithSignalHandling wraps a GitHub Actions operation with signal handling and panic recovery
func (sh *SignalHandler) WithSignalHandling(ctx context.Context, opType operationType, params interface{}, operation func(context.Context) error) error {
	// Create a cancellable context for this operation
	opCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Generate operation ID
	opID := sh.generateOperationID(opType, params)

	// Register the operation
	sh.registerOperation(opID, opCtx, cancel, opType, params)
	defer sh.unregisterOperation(opID)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// Channel to capture operation result
	resultChan := make(chan error, 1)

	// Run the operation in a goroutine with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				sh.logger.Error(opCtx, "🚨 Panic occurred during GitHub Actions operation %s: %v", opID, r)

				// Cancel the operation on panic - use a fresh context to avoid cancellation issues
				cancelCtx := context.WithoutCancel(opCtx)
				if err := sh.cancelOperation(cancelCtx, opID); err != nil {
					sh.logger.Error(cancelCtx, "❌ Failed to cancel operation %s after panic: %v", opID, err)
				}

				resultChan <- errors.Errorf("GitHub Actions operation panicked: %v", r)
			}
		}()

		// Execute the operation
		err := operation(opCtx)
		resultChan <- err
	}()

	// Wait for either completion, signal, or context cancellation
	select {
	case err := <-resultChan:
		return err
	case sig := <-sigChan:
		sh.logger.Info(opCtx, "🛑 Received signal %v, cancelling GitHub Actions operation %s", sig, opID)

		// Cancel the operation - use a fresh context to avoid cancellation issues
		cancelCtx := context.WithoutCancel(opCtx)
		if cancelErr := sh.cancelOperation(cancelCtx, opID); cancelErr != nil {
			sh.logger.Error(cancelCtx, "❌ Failed to cancel operation %s: %v", opID, cancelErr)
		}

		// Wait for operation to complete or timeout
		select {
		case err := <-resultChan:
			return errors.Wrapf(err, "GitHub Actions operation cancelled due to signal %v", sig)
		case <-opCtx.Done():
			return errors.Errorf("GitHub Actions operation %s cancelled due to signal %v", opID, sig)
		}
	case <-opCtx.Done():
		return opCtx.Err()
	}
}

// registerOperation registers an active operation
func (sh *SignalHandler) registerOperation(opID string, ctx context.Context, cancel context.CancelFunc, opType operationType, params interface{}) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	var cancelFunc func(context.Context) error
	switch opType {
	case opTypeDeploy:
		if deployParams, ok := params.(api.DeployParams); ok {
			cancelFunc = func(ctx context.Context) error {
				return sh.provisioner.Cancel(ctx, deployParams.StackParams)
			}
		}
	case opTypeProvision:
		if provisionParams, ok := params.(api.ProvisionParams); ok {
			cancelFunc = func(ctx context.Context) error {
				// For provision operations, we need to create appropriate StackParams for cancellation
				stackParams := api.StackParams{
					StacksDir: provisionParams.StacksDir,
					Parent:    true,
				}
				return sh.provisioner.CancelParent(ctx, stackParams)
			}
		}
	case opTypeDestroy:
		if destroyParams, ok := params.(api.DestroyParams); ok {
			cancelFunc = func(ctx context.Context) error {
				// For destroy operations, use Cancel for client stacks, CancelParent for parent stacks
				if destroyParams.StackParams.Parent {
					return sh.provisioner.CancelParent(ctx, destroyParams.StackParams)
				}
				return sh.provisioner.Cancel(ctx, destroyParams.StackParams)
			}
		}
	}

	sh.activeOps[opID] = &activeOperation{
		ctx:        ctx,
		cancel:     cancel,
		opType:     opType,
		params:     params,
		cancelFunc: cancelFunc,
	}

	sh.logger.Debug(ctx, "📝 Registered GitHub Actions operation %s for signal handling", opID)
}

// unregisterOperation removes an operation from tracking
func (sh *SignalHandler) unregisterOperation(opID string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	delete(sh.activeOps, opID)
	sh.logger.Debug(context.Background(), "🗑️  Unregistered GitHub Actions operation %s", opID)
}

// cancelOperation cancels a specific operation
func (sh *SignalHandler) cancelOperation(ctx context.Context, opID string) error {
	sh.mu.RLock()
	op, exists := sh.activeOps[opID]
	sh.mu.RUnlock()

	if !exists {
		return errors.Errorf("GitHub Actions operation %s not found", opID)
	}

	sh.logger.Info(ctx, "🛑 Cancelling GitHub Actions operation %s", opID)

	// Cancel the context first
	op.cancel()

	// Call the appropriate cancel function with a fresh context
	// Use WithoutCancel to ensure cancellation operations can complete
	// even if the original context was cancelled due to signal
	if op.cancelFunc != nil {
		cancelCtx := context.WithoutCancel(ctx)
		return op.cancelFunc(cancelCtx)
	}

	return nil
}

// CancelAllOperations cancels all active operations (useful for global shutdown)
func (sh *SignalHandler) CancelAllOperations(ctx context.Context) {
	sh.mu.RLock()
	ops := make(map[string]*activeOperation)
	for k, v := range sh.activeOps {
		ops[k] = v
	}
	sh.mu.RUnlock()

	sh.logger.Info(ctx, "🛑 Cancelling %d active GitHub Actions operations", len(ops))

	// Use a fresh context to ensure cancellation operations can complete
	// even if the original context was cancelled
	cancelCtx := context.WithoutCancel(ctx)
	for opID := range ops {
		if err := sh.cancelOperation(cancelCtx, opID); err != nil {
			sh.logger.Error(cancelCtx, "❌ Failed to cancel GitHub Actions operation %s: %v", opID, err)
		}
	}
}

// generateOperationID creates a unique ID for a GitHub Actions operation
func (sh *SignalHandler) generateOperationID(opType operationType, params interface{}) string {
	var opTypeStr string
	var identifier string

	switch opType {
	case opTypeDeploy:
		opTypeStr = "ga-deploy"
		if deployParams, ok := params.(api.DeployParams); ok {
			if deployParams.Environment != "" {
				identifier = deployParams.StackName + "-" + deployParams.Environment
			} else {
				identifier = deployParams.StackName
			}
		}
	case opTypeProvision:
		opTypeStr = "ga-provision"
		if provisionParams, ok := params.(api.ProvisionParams); ok {
			if len(provisionParams.Stacks) == 1 {
				identifier = provisionParams.Stacks[0]
			} else if len(provisionParams.Stacks) > 1 {
				identifier = "multi-stacks"
			} else {
				identifier = "all-stacks"
			}
		}
	case opTypeDestroy:
		opTypeStr = "ga-destroy"
		if destroyParams, ok := params.(api.DestroyParams); ok {
			if destroyParams.Environment != "" {
				identifier = destroyParams.StackName + "-" + destroyParams.Environment
			} else {
				identifier = destroyParams.StackName
			}
		}
	default:
		opTypeStr = "ga-unknown"
		identifier = "operation"
	}

	if identifier == "" {
		identifier = "operation"
	}

	return opTypeStr + "-" + identifier
}

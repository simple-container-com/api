package api

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ExpandStackReference(parentStack string, organization string, projectName string) string {
	parentStackParts := strings.SplitN(parentStack, "/", 3)
	if len(parentStackParts) == 3 {
		return parentStack
	} else if len(parentStackParts) == 2 {
		return fmt.Sprintf("%s/%s", organization, parentStack)
	} else {
		return fmt.Sprintf("%s/%s/%s", organization, projectName, parentStack)
	}
}

func CollapseStackReference(stackRef string) string {
	stackRefParts := strings.SplitN(stackRef, "/", 3)
	return stackRefParts[len(stackRefParts)-1]
}

func StackNameInEnv(stackName string, environment string) string {
	return fmt.Sprintf("%s--%s", stackName, environment)
}

func GetValueFromStack[T any](ctx *sdk.Context, refName, stackName, outName string, secret bool) (T, error) {
	// Create a StackReference to the parent stack
	ref, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s-ref", refName), &sdk.StackReferenceArgs{
		Name: sdk.String(stackName).ToStringOutput(),
	})
	if err != nil {
		return lo.Empty[T](), err
	}
	parentOutput, err := ref.GetOutputDetails(outName)
	if err != nil {
		return lo.Empty[T](), errors.Wrapf(err, "failed to get output %q from %q", outName, refName)
	}
	if secret && parentOutput.SecretValue == nil {
		return lo.Empty[T](), errors.Wrapf(err, "no secret value for output %q from %q", outName, refName)
	} else if secret {
		return parentOutput.SecretValue.(T), nil
	}
	if !secret && parentOutput.Value == nil {
		return lo.Empty[T](), errors.Wrapf(err, "no value for output %q from %q", outName, refName)
	}
	return parentOutput.Value.(T), nil
}

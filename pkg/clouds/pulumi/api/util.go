package api

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

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

func GetSecretStringValueFromStack(ctx *sdk.Context, refName, outName string) (string, error) {
	// Create a StackReference to the parent stack
	ref, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s-ref", outName), &sdk.StackReferenceArgs{
		Name: sdk.String(refName).ToStringOutput(),
	})
	if err != nil {
		return "", err
	}
	parentOutput, err := ref.GetOutputDetails(outName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get output %q from %q", outName, refName)
	}
	if parentOutput.SecretValue == nil {
		return "", errors.Wrapf(err, "no secret value for output %q from %q", outName, refName)
	}
	return parentOutput.SecretValue.(string), nil
}

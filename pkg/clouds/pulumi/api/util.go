// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
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
	return api.ParentStackName(stackRef)
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
	// Guard both type assertions with comma-ok. A hard `.(T)` panicked with
	// `interface conversion: interface {} is map[string]interface {}, not string`
	// on 2026-10-30 (api release run 41105957047) when Pulumi could not
	// decrypt a stack reference secret and substituted a placeholder map for
	// the expected string — the trigger was an upstream `[400] Bad Request:
	// read tcp ...: i/o timeout` against the state backend ("eliding
	// undecryptable secrets for stack reference"). A transient on the
	// state backend turned into an unrecovered panic that aborted the
	// deploy with no actionable diagnostic. Returning a typed error lets
	// the operator see which output was unreadable and whether it was a
	// secret vs a plain value, so a retry is diagnostic rather than
	// mystery.
	//
	// The `parentOutput.{,Secret}Value == nil` paths deliberately keep the
	// original `errors.Wrapf(err=nil, …) == nil` behaviour — a missing
	// output returns the zero value and no error, so a caller that is
	// `empty on an older parent stack` stays quiet instead of blowing up
	// (compose_stack.go:678 reads an open-ports export and must gracefully
	// handle the absent case). Only the wrong-type path, which did NOT
	// exist in a code shape where silence was ever the right answer, now
	// surfaces as a typed error.
	if secret {
		if parentOutput.SecretValue == nil {
			return lo.Empty[T](), nil
		}
		v, ok := parentOutput.SecretValue.(T)
		if !ok {
			return lo.Empty[T](), errors.Errorf("secret output %q from %q is %T, not %T (undecryptable secret? state backend reachable?)", outName, refName, parentOutput.SecretValue, lo.Empty[T]())
		}
		return v, nil
	}
	if parentOutput.Value == nil {
		return lo.Empty[T](), nil
	}
	v, ok := parentOutput.Value.(T)
	if !ok {
		return lo.Empty[T](), errors.Errorf("output %q from %q is %T, not %T (undecryptable secret rendered as placeholder? state backend reachable?)", outName, refName, parentOutput.Value, lo.Empty[T]())
	}
	return v, nil
}

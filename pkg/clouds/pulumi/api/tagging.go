package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

// Tag keys for consistent resource identification across clouds
const (
	// AWS tags - can contain dots
	// StackTag identifies the stack name
	StackTag = "simple-container.com/stack"

	// EnvironmentTag identifies the environment (e.g., production, staging)
	EnvironmentTag = "simple-container.com/env"

	// ParentStackTag identifies the parent stack for nested stacks
	ParentStackTag = "simple-container.com/parent-stack"

	// ClientStackTag identifies the client stack for nested stacks
	ClientStackTag = "simple-container.com/client-stack"

	// GCP labels - cannot contain dots, using hyphens instead
	// GCPStackTag identifies the stack name
	GCPStackTag = "simple-container-com/stack"

	// GCPEnvironmentTag identifies the environment (e.g., production, staging)
	GCPEnvironmentTag = "simple-container-com/env"

	// GCPParentStackTag identifies the parent stack for nested stacks
	GCPParentStackTag = "simple-container-com/parent-stack"

	// GCPClientStackTag identifies the client stack for nested stacks
	GCPClientStackTag = "simple-container-com/client-stack"
)

// Tags represents a set of tags/labels that can be applied to cloud resources
type Tags struct {
	StackName   string
	Environment string
	ParentStack *string
	ClientStack *string
}

// ToAWSTags converts Tags to AWS tag format
func (t *Tags) ToAWSTags() sdk.StringMap {
	tags := sdk.StringMap{
		StackTag:       sdk.String(t.StackName),
		EnvironmentTag: sdk.String(t.Environment),
	}

	if t.ParentStack != nil && *t.ParentStack != "" {
		tags[ParentStackTag] = sdk.String(*t.ParentStack)
	}

	if t.ClientStack != nil && *t.ClientStack != "" {
		tags[ClientStackTag] = sdk.String(*t.ClientStack)
	}

	return tags
}

// ToGCPLabels converts Tags to GCP label format
func (t *Tags) ToGCPLabels() map[string]string {
	labels := map[string]string{
		GCPStackTag:       t.StackName,
		GCPEnvironmentTag: t.Environment,
	}

	if t.ParentStack != nil && *t.ParentStack != "" {
		labels[GCPParentStackTag] = *t.ParentStack
	}

	if t.ClientStack != nil && *t.ClientStack != "" {
		labels[GCPClientStackTag] = *t.ClientStack
	}

	return labels
}

// BuildTagsFromStackParams creates Tags from StackParams
func BuildTagsFromStackParams(params api.StackParams) *Tags {
	tags := &Tags{
		StackName:   params.StackName,
		Environment: params.Environment,
	}
	return tags
}

// BuildTagsFromStackParamsWithParent creates Tags from StackParams with parent and client stack info
func BuildTagsFromStackParamsWithParent(params api.StackParams, parentStack, clientStack *string) *Tags {
	tags := &Tags{
		StackName:   params.StackName,
		Environment: params.Environment,
		ParentStack: parentStack,
		ClientStack: clientStack,
	}
	return tags
}

package aws

import (
	"strings"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// permissionsBoundaryPtr renders an AccountConfig.PermissionsBoundary ARN as
// the PermissionsBoundary input for an iam.RoleArgs. An empty ARN yields nil,
// i.e. no boundary is set on the role — the default for every stack that does
// not opt in, so existing deployments are unaffected. Setting/adding a
// boundary is an in-place role update (never a replacement).
func permissionsBoundaryPtr(arn string) sdk.StringPtrInput {
	arn = strings.TrimSpace(arn)
	if arn == "" {
		return nil
	}
	return sdk.String(arn)
}

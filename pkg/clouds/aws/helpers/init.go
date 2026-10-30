// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package helpers

import (
	"github.com/simple-container-com/api/pkg/api"
)

// init registers the two CloudHelper constructors with SC core's
// `api.cloudHelpersConfigMapping` registry. The ONLY consumer of
// that registry is the `cloud-helpers` Lambda binary at
// `cmd/cloud-helpers/main.go`, which blank-imports this package.
//
// Why this package stays in SC core (and not in sc-plugin-aws):
// `cmd/cloud-helpers` ships as the `simplecontainer/cloud-helpers`
// Docker image published by this repo's push.yaml, with provenance
// signed under the same trust root as `sc`. Keeping that pipeline
// intact — rather than migrating the image publish to each cloud
// plugin's own repo — avoids a signing-trust split across repos
// (push.yaml §449 explicit intent). The helpers themselves have
// zero pulumi-aws coupling, so leaving them in SC core costs only
// `aws-sdk-go-v2/service/{cloudtrail,cloudwatchlogs}` (~5 MB
// combined vs the 46 MB removed by the pulumi-aws drop).
func init() {
	api.RegisterCloudHelper(api.CloudHelpersRegisterMap{
		CHCloudwatchAlertLambda:   NewCloudwatchLambdaHelper,
		CHHealthBridgeAlertLambda: NewHealthBridgeLambdaHelper,
	})
}

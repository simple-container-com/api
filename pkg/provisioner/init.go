// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	// sc-plugin-mongodb/api blank-import stripped as part of A3
	// smoke; see pkg/clouds/pulumi/init.go for the rationale.
	_ "github.com/simple-container-com/api/pkg/clouds/gcloud"
	_ "github.com/simple-container-com/api/pkg/clouds/github"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi"
)

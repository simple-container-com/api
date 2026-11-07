// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	// sc-plugin-mongodb/api blank-import stripped as part of A3
	// smoke; see pkg/clouds/pulumi/init.go for the rationale.
	// pkg/clouds/gcloud blank-import stripped in phase 5 of the GCP
	// plugin extraction — sc-plugin-gcp now owns every GCP type-token
	// authoritatively via its own /api init + plugin_bootstrap.go.
	_ "github.com/simple-container-com/api/pkg/clouds/github"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi"
)

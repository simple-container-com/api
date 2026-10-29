// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	_ "github.com/simple-container-com/sc-plugin-mongodb/api"

	_ "github.com/simple-container-com/api/pkg/clouds/gcloud"
	_ "github.com/simple-container-com/api/pkg/clouds/github"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi"
)

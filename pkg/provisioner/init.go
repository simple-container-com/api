package provisioner

import (
	_ "api/pkg/clouds/cloudflare"
	_ "api/pkg/clouds/gcloud"
	_ "api/pkg/clouds/github"
	_ "api/pkg/clouds/mongodb"
	_ "api/pkg/clouds/pulumi"
)

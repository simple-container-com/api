package provisioner

import (
	_ "api/pkg/api/clouds/cloudflare"
	_ "api/pkg/api/clouds/gcloud"
	_ "api/pkg/api/clouds/github"
	_ "api/pkg/api/clouds/mongodb"
	_ "api/pkg/api/clouds/pulumi"
)

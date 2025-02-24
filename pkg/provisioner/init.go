package provisioner

import (
	_ "github.com/simple-container-com/api/pkg/clouds/cloudflare"
	_ "github.com/simple-container-com/api/pkg/clouds/gcloud"
	_ "github.com/simple-container-com/api/pkg/clouds/github"
	_ "github.com/simple-container-com/api/pkg/clouds/mongodb"
	_ "github.com/simple-container-com/api/pkg/clouds/pulumi"
	_ "github.com/simple-container-com/api/pkg/clouds/yandex"
)

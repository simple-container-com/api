package kubernetes

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

type ContainerImage struct {
	Container k8s.CloudRunContainer
	ImageName sdk.StringOutput
	AddOpts   []sdk.ResourceOption
}

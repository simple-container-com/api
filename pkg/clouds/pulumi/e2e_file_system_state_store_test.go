//go:build e2e

package pulumi

import (
	"testing"

	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	e2eFileSystemStateParentStackName = "e2e-fs--parent--stack"
)

func Test_CreateFileSystemStateParentStack(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2Etest()

	parentStackName := tmpResName(e2eFileSystemStateParentStackName)

	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForFileSystem(e2eConfig{
			templates: map[string]api.StackDescriptor{},
			resources: map[string]api.PerEnvResourcesDescriptor{},
			registrar: api.RegistrarDescriptor{},
		}),
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{},
		},
	}

	runProvisionTest(stack, cfg)
	runDestroyParentTest(stack, cfg)
}

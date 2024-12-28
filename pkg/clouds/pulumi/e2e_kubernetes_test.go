//go:build e2e

package pulumi

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/samber/lo"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/testutil"
	"github.com/simple-container-com/welder/pkg/exec"
	"github.com/simple-container-com/welder/pkg/util"

	. "github.com/onsi/gomega"
)

const (
	e2eKubernetesParentStackName = "e2e-kubernetes--parent--stack"
	e2eKubernetesChildStackName  = "e2e-kubernetes--child--stack"
)

func Test_CreateKubernetesParentStack(t *testing.T) {
	RegisterTestingT(t)

	cfg := testutil.PrepareE2Etest()

	parentStackName := tmpResName(e2eKubernetesParentStackName)
	childStackName := tmpResName(e2eKubernetesChildStackName)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kubeconfig, cancel := startK3dCluster(ctx)
	defer cancel()

	stack := api.Stack{
		Name: parentStackName,
		Server: e2eServerDescriptorForFileSystem(e2eConfig{
			templates: map[string]api.StackDescriptor{
				"e2e-kubernetes": {
					Type: k8s.TemplateTypeKubernetesCloudrun,
					Config: api.Config{
						Config: k8s.CloudrunTemplate{
							KubernetesConfig: k8s.KubernetesConfig{
								Kubeconfig: kubeconfig,
							},
							RegistryCredentials: lo.FromPtr(cfg.DockerCreds),
							CaddyResource:       lo.ToPtr("caddy"),
						},
					},
					Inherit: api.Inherit{},
				},
			},
			resources: map[string]api.PerEnvResourcesDescriptor{
				"test": {
					Template: "e2e-kubernetes",
					Resources: map[string]api.ResourceDescriptor{
						"caddy": {
							Type: k8s.ResourceTypeCaddy,
							Config: api.Config{
								Config: &k8s.CaddyResource{
									KubernetesConfig: &k8s.KubernetesConfig{
										Kubeconfig: kubeconfig,
									},
									CaddyConfig: &k8s.CaddyConfig{
										Enable:    lo.ToPtr(true),
										Namespace: lo.ToPtr("caddy"),
										Replicas:  lo.ToPtr(2),
									},
								},
							},
							Inherit: api.Inherit{},
						},
					},
					Inherit: api.Inherit{},
				},
			},
			registrar: api.RegistrarDescriptor{},
		}),
		Client: api.ClientDescriptor{
			Stacks: map[string]api.StackClientDescriptor{
				"test": {
					Type:        api.ClientTypeCloudCompose,
					ParentStack: parentStackName,
					Template:    "e2e-kubernetes",
					Config: api.Config{
						Config: &api.StackConfigCompose{
							DockerComposeFile: "docker-compose.yaml",
							Runs: []string{
								"backend",
							},
						},
					},
				},
			},
		},
	}

	runProvisionAndDeployTest(stack, cfg, childStackName)
	runDestroyParentTest(stack, cfg)
}

func startK3dCluster(ctx context.Context) (string, func()) {
	clusterName := tmpResName("kube-e2e-test")

	k3d := exec.NewExec(ctx, util.NewStdoutLogger(os.Stdout, os.Stderr))
	err := k3d.ProxyExec(fmt.Sprintf("k3d cluster create %s", clusterName), exec.Opts{})
	Expect(err).To(BeNil())

	kubeconfig, err := k3d.ExecCommand(fmt.Sprintf("k3d kubeconfig get %s", clusterName), exec.Opts{})
	Expect(err).To(BeNil())

	return kubeconfig, func() {
		_ = k3d.ProxyExec(fmt.Sprintf("k3d cluster delete %s", clusterName), exec.Opts{})
	}
}

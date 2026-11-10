// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	"github.com/simple-container-com/api/pkg/clouds/vm"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// componentMergeVmCompose is the per-template-type merge func for
// `vm-compose`-dispatched SC components. The pass-through default from
// plugin_sidecars.go gives back a ResourceDescriptor whose Config is
// the template's own *vm.ComposeTemplateConfig, but vm-compose's
// ProvisionFunc (VmComposeStack) casts to *vm.VmComposeInput — a
// supertype that embeds ComposeTemplateConfig AND carries a
// k8s.DeploymentConfig naming the containers to run.
//
// For a component dispatch, we construct that DeploymentConfig from the
// component's plugin-supplied .image (+ env + ingressRoutes in future
// slices). Slice 1a smoke exercises just the image path.
func componentMergeVmCompose(template api.StackDescriptor, componentResourceName string, cfg *api.ComponentConfig) (*api.ResourceDescriptor, error) {
	tplCfg, ok := template.Config.Config.(*vm.ComposeTemplateConfig)
	if !ok {
		return nil, errors.Errorf("component merge: template (type %q) has config %T, want *vm.ComposeTemplateConfig", template.Type, template.Config.Config)
	}
	image, err := cfg.Image()
	if err != nil {
		return nil, errors.Wrap(err, "component merge: image required")
	}
	envMap, err := cfg.ServiceEnv()
	if err != nil {
		return nil, errors.Wrap(err, "component merge: env parse")
	}
	// Minimum-viable DeploymentConfig for slice 1a: one container named
	// after the component, running the operator-supplied image, with the
	// plugin-supplied env (if any). IngressContainer points at it so the
	// compose service is reachable.
	appContainer := k8s.CloudRunContainer{
		Name:  componentResourceName,
		Image: api.ContainerImage{Name: image},
	}
	deployment := k8s.DeploymentConfig{
		StackConfig: &api.StackConfigCompose{
			Env: envMap,
		},
		Containers:       []k8s.CloudRunContainer{appContainer},
		IngressContainer: &appContainer,
	}
	merged := &vm.VmComposeInput{
		ComposeTemplateConfig: *tplCfg,
		Deployment:            deployment,
		SingleImage:           true,
	}
	return &api.ResourceDescriptor{
		Type: template.Type,
		Name: componentResourceName,
		Config: api.Config{
			Config: merged,
		},
		Inherit: template.Inherit,
	}, nil
}

func init() {
	pApi.RegisterComponentTemplateMergeFunc(vm.TemplateTypeVmCompose, componentMergeVmCompose)
}

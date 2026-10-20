// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// SingleImageService is the compose service name a `type: single-image` stack is
// projected onto. The client config names no service — it has one image — and the
// generated compose file needs a name for it; "app" is what the operator will see in
// `docker ps` and in the Caddy upstream.
const SingleImageService = "app"

// ToVmComposeConfig converts a `vm-compose` template plus a `cloud-compose` client
// stack into the provisioning input.
//
// The container set comes from the SAME k8s.ConvertComposeToContainers the Kubernetes
// path uses, deliberately: `runs`, `env`, `size`, probes and image-build inputs then
// behave identically here and on GKE. What this function adds on top is the three
// things compose expresses and a Kubernetes Deployment does not (mounts by type,
// depends_on, the verbatim healthcheck) and the §2.2 validation.
func ToVmComposeConfig(tpl any, composeCfg compose.Config, stackCfg *api.StackConfigCompose) (any, error) {
	templateCfg, err := toComposeTemplate(tpl)
	if err != nil {
		return nil, err
	}
	if stackCfg == nil {
		return nil, errors.Errorf("stack config cannot be nil")
	}
	if composeCfg.Project == nil {
		return nil, errors.Errorf("docker compose config is nil; `dockerComposeFile` is required for a `%s` stack", api.ClientTypeCloudCompose)
	}

	res := &VmComposeInput{
		ComposeTemplateConfig: *templateCfg,
		Deployment: k8s.DeploymentConfig{
			StackConfig: stackCfg,
			Scale:       k8s.ToScale(stackCfg),
			Headers:     lo.ToPtr(k8s.ToHeaders(stackCfg.Headers)),
			TextVolumes: k8s.ToSimpleTextVolumes(stackCfg),
		},
	}
	if err := res.validateStackConfig(); err != nil {
		return nil, err
	}

	containers, err := k8s.ConvertComposeToContainers(composeCfg, stackCfg)
	if err != nil {
		return nil, err
	}
	iContainer, err := k8s.FindIngressContainer(composeCfg, containers)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect ingress container")
	}
	res.Deployment.Containers = containers
	res.Deployment.IngressContainer = iContainer

	if err := res.collectFromCompose(composeCfg); err != nil {
		return nil, err
	}
	if err := res.finalize(); err != nil {
		return nil, err
	}
	return res, nil
}

// ToVmComposeSingleImageConfig converts a `vm-compose` template plus a `single-image`
// client stack into the provisioning input.
//
// It exists because the first stack this path has to carry (forge-atrium) is
// `single-image`, not `cloud-compose`. The projection is deliberate and total: a
// synthetic StackConfigCompose with one service, so that everything downstream —
// render, env file, Caddy site, blue-green — has exactly one shape to handle. Fields
// that have no meaning on a VM are refused or warned about here, never dropped
// silently.
func ToVmComposeSingleImageConfig(tpl any, stackCfg *api.StackConfigSingleImage) (any, error) {
	templateCfg, err := toComposeTemplate(tpl)
	if err != nil {
		return nil, err
	}
	if stackCfg == nil {
		return nil, errors.Errorf("stack config cannot be nil")
	}
	if stackCfg.Image == nil {
		return nil, errors.Errorf("`image` is required for a `%s` stack", api.ClientTypeSingleImage)
	}

	composeCfg := &api.StackConfigCompose{
		Domain:      stackCfg.Domain,
		BaseDnsZone: stackCfg.BaseDnsZone,
		Uses:        stackCfg.Uses,
		Runs:        []string{SingleImageService},
		Env:         stackCfg.Env,
		Secrets:     stackCfg.Secrets,
		Version:     stackCfg.Version,
		// basePath is the path the application itself serves under, so the prefix has
		// to survive the proxy hop — which is exactly `proxyKeepPrefix: true`.
		Prefix:          strings.TrimSuffix(stackCfg.BasePath, "/"),
		ProxyKeepPrefix: stackCfg.BasePath != "",
		Dependencies:    stackCfg.Dependencies,
		CloudExtras:     stackCfg.CloudExtras,
		StaticEgressIP:  stackCfg.StaticEgressIP,
		Security:        stackCfg.Security,
	}
	if stackCfg.Min != 0 || stackCfg.Max != 0 {
		composeCfg.Scale = &api.StackConfigComposeScale{Min: stackCfg.Min, Max: stackCfg.Max}
	}
	if m := lo.FromPtr(stackCfg.MaxMemory); m > 0 {
		// maxMemory is MiB on every target that has it (Lambda, YC container), and
		// `size.memory` is a bare MB number — k8s.toMemoryLimit parses it with
		// strconv.Atoi and multiplies by 1024² (k8s/types.go:342-348). A Kubernetes
		// quantity here ("1024Mi") fails conversion with a strconv error, which reads
		// like a malformed client.yaml.
		composeCfg.Size = &api.StackConfigComposeSize{Memory: fmt.Sprintf("%d", m)}
	}

	res := &VmComposeInput{
		ComposeTemplateConfig: *templateCfg,
		SingleImage:           true,
		Deployment: k8s.DeploymentConfig{
			StackConfig: composeCfg,
			Scale:       k8s.ToScale(composeCfg),
		},
	}

	resources, err := k8s.ToResources(composeCfg, types.ServiceConfig{Name: SingleImageService})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert resources")
	}
	container := k8s.CloudRunContainer{
		Name:      SingleImageService,
		Image:     *stackCfg.Image,
		Resources: resources,
		Ports:     []k8s.ContainerPort{{Port: DefaultSingleImagePort, Protocol: "tcp"}},
		MainPort:  lo.ToPtr(DefaultSingleImagePort),
		// PORT is how both Lambda and YC Serverless Container tell a container which
		// port to listen on, so an image that already runs on either needs no change.
		Env: map[string]string{EnvPort: fmt.Sprintf("%d", DefaultSingleImagePort)},
	}
	if container.Image.Platform == "" {
		container.Image.Platform = api.ImagePlatformLinuxAmd64
	}
	res.Deployment.Containers = []k8s.CloudRunContainer{container}
	res.Deployment.IngressContainer = lo.ToPtr(container)

	if err := res.validateStackConfig(); err != nil {
		return nil, err
	}
	if stackCfg.Timeout != nil {
		res.warn("`timeout: %d` is not supported on a VM and is ignored: it is a request timeout "+
			"of an API gateway in front of a function, and there is no gateway here. Set a timeout in "+
			"the application, or use `lbConfig` once P5.3 exposes Caddy's.", *stackCfg.Timeout)
	}
	if stackCfg.MaxEphemeralStorage != nil {
		res.warn("`maxEphemeralStorage: %d` is not supported on a VM and is ignored: a container's "+
			"writable layer is bounded by the machine's disk, not per-container. Mount a tmpfs or a "+
			"named volume if the application needs a bounded scratch area.", *stackCfg.MaxEphemeralStorage)
	}
	if err := res.finalize(); err != nil {
		return nil, err
	}
	return res, nil
}

func toComposeTemplate(tpl any) (*ComposeTemplateConfig, error) {
	templateCfg, ok := tpl.(*ComposeTemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type *vm.ComposeTemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}
	if err := templateCfg.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid `%s` template config", TemplateTypeVmCompose)
	}
	return templateCfg, nil
}

// validateStackConfig enforces the §2.2 table of compose-on-vm.md. Its governing rule:
// a field is either supported or refused. A field accepted and silently ignored is a
// future incident where the operator is certain the setting applied.
func (i *VmComposeInput) validateStackConfig() error {
	cfg := i.Deployment.StackConfig

	if cfg.DomainProxied != nil {
		return errors.Errorf(
			"`domainProxied` is not supported by the `%s` template: it proxies the domain through "+
				"Cloudflare, and a VM serves TLS itself (Caddy obtains the certificate via ACME, which a "+
				"proxied domain breaks). Remove the field, or put the machine behind Cloudflare outside SC",
			TemplateTypeVmCompose,
		)
	}
	if cfg.ClusterIPAddress != nil {
		return errors.Errorf(
			"`clusterIPAddress` is not applicable to the `%s` template: there is no cluster. The stack is "+
				"reachable at the machine's address", TemplateTypeVmCompose,
		)
	}
	if s := cfg.Scale; s != nil {
		if s.Max > 1 {
			return errors.Errorf(
				"`scale: {min: %d, max: %d}` is not supported by the `%s` template: one machine runs one "+
					"replica of each service. Use `scale: {min: 1, max: 1}`, or move the stack to a k3s/Kubernetes "+
					"parent, which is what horizontal scaling needs", s.Min, s.Max, TemplateTypeVmCompose,
			)
		}
		if s.Min == 0 {
			i.warn("`scale.min: 0` has no effect on a VM: containers are started with `restart: always` " +
				"and stay up. There is no scale-to-zero on this delivery path.")
		}
		if s.Policy != nil {
			i.warn("`scale.policy` is ignored on a VM: autoscaling needs more than one node to scale onto.")
		}
	}
	if lo.FromPtr(cfg.StaticEgressIP) {
		i.warn("`staticEgressIP: true` is already true on a VM — outbound traffic leaves from the " +
			"machine's own address. Nothing is provisioned for it.")
	}
	if cfg.ImagePullPolicy != nil && i.EffectiveImageDelivery() == ImageDeliverySshTar {
		i.warn("`imagePullPolicy: %s` is ignored with `imageDelivery: %s`: the image is streamed over "+
			"SSH and loaded into the machine's docker, never pulled. Use `imageDelivery: %s` for the "+
			"policy to mean anything.", lo.FromPtr(cfg.ImagePullPolicy), ImageDeliverySshTar, ImageDeliveryRegistry)
	}
	if cfg.CloudExtras != nil {
		i.warn("`cloudExtras` is ignored by the `%s` template: its keys are Kubernetes-specific "+
			"(nodeSelector, affinity, VPA, disruption budgets) and have no counterpart on a single "+
			"machine.", TemplateTypeVmCompose)
	}
	if cfg.Alerts != nil {
		i.warn("`alerts` is not wired on the `%s` template yet; the block is accepted so that moving a "+
			"stack between parents needs no edit, but nothing is provisioned from it.", TemplateTypeVmCompose)
	}
	return nil
}

// collectFromCompose pulls the three things out of the customer's compose file that
// k8s.DeploymentConfig cannot carry. It walks ONLY the services in `runs` — the same
// set ConvertComposeToContainers walked — so an unused service in the file cannot
// introduce a mount or a dependency.
func (i *VmComposeInput) collectFromCompose(composeCfg compose.Config) error {
	runs := lo.SliceToMap(i.Deployment.StackConfig.Runs, func(s string) (string, bool) { return s, true })
	services := lo.Associate(composeCfg.Project.Services, func(svc types.ServiceConfig) (string, types.ServiceConfig) {
		return svc.Name, svc
	})

	for _, name := range i.Deployment.StackConfig.Runs {
		svc, found := services[name]
		if !found {
			// ConvertComposeToContainers already refused this; keep the guard anyway
			// so collectFromCompose is safe to call on its own.
			return errors.Errorf("service %s not found in docker-compose config", name)
		}
		i.carryImage(svc)

		mounts, err := toMounts(svc)
		if err != nil {
			return err
		}
		i.Mounts = append(i.Mounts, mounts...)

		if deps := toDependsOn(svc, runs); len(deps) > 0 {
			if i.DependsOn == nil {
				i.DependsOn = map[string][]VmDependsOn{}
			}
			i.DependsOn[name] = deps
		}
		for _, dep := range lo.Keys(svc.DependsOn) {
			if !runs[dep] {
				i.warn("service %q depends_on %q, which is not in `runs` and will not be started; "+
					"the dependency is dropped.", name, dep)
			}
		}

		if hc := toHealthCheck(svc.HealthCheck); hc != nil {
			if i.HealthChecks == nil {
				i.HealthChecks = map[string]VmHealthCheck{}
			}
			i.HealthChecks[name] = *hc
		}
	}
	return nil
}

// carryImage copies a service's `image:` onto its container.
//
// ConvertComposeToContainers drops it: the Kubernetes path assumes every container is
// built from a Dockerfile, and for a service with no build context it substitutes the
// SERVICE NAME as the image reference (pulumi/kubernetes/images_build.go:36-41) — which
// only resolves when the two happen to coincide, and silently deploys a nonexistent
// image when they don't (`mongodb: {image: mongo:7}` becomes image `mongodb`).
//
// A generated compose file cannot paper over that: the machine needs a reference it can
// actually pull. So a service with no build context keeps its `image:`, and a service
// WITH one keeps an empty name on purpose — the deploy must pass the reference it built
// (RenderOptions.Images), and Render refuses rather than letting compose build on the
// machine.
func (i *VmComposeInput) carryImage(svc types.ServiceConfig) {
	if svc.Build != nil || svc.Image == "" {
		return
	}
	for idx := range i.Deployment.Containers {
		if i.Deployment.Containers[idx].Name == svc.Name {
			i.Deployment.Containers[idx].Image.Name = svc.Image
			if i.Deployment.IngressContainer != nil && i.Deployment.IngressContainer.Name == svc.Name {
				i.Deployment.IngressContainer.Image.Name = svc.Image
			}
			return
		}
	}
}

func toMounts(svc types.ServiceConfig) ([]VmMount, error) {
	var res []VmMount
	for _, v := range svc.Volumes {
		switch v.Type {
		case types.VolumeTypeVolume:
			if v.Source == "" {
				return nil, errors.Errorf(
					"service %q mounts an anonymous volume at %q, which a VM deploy cannot keep: the slot "+
						"that owns it is torn down on the next deploy and the data goes with it. Give the volume "+
						"a name in the compose file's top-level `volumes:`", svc.Name, v.Target,
				)
			}
			res = append(res, VmMount{
				Service: svc.Name, Type: MountTypeVolume,
				Name: v.Source, MountPath: v.Target, ReadOnly: v.ReadOnly,
			})
		case types.VolumeTypeTmpfs:
			m := VmMount{Service: svc.Name, Type: MountTypeTmpfs, MountPath: v.Target}
			if v.Tmpfs != nil && v.Tmpfs.Size > 0 {
				m.Size = fmt.Sprintf("%d", int64(v.Tmpfs.Size))
			}
			res = append(res, m)
		case types.VolumeTypeBind:
			return nil, errors.Errorf(
				"service %q bind-mounts %q into %q, which cannot work on a VM: the source is a path on the "+
					"machine that ran `sc deploy`, not on the target. Use `textVolumes` in client.yaml for "+
					"configuration files, or a named volume for data", svc.Name, v.Source, v.Target,
			)
		default:
			return nil, errors.Errorf(
				"service %q mounts an unsupported volume type %q at %q; the `%s` template supports %s and %s",
				svc.Name, v.Type, v.Target, TemplateTypeVmCompose, MountTypeVolume, MountTypeTmpfs,
			)
		}
	}
	return res, nil
}

func toDependsOn(svc types.ServiceConfig, runs map[string]bool) []VmDependsOn {
	deps := lo.FilterMap(lo.Keys(svc.DependsOn), func(name string, _ int) (VmDependsOn, bool) {
		return VmDependsOn{Service: name, Condition: svc.DependsOn[name].Condition}, runs[name]
	})
	sort.Slice(deps, func(a, b int) bool { return deps[a].Service < deps[b].Service })
	return deps
}

func toHealthCheck(hc *types.HealthCheckConfig) *VmHealthCheck {
	if hc == nil || hc.Disable || len(hc.Test) == 0 {
		return nil
	}
	res := &VmHealthCheck{
		Test:        hc.Test,
		Interval:    durationOrEmpty(hc.Interval),
		Timeout:     durationOrEmpty(hc.Timeout),
		StartPeriod: durationOrEmpty(hc.StartPeriod),
	}
	if hc.Retries != nil {
		res.Retries = lo.ToPtr(int(*hc.Retries))
	}
	return res
}

func durationOrEmpty(d *types.Duration) string {
	if d == nil {
		return ""
	}
	return time.Duration(*d).String()
}

// finalize applies the checks that need the whole picture: that an ingress exists when
// a domain was asked for, that the ingress service can be health-gated, and that
// blue-green is actually possible for this service set.
func (i *VmComposeInput) finalize() error {
	cfg := i.Deployment.StackConfig
	ingress := i.Deployment.IngressContainer

	if ingress == nil {
		if cfg.Domain != "" {
			return errors.Errorf(
				"`domain: %s` is set but no ingress container was found: label the service that serves "+
					"HTTP with `%s: \"true\"` (and `%s: \"<port>\"` if it exposes several), or drop `domain`",
				cfg.Domain, api.ComposeLabelIngressContainer, api.ComposeLabelIngressPort,
			)
		}
		i.warn("no ingress container: nothing on this stack will be reachable from outside the machine. " +
			"That is correct for a worker-only stack.")
	} else if ingress.MainPort == nil {
		return errors.Errorf(
			"ingress container %q has no port to proxy to: give the service a `ports:` entry, or set the "+
				"`%s` label", ingress.Name, api.ComposeLabelIngressPort,
		)
	}

	// §6 rule 2: the ingress service's healthcheck is the blue-green gate. A
	// synthesized TCP check is weak — it proves the port is open, not that the
	// application works — but flipping Caddy onto an upstream nobody checked is worse.
	if ingress != nil {
		if _, ok := i.HealthChecks[ingress.Name]; !ok {
			if i.HealthChecks == nil {
				i.HealthChecks = map[string]VmHealthCheck{}
			}
			port := lo.FromPtr(ingress.MainPort)
			i.HealthChecks[ingress.Name] = VmHealthCheck{
				// nc is in busybox and in debian-slim's netcat-openbsd, but NOT in a
				// distroless or scratch image. The check then fails, the deploy is
				// gated and the operator is told to declare a real one — which is the
				// right failure direction.
				//
				// The addresses are literals, and BOTH of them, because `localhost` is
				// a resolver question with the wrong answer: in an Alpine container
				// `getent hosts localhost` returns `::1` first, so `nc -z localhost`
				// dials [::1] — while `listen 8080;` in nginx, and every other
				// IPv4-only bind, is not there. The gate then never passes, for a
				// container that is serving perfectly well on 0.0.0.0. v4 first
				// because it is the common case; the v6 arm costs one failed connect
				// and catches a service bound only to the v6 loopback.
				//
				// Live-caught 2026-10-18 with nginx on a byo host: 120 s of `running /
				// unhealthy`, then a rolled-back deploy, with the container answering
				// on 127.0.0.1:8080 the whole time.
				Test: []string{"CMD-SHELL", fmt.Sprintf(
					"nc -z 127.0.0.1 %d || nc -z ::1 %d || exit 1", port, port)},
				Interval:    "10s",
				Timeout:     "3s",
				StartPeriod: "20s",
				Retries:     lo.ToPtr(3),
				Synthesized: true,
			}
			i.warn("service %q serves ingress but declares no `healthcheck`, so SC synthesized a TCP check "+
				"on port %d. It gates the blue-green switch, and it only proves the port is open — declare a "+
				"real healthcheck in the compose file for the gate to mean anything.", ingress.Name, port)
		}
	}

	// §9.1: a service with a writable named volume must not run in two slots at once.
	stateful := i.StatefulServices()
	if len(stateful) > 0 && i.EffectiveStrategy() == StrategyBlueGreen {
		if ingress != nil && lo.Contains(stateful, ingress.Name) {
			return errors.Errorf(
				"service %q both serves ingress and writes to a named volume (%s), so `strategy: %s` cannot "+
					"work: the flip needs two copies of the ingress service running at once, and two writers on "+
					"one volume corrupt it. Use `strategy: %s` (a few seconds of downtime per deploy), or split "+
					"the state into its own service",
				ingress.Name, strings.Join(lo.Map(i.MountsOf(ingress.Name), func(m VmMount, _ int) string {
					return m.Name
				}), ", "), StrategyBlueGreen, StrategyRecreate,
			)
		}
		i.warn("services %s write to named volumes, so they run outside the blue-green slots in project "+
			"`<stack>-<env>-data` and are NOT redeployed by a flip. A change to their image or env needs "+
			"`strategy: %s`.", strings.Join(quoteAll(stateful), ", "), StrategyRecreate)
	}

	for _, c := range i.Deployment.Containers {
		for _, w := range c.Warnings {
			i.warn("container %q: %s", c.Name, w)
		}
	}
	return nil
}

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

func renderOpts() RenderOptions {
	return RenderOptions{
		Stack:   "refapp",
		Env:     "staging",
		Slot:    SlotA,
		Version: "2026.10.16-abc1234",
		Images: map[string]string{
			"backend": "cr.cloud.ru/refapp/backend:2026.10.16-abc1234",
			"worker":  "cr.cloud.ru/refapp/worker:2026.10.16-abc1234",
		},
	}
}

// parsed is the generated file read back as compose would read it. Asserting on the
// parsed tree rather than on substrings is what keeps the test from passing on a file
// that is textually right and syntactically wrong.
func parsed(t *testing.T, content string) map[string]any {
	t.Helper()
	var out map[string]any
	Expect(yaml.Unmarshal([]byte(content), &out)).To(Succeed())
	return out
}

func service(t *testing.T, content, name string) map[string]any {
	t.Helper()
	services, ok := parsed(t, content)["services"].(map[string]any)
	Expect(ok).To(BeTrue())
	svc, ok := services[name].(map[string]any)
	Expect(ok).To(BeTrue(), "service %q is not in the rendered file:\n%s", name, content)
	return svc
}

func renderTestInput(t *testing.T) *VmComposeInput {
	t.Helper()
	got, err := ToVmComposeConfig(testTemplate(), readTestCompose(t), &api.StackConfigCompose{
		Domain: "refapp.example.com",
		Runs:   []string{"backend", "mongo", "worker"},
		Env:    map[string]string{"NODE_ENV": "production"},
	})
	Expect(err).ToNot(HaveOccurred())
	return toInput(got)
}

// The whole point of the two-project split: a writable named volume means the service
// must not be duplicated by a flip, so it goes to a project the flip never touches.
func TestRenderSplitsStatefulIntoItsOwnProject(t *testing.T) {
	RegisterTestingT(t)

	out, err := Render(renderTestInput(t), renderOpts())
	Expect(err).ToNot(HaveOccurred())

	Expect(out.SlotProject).To(Equal("refapp-staging-a"))
	Expect(out.SlotServices).To(ConsistOf("backend", "worker"))
	Expect(out.DataProject).To(Equal("refapp-staging-data"))
	Expect(out.DataServices).To(ConsistOf("mongo"))

	Expect(parsed(t, out.SlotFile)["name"]).To(Equal("refapp-staging-a"))
	Expect(parsed(t, out.DataFile)["name"]).To(Equal("refapp-staging-data"))

	// A stateful container belongs to no slot, so neither its name nor its env carries
	// one — the slot services reach it by its bare compose name.
	mongo := service(t, out.DataFile, "mongo")
	Expect(mongo["container_name"]).To(Equal("refapp-staging-mongo"))
	Expect(mongo["environment"]).ToNot(HaveKey(EnvSlot))
	Expect(mongo["image"]).To(Equal("mongo:7"))

	// Both projects attach to the SAME external internal network. That is what lets
	// `MONGO_URI=mongodb://mongo:27017` keep working across the project boundary, and
	// it is why the network is external rather than a compose `default`.
	for _, file := range []string{out.SlotFile, out.DataFile} {
		networks, ok := parsed(t, file)["networks"].(map[string]any)
		Expect(ok).To(BeTrue())
		internal, ok := networks[networkKeyInternal].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(internal["name"]).To(Equal("refapp-staging-internal"))
		Expect(internal["external"]).To(Equal(true))
	}

	// The named volume is external in whichever project mounts it, so tearing down a
	// slot (`docker compose down -v`) cannot delete the data.
	for _, file := range []string{out.SlotFile, out.DataFile} {
		volumes, ok := parsed(t, file)["volumes"].(map[string]any)
		Expect(ok).To(BeTrue())
		vol, ok := volumes["mongo-data"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(vol["external"]).To(Equal(true))
		Expect(vol["name"]).To(Equal("refapp-staging-mongo-data"))
	}
}

func TestRenderIngressJoinsTheSharedNetworkAlone(t *testing.T) {
	RegisterTestingT(t)

	opts := renderOpts()
	opts.IngressHostPort = IngressHostPort(opts.Stack, opts.Env)
	out, err := Render(renderTestInput(t), opts)
	Expect(err).ToNot(HaveOccurred())

	backend := service(t, out.SlotFile, "backend")
	networks, ok := backend["networks"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(networks).To(HaveKey(networkKeyIngress))
	// The alias carries the slot, which is what makes the flip a one-line change in
	// Caddy's config, and the stack-env prefix, without which two stacks that both
	// call their service `backend` would collide on the shared network.
	ingressNet, ok := networks[networkKeyIngress].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(ingressNet["aliases"]).To(ConsistOf("refapp-staging-backend-a"))
	Expect(backend["container_name"]).To(Equal("refapp-staging-backend-a"))

	// Only the ingress container joins it, so two stacks on one machine cannot resolve
	// each other's `mongo`.
	worker := service(t, out.SlotFile, "worker")
	workerNets, ok := worker["networks"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(workerNets).ToNot(HaveKey(networkKeyIngress))
	Expect(worker).ToNot(HaveKey("ports"))

	// And the slot file declares the shared network as external — it outlives every
	// compose project on the machine.
	networksBlock, ok := parsed(t, out.SlotFile)["networks"].(map[string]any)
	Expect(ok).To(BeTrue())
	shared, ok := networksBlock[networkKeyIngress].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(shared["name"]).To(Equal(IngressNetwork))
}

func TestRenderPublishesIngressPortOnlyWhenAsked(t *testing.T) {
	RegisterTestingT(t)

	input := renderTestInput(t)

	// 0 publishes nothing: with Caddy in front, the service is reachable over the
	// docker network only.
	out, err := Render(input, renderOpts())
	Expect(err).ToNot(HaveOccurred())
	Expect(service(t, out.SlotFile, "backend")).ToNot(HaveKey("ports"))

	// A host port is what makes the P5.2 definition of done checkable before Caddy
	// exists, and what `ingress.enable: false` needs.
	opts := renderOpts()
	opts.IngressHostPort = 20123
	out, err = Render(input, opts)
	Expect(err).ToNot(HaveOccurred())
	Expect(service(t, out.SlotFile, "backend")["ports"]).To(ConsistOf("20123:8080"))
}

func TestRenderServiceBody(t *testing.T) {
	RegisterTestingT(t)

	out, err := Render(renderTestInput(t), renderOpts())
	Expect(err).ToNot(HaveOccurred())
	backend := service(t, out.SlotFile, "backend")

	// The deploy's image reference wins over whatever the conversion produced.
	Expect(backend["image"]).To(Equal("cr.cloud.ru/refapp/backend:2026.10.16-abc1234"))
	// restart: always, not unless-stopped — the machine WILL reboot and the stack has
	// to come back by itself.
	Expect(backend["restart"]).To(Equal("always"))
	// Values live in the env file, never in the generated YAML: this file is read by
	// eye during an incident and may end up in a ticket.
	Expect(backend["env_file"]).To(ConsistOf("/etc/sc/refapp/staging/env"))

	env, ok := backend["environment"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(env[EnvCloud]).To(Equal(EnvCloudVm))
	Expect(env[EnvSlot]).To(Equal(SlotA))
	Expect(env[EnvDeployment]).To(Equal("2026.10.16-abc1234"))

	// Unbounded container logs filling the disk is the most common way a long-lived VM
	// dies, and it presents as the application failing rather than as a full disk.
	logging, ok := backend["logging"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(logging["driver"]).To(Equal("json-file"))
	options, ok := logging["options"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(options["max-size"]).To(Equal(LogMaxSize))
	Expect(options["max-file"]).To(Equal(LogMaxFile))

	// The healthcheck is the blue-green gate, so it must survive into the file as the
	// test command the operator wrote.
	hc, ok := backend["healthcheck"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(hc["test"]).To(ConsistOf("CMD", "curl", "-f", "http://localhost:8080/health"))
	Expect(hc["interval"]).To(Equal("15s"))
	Expect(hc["retries"]).To(Equal(4))

	Expect(backend["tmpfs"]).To(ConsistOf("/scratch:size=67108864"))
	depends, ok := backend["depends_on"].(map[string]any)
	Expect(ok).To(BeTrue())
	mongoDep, ok := depends["mongo"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(mongoDep["condition"]).To(Equal("service_healthy"))

	// A read-only mount of the same named volume renders with :ro.
	Expect(service(t, out.SlotFile, "worker")["volumes"]).To(ConsistOf("mongo-data:/data/db:ro"))
}

// The header is the first thing a human sees in the file they are about to edit during
// an incident, so it has to say that edits do not survive and where the source is.
func TestRenderWritesTheGeneratedHeader(t *testing.T) {
	RegisterTestingT(t)

	out, err := Render(renderTestInput(t), renderOpts())
	Expect(err).ToNot(HaveOccurred())

	Expect(out.SlotFile).To(HavePrefix("# GENERATED BY SIMPLE CONTAINER"))
	Expect(out.SlotFile).To(ContainSubstring("local edits are lost"))
	Expect(out.SlotFile).To(ContainSubstring(".sc/stacks/refapp/client.yaml"))
	Expect(out.SlotFile).To(ContainSubstring("sc deploy -s refapp -e staging"))
	Expect(out.DataFile).To(HavePrefix("# GENERATED BY SIMPLE CONTAINER"))
}

// Byte-stability matters because the file is written to the machine on every deploy: a
// map order that wandered would make every deploy look like a change and every diff
// unreadable.
func TestRenderIsByteStable(t *testing.T) {
	RegisterTestingT(t)

	first, err := Render(renderTestInput(t), renderOpts())
	Expect(err).ToNot(HaveOccurred())
	for i := 0; i < 5; i++ {
		again, err := Render(renderTestInput(t), renderOpts())
		Expect(err).ToNot(HaveOccurred())
		Expect(again.SlotFile).To(Equal(first.SlotFile))
		Expect(again.DataFile).To(Equal(first.DataFile))
	}
}

func TestRenderSingleImageStack(t *testing.T) {
	RegisterTestingT(t)

	got, err := ToVmComposeSingleImageConfig(testTemplate(), &api.StackConfigSingleImage{
		Image:     &api.ContainerImage{Name: "simplecontainer/atrium:2026.10.16-abc1234"},
		Domain:    "atriumdev.ru",
		MaxMemory: lo.ToPtr(512),
	})
	Expect(err).ToNot(HaveOccurred())

	out, err := Render(toInput(got), RenderOptions{
		Stack: "atrium", Env: "staging-ru", Slot: SlotB,
		Version: "2026.10.16-abc1234", IngressHostPort: 20500,
	})
	Expect(err).ToNot(HaveOccurred())

	// One service, no data project, and the image comes from client.yaml rather than
	// from a build — nothing in RenderOptions.Images.
	Expect(out.SlotServices).To(ConsistOf(SingleImageService))
	Expect(out.DataProject).To(BeEmpty())
	Expect(out.DataFile).To(BeEmpty())

	app := service(t, out.SlotFile, SingleImageService)
	Expect(app["image"]).To(Equal("simplecontainer/atrium:2026.10.16-abc1234"))
	Expect(app["container_name"]).To(Equal("atrium-staging-ru-app-b"))
	Expect(app["ports"]).To(ConsistOf("20500:8080"))

	env, ok := app["environment"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(env[EnvPort]).To(Equal("8080"))
	Expect(env[EnvSlot]).To(Equal(SlotB))

	// maxMemory reached the container as a compose limit, in compose's units.
	deploy, ok := app["deploy"].(map[string]any)
	Expect(ok).To(BeTrue())
	resources, ok := deploy["resources"].(map[string]any)
	Expect(ok).To(BeTrue())
	limits, ok := resources["limits"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(limits["memory"]).To(Equal("512m"))
}

func TestRenderRefusals(t *testing.T) {
	RegisterTestingT(t)

	input := renderTestInput(t)

	_, err := Render(nil, renderOpts())
	Expect(err).To(MatchError(ContainSubstring("input is nil")))

	badNames := renderOpts()
	badNames.Stack = "RefApp"
	_, err = Render(input, badNames)
	Expect(err).To(MatchError(ContainSubstring(`stack name "RefApp"`)))

	badSlot := renderOpts()
	badSlot.Slot = "green"
	_, err = Render(input, badSlot)
	Expect(err).To(MatchError(ContainSubstring(`unknown slot "green"`)))

	// A built service with no reference from the deploy would make compose try to
	// build on the machine, which needs the source tree that is not there.
	noImage := renderOpts()
	noImage.Images = nil
	_, err = Render(input, noImage)
	Expect(err).To(MatchError(ContainSubstring("a compose file on the machine must never build")))
}

// k8s says `250m` CPU and `512Mi` memory; compose says `0.25` and `512m`. Rendering one
// as the other yields either a parse error or a limit 1024× off.
func TestResourceUnitConversion(t *testing.T) {
	RegisterTestingT(t)

	Expect(toComposeCPUs("250m")).To(Equal("0.25"))
	Expect(toComposeCPUs("2000m")).To(Equal("2"))
	Expect(toComposeCPUs("1500m")).To(Equal("1.5"))
	Expect(toComposeCPUs("100m")).To(Equal("0.1"))
	// A value already in cores passes through.
	Expect(toComposeCPUs("2")).To(Equal("2"))
	Expect(toComposeCPUs("")).To(Equal(""))
	Expect(toComposeCPUs("abcm")).To(Equal(""))

	// Both notations are 1024-based, so only the suffix changes.
	Expect(toComposeMemory("512Mi")).To(Equal("512m"))
	Expect(toComposeMemory("4Gi")).To(Equal("4g"))
	Expect(toComposeMemory("64Ki")).To(Equal("64k"))
	Expect(toComposeMemory("1Ti")).To(Equal("1t"))
	// A plain number is bytes in both; a zero limit means "unset", not "no memory".
	Expect(toComposeMemory("1048576")).To(Equal("1048576"))
	Expect(toComposeMemory("0")).To(Equal(""))
}

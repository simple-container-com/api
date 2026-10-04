// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

func res(placeholder string) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Type:   "gcp-bucket",
		Config: api.Config{Config: map[string]any{"name": placeholder}},
	}
}

func client(parent, template string, cfg any) api.StackClientDescriptor {
	return api.StackClientDescriptor{
		Type: "cloud-compose", ParentStack: parent, Template: template,
		Config: api.Config{Config: cfg},
	}
}

// unresolvedFixture is a parent with client stacks web and batch. Every part the
// deploy does not consume carries an unresolved placeholder of its own.
func unresolvedFixture() (api.StacksMap, map[string]bool) {
	server := api.ServerDescriptor{
		Provisioner: api.ProvisionerDescriptor{Type: "pulumi"},
		CiCd:        api.CiCdDescriptor{Type: "github-actions", Config: api.Config{Config: map[string]any{"token": "${secret:CICD_TOKEN}"}}},
		Templates: map[string]api.StackDescriptor{
			"web-tpl":    {Type: "cloudrun", Config: api.Config{Config: map[string]any{"k": "ok"}}},
			"unused-tpl": {Type: "cloudrun", Config: api.Config{Config: map[string]any{"k": "${secret:UNUSED_TPL}"}}},
			"batch-tpl":  {Type: "cloudrun", Config: api.Config{Config: map[string]any{"k": "${secret:BATCH_TPL}"}}},
		},
		Resources: api.PerStackResourcesDescriptor{
			Resources: map[string]api.PerEnvResourcesDescriptor{
				"staging": {
					Template: "web-tpl",
					Resources: map[string]api.ResourceDescriptor{
						"web-res":   res("plain"),
						"batch-res": res("${secret:BATCH_TOKEN}"),
						"dep-res":   res("${secret:DEP_TOKEN}"),
					},
				},
				"production": {Resources: map[string]api.ResourceDescriptor{"prod-res": res("${secret:PROD_TOKEN}")}},
			},
		},
	}
	stacks := api.StacksMap{
		"infra": {Name: "infra", Server: server},
		"web": {Name: "web", Server: server, Client: api.ClientDescriptor{Stacks: map[string]api.StackClientDescriptor{
			"staging": client("acme/infra", "", &api.StackConfigCompose{Uses: []string{"web-res"}}),
		}}},
		"batch": {Name: "batch", Server: server, Client: api.ClientDescriptor{Stacks: map[string]api.StackClientDescriptor{
			"staging": client("acme/infra", "", &api.StackConfigCompose{Uses: []string{"batch-res"}}),
		}}},
	}
	return stacks, map[string]bool{"infra": true}
}

func checkFor(t *testing.T, stacks api.StacksMap, scopedOnly map[string]bool, stack string) (error, *captureLogger) {
	t.Helper()
	log := &captureLogger{}
	p := &provisioner{stacks: stacks, scopedOnly: scopedOnly, log: log}
	err := p.checkUnresolvedPlaceholders(context.Background(), api.StackParams{StackName: stack, Environment: "staging"})
	return err, log
}

func setClient(stacks api.StacksMap, stack string, desc api.StackClientDescriptor) {
	s := stacks[stack]
	s.Client = api.ClientDescriptor{Stacks: map[string]api.StackClientDescriptor{"staging": desc}}
	stacks[stack] = s
}

func Test_checkUnresolved_SiblingResourceDoesNotBlockDeploy(t *testing.T) {
	RegisterTestingT(t)
	stacks, scoped := unresolvedFixture()

	err, _ := checkFor(t, stacks, scoped, "web")
	Expect(err).NotTo(HaveOccurred(), "web does not use batch-res, the unfillable CI/CD, template or production placeholders")

	err, _ = checkFor(t, stacks, scoped, "batch")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${secret:BATCH_TOKEN}"))
	Expect(err.Error()).NotTo(ContainSubstring("CICD_TOKEN"))
	Expect(err.Error()).NotTo(ContainSubstring("PROD_TOKEN"))
}

func Test_checkUnresolved_OnlyTheSelectedTemplateIsScanned(t *testing.T) {
	RegisterTestingT(t)
	stacks, scoped := unresolvedFixture()

	// Default template of the parent environment (web-tpl) is clean; the unused
	// ones are ignored.
	err, _ := checkFor(t, stacks, scoped, "web")
	Expect(err).NotTo(HaveOccurred())

	// An explicit client template wins over the environment default.
	setClient(stacks, "web", client("acme/infra", "batch-tpl", &api.StackConfigCompose{}))
	err, _ = checkFor(t, stacks, scoped, "web")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${secret:BATCH_TPL}"))
	Expect(err.Error()).NotTo(ContainSubstring("UNUSED_TPL"))

	// With no client template the environment default is the one scanned.
	env := stacks["web"].Server.Resources.Resources["staging"]
	env.Template = "unused-tpl"
	stacks["web"].Server.Resources.Resources["staging"] = env
	setClient(stacks, "web", client("acme/infra", "", &api.StackConfigCompose{}))
	err, _ = checkFor(t, stacks, scoped, "web")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${secret:UNUSED_TPL}"))
	Expect(err.Error()).NotTo(ContainSubstring("BATCH_TPL"))
}

func Test_checkUnresolved_PluralRegistrar(t *testing.T) {
	RegisterTestingT(t)
	stacks, scoped := unresolvedFixture()
	srv := stacks["web"].Server
	srv.Resources.Registrars = map[string]api.RegistrarDescriptor{
		"cf": {Type: "cloudflare", Config: api.Config{Config: map[string]any{"apiToken": "${secret:CF_TOKEN}"}}},
	}
	s := stacks["web"]
	s.Server = srv
	stacks["web"] = s

	err, _ := checkFor(t, stacks, scoped, "web")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${secret:CF_TOKEN}"))
}

func Test_checkUnresolved_DependenciesAreScanned(t *testing.T) {
	RegisterTestingT(t)
	stacks, scoped := unresolvedFixture()
	setClient(stacks, "web", client("acme/infra", "", &api.StackConfigCompose{
		Uses:         []string{"web-res"},
		Dependencies: []api.StackConfigDependencyResource{{Name: "d", Owner: "other", Resource: "dep-res"}},
	}))

	err, _ := checkFor(t, stacks, scoped, "web")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${secret:DEP_TOKEN}"))
}

func Test_checkUnresolved_ProvisionerAndMissingResourceName(t *testing.T) {
	RegisterTestingT(t)
	stacks, scoped := unresolvedFixture()
	setClient(stacks, "web", client("acme/infra", "", &api.StackConfigCompose{Uses: []string{"no-such-resource"}}))
	err, _ := checkFor(t, stacks, scoped, "web")
	Expect(err).NotTo(HaveOccurred(), "a resource missing from the map must not panic or fail the check")

	s := stacks["web"]
	s.Server.Provisioner = api.ProvisionerDescriptor{Type: "pulumi", Config: api.Config{Config: map[string]any{"key": "${auth:gcloud}"}}}
	stacks["web"] = s
	err, _ = checkFor(t, stacks, scoped, "web")
	Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "err = %v", err)
	Expect(err.Error()).To(ContainSubstring("${auth:gcloud}"))
}

func Test_checkUnresolved_ErrorListsNamesSorted(t *testing.T) {
	RegisterTestingT(t)
	stacks, scoped := unresolvedFixture()
	setClient(stacks, "web", client("acme/infra", "", &api.StackConfigCompose{
		Uses: []string{"web-res", "dep-res", "batch-res"},
	}))
	err, _ := checkFor(t, stacks, scoped, "web")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("${secret:BATCH_TOKEN}, ${secret:DEP_TOKEN}"))
}

func Test_checkUnresolved_NotScopedOnlyWarns(t *testing.T) {
	RegisterTestingT(t)
	stacks, _ := unresolvedFixture()

	err, log := checkFor(t, stacks, map[string]bool{}, "batch")
	Expect(err).NotTo(HaveOccurred())
	Expect(log.warns).To(HaveLen(1))
	Expect(log.warns[0]).To(ContainSubstring("${secret:BATCH_TOKEN}"))

	err, log = checkFor(t, stacks, map[string]bool{}, "web")
	Expect(err).NotTo(HaveOccurred())
	Expect(log.warns).To(BeEmpty())
}

func Test_ParentStackName_SharedByReconcileAndCheck(t *testing.T) {
	RegisterTestingT(t)
	for ref, want := range map[string]string{"infra": "infra", "acme/infra": "infra", "acme/proj/infra": "infra"} {
		Expect(api.ParentStackName(ref)).To(Equal(want), ref)

		stacks, scoped := unresolvedFixture()
		setClient(stacks, "batch", client(ref, "", &api.StackConfigCompose{Uses: []string{"batch-res"}}))
		// the scopedOnly lookup uses the same parsing as the reconcile step
		err, _ := checkFor(t, stacks, scoped, "batch")
		Expect(errors.Is(err, ErrUnresolvedPlaceholders)).To(BeTrue(), "ref %q: %v", ref, err)

		reconciled, err := stacks.ReconcileForDeploy(api.StackParams{StackName: "batch", Environment: "staging"})
		Expect(err).NotTo(HaveOccurred(), ref)
		Expect(strings.TrimSpace((*reconciled)["batch"].Server.Provisioner.Type)).To(Equal("pulumi"))
	}
}

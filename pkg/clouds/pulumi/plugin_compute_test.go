// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// A plugin compute processor's env and placeholders used to be discarded, so a
// plugin resource could give a `uses:` consumer nothing.
func TestApplyComputeProcessResponse_FillsCollector(t *testing.T) {
	RegisterTestingT(t)

	collector := pApi.NewComputeContextCollector(context.Background(), logger.New(), "site", "prod")
	resp := &scpluginv1.ComputeProcessResponse{
		Env: map[string]*scpluginv1.InputValue{
			"PGHOST":     {Value: &scpluginv1.InputValue_StringValue{StringValue: "c-1.rw.mdb.yandexcloud.net"}},
			"PGPASSWORD": {Value: &scpluginv1.InputValue_StringValue{StringValue: "s3cret"}, Secret: true},
		},
		Placeholders: map[string]string{"host": "c-1.rw.mdb.yandexcloud.net"},
	}
	desc := &api.ResourceDescriptor{Type: "yc-postgres", Name: "main-db"}
	applyComputeProcessResponse(resp, collector, desc, &pApi.ParentInfo{StackName: "clients"})

	plain := lo.SliceToMap(collector.EnvVariables(), func(v pApi.ComputeEnvVariable) (string, string) { return v.Name, v.Value })
	Expect(plain).To(Equal(map[string]string{"PGHOST": "c-1.rw.mdb.yandexcloud.net"}))
	secret := collector.SecretEnvVariables()
	Expect(secret).To(HaveLen(1))
	Expect(secret[0].Name).To(Equal("PGPASSWORD"))
	Expect(secret[0].ResourceType).To(Equal("yc-postgres"))
	Expect(secret[0].StackName).To(Equal("clients"))

	// Placeholders resolve as ${resource:main-db.host}.
	cfg := map[string]any{"x": "${resource:main-db.host}"}
	var anyCfg any = cfg
	Expect(collector.ResolvePlaceholders(&anyCfg)).To(Succeed())

	// Nil-safe.
	applyComputeProcessResponse(nil, collector, desc, nil)
	applyComputeProcessResponse(resp, nil, desc, nil)
}

func TestParentInfoToDependencyOutputs(t *testing.T) {
	RegisterTestingT(t)

	Expect(parentInfoToDependencyOutputs(nil)).To(BeNil())
	out := parentInfoToDependencyOutputs(&pApi.ParentInfo{
		StackName:         "clients",
		ParentEnv:         "prod",
		FullReference:     "org/clients/prod",
		UsesResource:      true,
		DependsOnResource: &api.StackConfigDependencyResource{Name: "main-db"},
	})
	Expect(out[DepOutParentFullReference]).To(Equal("org/clients/prod"))
	Expect(out[DepOutUsesResource]).To(Equal("true"))
	Expect(out[DepOutDependsOnResource]).To(Equal("main-db"))
}

// The consuming stack's env reaches a plugin compute template (a YC container)
// with secrets still flagged.
func TestComputeEnvToInput(t *testing.T) {
	RegisterTestingT(t)

	Expect(computeEnvToInput(nil)).To(BeNil())
	collector := pApi.NewComputeContextCollector(context.Background(), logger.New(), "site", "prod")
	Expect(computeEnvToInput(collector)).To(BeNil(), "nothing to forward")

	collector.AddEnvVariableIfNotExist("PGHOST", "h", "yc-postgres", "main-db", "clients")
	collector.AddSecretEnvVariableIfNotExist("PGPASSWORD", "p", "yc-postgres", "main-db", "clients")
	plain, ok := scPlugin.InputValueToPlain(computeEnvToInput(collector))
	Expect(ok).To(BeTrue())
	Expect(plain).To(ConsistOf(
		HaveKeyWithValue("name", "PGHOST"),
		SatisfyAll(HaveKeyWithValue("name", "PGPASSWORD"), HaveKeyWithValue("secret", true)),
	))
}

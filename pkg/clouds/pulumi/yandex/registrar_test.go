// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
)

// TestRegistrarIsRegistered pins the wiring. A registrar needs BOTH tiers registered
// under the SAME type string — the config reader in clouds/yandex and the provisioner
// here — and a mismatch fails only at deploy time, as "unsupported registrar type".
func TestRegistrarIsRegistered(t *testing.T) {
	RegisterTestingT(t)

	Expect(pApi.RegistrarFuncByType).To(HaveKey(yandex.RegistrarTypeYandexDns))
	Expect(api.GetRegisteredProviderConfigs()).To(HaveKey(yandex.RegistrarTypeYandexDns))
	// schema-gen's guessProviderFromResourceType tests the AWS token set before yc/yandex.
	for _, awsToken := range []string{"aws", "s3", "ecr", "rds", "ecs", "lambda", "fargate"} {
		Expect(yandex.RegistrarTypeYandexDns).ToNot(ContainSubstring(awsToken))
	}
}

func TestFqdn(t *testing.T) {
	RegisterTestingT(t)

	Expect(fqdn("ycdemo.simple-forge.ru")).To(Equal("ycdemo.simple-forge.ru."))
	Expect(fqdn("ycdemo.simple-forge.ru.")).To(Equal("ycdemo.simple-forge.ru."))
}

// TestGuardZoneInfrastructureRecord covers the two record shapes a deploy must never
// write. Both fail silently rather than loudly: overwriting NS takes the zone off the
// air, and overwriting the ACME challenge breaks renewal up to 90 days later.
func TestGuardZoneInfrastructureRecord(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name      string
		record    api.DnsRecord
		expectErr string
	}{
		{"plain CNAME is fine", api.DnsRecord{Name: "ycdemo.simple-forge.ru", Type: "CNAME"}, ""},
		{"TXT is fine", api.DnsRecord{Name: "simple-forge.ru", Type: "TXT"}, ""},
		{"NS is refused", api.DnsRecord{Name: "simple-forge.ru", Type: "NS"}, "belong to the zone itself"},
		{"lowercase ns is refused", api.DnsRecord{Name: "simple-forge.ru", Type: "ns"}, "belong to the zone itself"},
		{"SOA is refused", api.DnsRecord{Name: "simple-forge.ru", Type: "SOA"}, "belong to the zone itself"},
		{
			"acme challenge is refused",
			api.DnsRecord{Name: "_acme-challenge.simple-forge.ru", Type: "CNAME"},
			"Certificate Manager owns",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := guardZoneInfrastructureRecord(tc.record)
			if tc.expectErr == "" {
				Expect(err).ToNot(HaveOccurred())
				return
			}
			Expect(err).To(MatchError(ContainSubstring(tc.expectErr)))
		})
	}
}

// TestApiGatewayName checks the derived name against YC's own rule. A name YC rejects
// surfaces as an opaque provider error several minutes into a deploy.
func TestApiGatewayName(t *testing.T) {
	RegisterTestingT(t)

	name, err := apiGatewayName("ycdemo--smoke--smoke")
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(Equal("ycdemo--smoke--smoke-apigw"))

	// A service name at the very limit must still produce a legal gateway name, and
	// must not end up with a trailing hyphen after truncation.
	long, err := apiGatewayName("a" + strings.Repeat("b", 60) + "-c")
	Expect(err).ToNot(HaveOccurred())
	Expect(len(long)).To(BeNumerically("<=", 63))
	Expect(validateYcResourceName(long)).ToNot(HaveOccurred())

	_, err = apiGatewayName("9starts-with-a-digit")
	Expect(err).To(MatchError(ContainSubstring("not a valid yandex cloud resource name")))
}

// TestProxySpecForwardsEverything pins the two paths. `/{path+}` alone does not match
// the empty path, so a gateway without `/` answers 404 on the service root — the first
// thing any smoke requests.
func TestProxySpecForwardsEverything(t *testing.T) {
	RegisterTestingT(t)

	spec := proxySpecFor("ycdemo-apigw", "bba8nrirdcjgkiereo9c.containers.yandexcloud.net")

	Expect(spec).To(ContainSubstring("\n  /:\n"))
	Expect(spec).To(ContainSubstring("\n  /{path+}:\n"))
	Expect(strings.Count(spec, "x-yc-apigateway-any-method")).To(Equal(2))
	Expect(spec).To(ContainSubstring("url: https://bba8nrirdcjgkiereo9c.containers.yandexcloud.net/"))
	Expect(spec).To(ContainSubstring("url: https://bba8nrirdcjgkiereo9c.containers.yandexcloud.net/{path}"))
	// `http` rather than `serverless_containers`: the registrar is handed a hostname,
	// not a container id, and the http integration is what forwards the target's Host.
	Expect(spec).To(ContainSubstring("type: http"))
}

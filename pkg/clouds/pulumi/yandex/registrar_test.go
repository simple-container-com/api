// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
	sdkYandex "github.com/simple-container-com/pulumi-yandex/sdk/go/yandex"
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

	spec := proxySpecFor("ycdemo-apigw", "bba8nrirdcjgkiereo9c.containers.yandexcloud.net", false)

	Expect(spec).To(ContainSubstring("\n  /:\n"))
	Expect(spec).To(ContainSubstring("\n  /{path+}:\n"))
	Expect(strings.Count(spec, "x-yc-apigateway-any-method")).To(Equal(2))
	// Sockets are opt-in: a service that did not ask for one must get a spec with no
	// WebSocket operation at all, so no client can hold a 60-minute connection against a
	// handler that would answer a frame with its 404 body.
	Expect(spec).ToNot(ContainSubstring("x-yc-apigateway-websocket"))
	Expect(spec).To(ContainSubstring("url: https://bba8nrirdcjgkiereo9c.containers.yandexcloud.net/"))
	Expect(spec).To(ContainSubstring("url: https://bba8nrirdcjgkiereo9c.containers.yandexcloud.net/{path}"))
	// `http` rather than `serverless_containers`: the registrar is handed a hostname,
	// not a container id, and the http integration is what forwards the target's Host.
	Expect(spec).To(ContainSubstring("type: http"))
}

// TestProxySpecIsValidYamlAndForwardsHeadersAndQuery is the regression test for a gateway
// that answered 200 while forwarding nothing. An API Gateway passes no query string and no
// header but User-Agent unless the spec says otherwise, so the absence of these keys is a
// silent defect: presigned URLs lose their signature, cookie sessions 401, and a
// host-routing service serves its default site under every domain.
//
// The spec is assembled by string formatting, so it is parsed here rather than grepped —
// one wrong indent produces a document YC rejects minutes into a deploy with a generic
// "failed to create API gateway".
func TestProxySpecIsValidYamlAndForwardsHeadersAndQuery(t *testing.T) {
	RegisterTestingT(t)

	const target = "bba8nrirdcjgkiereo9c.containers.yandexcloud.net"
	var doc struct {
		Paths map[string]struct {
			AnyMethod struct {
				Parameters []struct {
					Name string `yaml:"name"`
					In   string `yaml:"in"`
				} `yaml:"parameters"`
				Integration struct {
					Type                     string            `yaml:"type"`
					URL                      string            `yaml:"url"`
					Query                    map[string]string `yaml:"query"`
					Headers                  map[string]string `yaml:"headers"`
					OmitEmptyHeaders         bool              `yaml:"omitEmptyHeaders"`
					OmitEmptyQueryParameters bool              `yaml:"omitEmptyQueryParameters"`
				} `yaml:"x-yc-apigateway-integration"`
			} `yaml:"x-yc-apigateway-any-method"`
		} `yaml:"paths"`
	}
	Expect(yaml.Unmarshal([]byte(proxySpecFor("ycdemo-apigw", target, false)), &doc)).To(Succeed())
	Expect(doc.Paths).To(HaveLen(2))

	for path, expectedURL := range map[string]string{
		"/":        "https://" + target + "/",
		"/{path+}": "https://" + target + "/{path}",
	} {
		p, ok := doc.Paths[path]
		Expect(ok).To(BeTrue(), "path %q missing from the spec", path)
		in := p.AnyMethod.Integration

		Expect(in.Type).To(Equal("http"))
		Expect(in.URL).To(Equal(expectedURL))
		// Everything through, both directions of the request line.
		Expect(in.Query).To(HaveKeyWithValue("*", "*"))
		Expect(in.Headers).To(HaveKeyWithValue("*", "*"))
		// Host pinned to the target: the upstream is reached over TLS by that name, so
		// relaying the visitor's Host would send an SNI its certificate does not cover.
		Expect(in.Headers).To(HaveKeyWithValue("Host", target))
		// ...and the visitor's hostname travels under the name host-routing services read.
		Expect(in.Headers).To(HaveKeyWithValue("X-Forwarded-Host", "{Host}"))
		// Authorization is suppressed, not relayed: a Serverless Container's ingress reads
		// `Authorization: Bearer …` as an IAM token and 403s before the container runs, so
		// relaying it makes every bearer-token caller unreachable. Empty + omitEmptyHeaders
		// drops it; the credential travels under X-Forwarded-Authorization instead.
		Expect(in.Headers).To(HaveKeyWithValue("Authorization", ""))
		Expect(in.Headers).To(HaveKeyWithValue("X-Forwarded-Authorization", "{Authorization}"))
		Expect(in.OmitEmptyHeaders).To(BeTrue())
		Expect(in.OmitEmptyQueryParameters).To(BeTrue())

		// `{Host}` only interpolates if Host is declared as a parameter; undeclared, it
		// arrives at the service as the literal string "{Host}".
		names := map[string]string{}
		for _, prm := range p.AnyMethod.Parameters {
			names[prm.Name] = prm.In
		}
		Expect(names).To(HaveKeyWithValue("Host", "header"))
		Expect(names).To(HaveKeyWithValue("Authorization", "header"))
		if path == "/{path+}" {
			Expect(names).To(HaveKeyWithValue("path", "path"))
		}
	}
}

// TestProxySpecWebSocketOperations covers the only way a service behind a YC Serverless
// Container can stream. Its HTTP path cannot: the runtime rejects a response carrying
// Transfer-Encoding, so it buffers the whole body and sends a Content-Length — measured
// 2026-09-29 as 8.33 s of nothing followed by 8 s worth of SSE events at once, at the
// container URL as well as through a gateway, with `X-Accel-Buffering: no` ignored. Over a
// socket the gateway terminates, the service pushes each delta out of band and the client
// renders it immediately.
//
// All three operations are asserted on BOTH paths: a path with only
// x-yc-apigateway-any-method rejects an upgrade with an instant 405, so a spec that
// declares the socket on `/` alone leaves every real endpoint unable to stream.
func TestProxySpecWebSocketOperations(t *testing.T) {
	RegisterTestingT(t)

	const target = "bba8nrirdcjgkiereo9c.containers.yandexcloud.net"

	type operation struct {
		Parameters []struct {
			Name string `yaml:"name"`
			In   string `yaml:"in"`
		} `yaml:"parameters"`
		Integration struct {
			Type             string            `yaml:"type"`
			URL              string            `yaml:"url"`
			Query            map[string]string `yaml:"query"`
			Headers          map[string]string `yaml:"headers"`
			OmitEmptyHeaders bool              `yaml:"omitEmptyHeaders"`
		} `yaml:"x-yc-apigateway-integration"`
	}
	var doc struct {
		Paths map[string]struct {
			AnyMethod  operation `yaml:"x-yc-apigateway-any-method"`
			Connect    operation `yaml:"x-yc-apigateway-websocket-connect"`
			Message    operation `yaml:"x-yc-apigateway-websocket-message"`
			Disconnect operation `yaml:"x-yc-apigateway-websocket-disconnect"`
		} `yaml:"paths"`
	}
	spec := proxySpecFor("ycdemo-apigw", target, true)
	Expect(yaml.Unmarshal([]byte(spec), &doc)).To(Succeed())
	Expect(doc.Paths).To(HaveLen(2))

	for path, expectedURL := range map[string]string{
		"/":        "https://" + target + "/",
		"/{path+}": "https://" + target + "/{path}",
	} {
		p, ok := doc.Paths[path]
		Expect(ok).To(BeTrue(), "path %q missing from the spec", path)
		// The HTTP operation stays — the same host serves ordinary requests and sockets.
		Expect(p.AnyMethod.Integration.URL).To(Equal(expectedURL))

		for name, op := range map[string]operation{"connect": p.Connect, "message": p.Message, "disconnect": p.Disconnect} {
			in := op.Integration
			Expect(in.Type).To(Equal("http"), "%s on %q", name, path)
			Expect(in.URL).To(Equal(expectedURL), "%s on %q", name, path)
			// Relaying every header is what makes a cookie session work over the socket:
			// Cookie arrives verbatim on CONNECT and on MESSAGE, so the handshake
			// authenticates the same way a request does and needs no second mechanism.
			Expect(in.Headers).To(HaveKeyWithValue("*", "*"), "%s on %q", name, path)
			Expect(in.Headers).To(HaveKeyWithValue("Host", target), "%s on %q", name, path)
			Expect(in.Headers).To(HaveKeyWithValue("X-Forwarded-Host", "{Host}"), "%s on %q", name, path)
			Expect(in.OmitEmptyHeaders).To(BeTrue(), "%s on %q", name, path)

			// Same rule as the HTTP path: an undeclared parameter renders as literal
			// braces, which on the greedy path means requesting the URL "/{path}".
			names := map[string]string{}
			for _, prm := range op.Parameters {
				names[prm.Name] = prm.In
			}
			Expect(names).To(HaveKeyWithValue("Host", "header"), "%s on %q", name, path)
			if path == "/{path+}" {
				Expect(names).To(HaveKeyWithValue("path", "path"), "%s on %q", name, path)
			}
		}
	}

	// Turning the flag on may only ADD. Every line of the plain spec must still be present,
	// in order — an indent or ordering slip on the HTTP half would otherwise reach a deploy,
	// where YC reports it minutes in as a generic "failed to create API gateway".
	plain := strings.Split(proxySpecFor("ycdemo-apigw", target, false), "\n")
	rest := spec
	for _, line := range plain {
		idx := strings.Index(rest, line+"\n")
		Expect(idx).To(BeNumerically(">=", 0), "websocket spec dropped or reordered line %q", line)
		rest = rest[idx+len(line)+1:]
	}
}

// TestHostnameRecordTypeUsesAnameAtApex pins the apex rule. A CNAME at a zone apex is
// illegal and YC has no ALIAS, so a service whose `domain:` IS the zone — the normal shape
// for a product's own front door — cannot be published with a CNAME at all. YC's answer is
// ANAME, resolved server-side and answered as an A.
func TestHostnameRecordTypeUsesAnameAtApex(t *testing.T) {
	RegisterTestingT(t)

	Expect(hostnameRecordType("atriumdev.ru", "atriumdev.ru")).To(Equal("ANAME"))
	// Trailing dots and case are how the same name arrives from a zone lookup vs a
	// client stack's `domain:`, and they must not decide the record type.
	Expect(hostnameRecordType("atriumdev.ru.", "atriumdev.ru")).To(Equal("ANAME"))
	Expect(hostnameRecordType("AtriumDev.ru", "atriumdev.ru.")).To(Equal("ANAME"))

	Expect(hostnameRecordType("www.atriumdev.ru", "atriumdev.ru")).To(Equal("CNAME"))
	Expect(hostnameRecordType("a.b.atriumdev.ru", "atriumdev.ru")).To(Equal("CNAME"))
	// Not in the zone at all: the caller rejects that earlier, and a bad match here must
	// not silently become an apex record.
	Expect(hostnameRecordType("notatriumdev.ru", "atriumdev.ru")).To(Equal("CNAME"))
}

// TestZoneIDOf covers the field the lookup actually fills. `DnsZoneId` echoes the
// argument, so a lookup by name leaves it empty and the id arrives in `Id` — reading
// only the first yields an empty ZoneId, which YC rejects generically several minutes
// into a deploy.
func TestZoneIDOf(t *testing.T) {
	RegisterTestingT(t)

	cfg := &yandex.RegistrarConfig{ZoneName: "simple-forge.ru"}

	byName := &sdkYandex.LookupDnsZoneResult{Name: "simple-forge-ru", Id: "dns8cs728kosp7ms1s3u"}
	id, err := zoneIDOf(byName, cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(id).To(Equal("dns8cs728kosp7ms1s3u"))

	byID := &sdkYandex.LookupDnsZoneResult{Name: "simple-forge-ru", DnsZoneId: "dns8cs728kosp7ms1s3u"}
	id, err = zoneIDOf(byID, cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(id).To(Equal("dns8cs728kosp7ms1s3u"))

	_, err = zoneIDOf(&sdkYandex.LookupDnsZoneResult{Name: "simple-forge-ru"}, cfg)
	Expect(err).To(MatchError(ContainSubstring("resolved without an id")))
}

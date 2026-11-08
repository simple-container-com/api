// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"github.com/pkg/errors"
)

// TlsConfig is the TLS vocabulary an edge-facing template or resource exposes in
// server.yaml. Embedded inline, so the yaml shape stays flat and every template that
// embeds it spells the switches the same way.
//
// There are two separate decisions here and `useSSL` named neither of them:
//
//   - Does the edge TERMINATE TLS for the sites it serves? That decides whether the
//     generated Caddy site address is `https://<domain>` or `http://<domain>`, and the
//     scheme is not decoration — `https://` is what arms Caddy's automatic HTTPS and
//     therefore ACME. Until now this was spelled only on the client side, as each
//     stack's `lbConfig.https`, with no way to state it once for the edge.
//     Here: `originTls`.
//
//   - Given that clients reach us over HTTPS (whoever terminated it), do we send HSTS
//     and does Caddy own the HTTP→HTTPS redirect? That is all `useSSL` ever did: one
//     `import hsts` in the site entry and the `ingress.kubernetes.io/ssl-redirect:
//     false` annotation. Here: `clientTls`.
//
// `useSSL` is kept working as the deprecated spelling of `clientTls` because stacks in
// the fleet set it today (forge-baas and forge-atrium both do) and a rename that breaks
// a deployed server.yaml is not a naming fix, it is an outage. A contradiction between
// the two spellings is refused rather than resolved silently.
//
// The vm path already calls the first decision `ingress.originTls`
// (pkg/clouds/vm/vm.go:142) and this aligns the two front doors on one word for one
// decision. One difference survives on purpose and is stated on the field: the vm
// resource's `originTls` defaults ON, because a vm's Caddy IS the front door, whereas
// here it defaults OFF, because the fleet's normal arrangement is Cloudflare (or a
// cloud load balancer) terminating in front.
type TlsConfig struct {
	// UseSSL is the DEPRECATED spelling of ClientTls and still honoured. It reads
	// like "serve TLS" and never did that; see ClientTls for what it actually does.
	UseSSL *bool `json:"useSSL,omitempty" yaml:"useSSL,omitempty"`

	// ClientTls is whether connections from clients are HTTPS, whoever terminated
	// them. It adds `import hsts` to the generated site entry — the shared Caddy's
	// `(hsts)` snippet sets Strict-Transport-Security and redirects HTTP→HTTPS on
	// `X-Forwarded-Proto: http` — and sets `ingress.kubernetes.io/ssl-redirect:
	// false` on the Ingress, because with that snippet in place Caddy owns the
	// redirect and the ingress must not also do it.
	//
	// It does NOT make this edge serve TLS. `originTls` does that.
	//
	// Unset means true, which is both the historical `useSSL` default and the right
	// one: the fleet's edges sit behind TLS either way, and HSTS received over the
	// edge's TLS is honoured by the browser even when the origin hop is cleartext.
	ClientTls *bool `json:"clientTls,omitempty" yaml:"clientTls,omitempty"`

	// OriginTls is the edge-wide default for whether this edge terminates TLS
	// itself: `https://<domain>` as the site address, automatic HTTPS armed, a
	// certificate obtained over ACME. Each stack can still override it with
	// `lbConfig.originTls` in its own client.yaml, and the deprecated
	// `lbConfig.https: true` keeps meaning the same thing.
	//
	// Unset means "no edge-wide default", which leaves the decision entirely to
	// each stack's lbConfig and so preserves today's behaviour exactly — including
	// its default of plain HTTP for a stack that says nothing. Setting it true is
	// how an edge that really is the front door says so once instead of once per
	// stack; setting it false pins the whole edge to plain HTTP regardless of what
	// a stack asks for.
	//
	// Not to be confused with the vm resource's `ingress.originTls`, which answers
	// the same question for a machine's Caddy but defaults ON — a vm's Caddy is the
	// front door, this one usually is not.
	OriginTls *bool `json:"originTls,omitempty" yaml:"originTls,omitempty"`
}

// ClientTlsEnabled reports whether to send HSTS and let Caddy own the redirect.
//
// Precedence: `clientTls`, then the deprecated `useSSL`, then true. A nil receiver
// answers the same as an empty one — a template that embeds this and sets nothing is
// the common case.
func (t *TlsConfig) ClientTlsEnabled() bool {
	if t == nil {
		return true
	}
	if t.ClientTls != nil {
		return *t.ClientTls
	}
	if t.UseSSL != nil {
		return *t.UseSSL
	}
	return true
}

// OriginTlsDefault is the edge-wide origin-TLS default to hand to each stack's
// lbConfig resolution, or nil when the edge states none.
func (t *TlsConfig) OriginTlsDefault() *bool {
	if t == nil {
		return nil
	}
	return t.OriginTls
}

// ValidateTls refuses the one combination that cannot be honoured both ways. Everything
// else resolves by the documented precedence.
func (t *TlsConfig) ValidateTls() error {
	if t == nil || t.ClientTls == nil || t.UseSSL == nil {
		return nil
	}
	if *t.ClientTls != *t.UseSSL {
		return errors.Errorf("`clientTls: %t` contradicts `useSSL: %t`: `useSSL` is the deprecated "+
			"spelling of `clientTls` and they are the same switch — drop `useSSL` and keep `clientTls`",
			*t.ClientTls, *t.UseSSL)
	}
	return nil
}

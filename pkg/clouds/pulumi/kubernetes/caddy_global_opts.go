// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// staticCaddyfilePrefix assembles the CADDYFILE_PREFIX for an edge whose cloud did not
// compute one of its own (see CaddyfilePrefixOut) — which is every cloud but GKE.
//
// Reads the config through one nil-safe copy: CaddyDeployment embeds *k8s.CaddyConfig,
// so the promoted `caddy.CaddyfilePrefix` dereferences a pointer that several callers
// leave nil.
//
// `namespace` and `deploymentName` reach the certificate store's block, where they name
// the log to read if certificates never issue.
func staticCaddyfilePrefix(caddy CaddyDeployment, namespace, deploymentName string) (string, error) {
	cfg := lo.FromPtrOr(caddy.CaddyConfig, k8s.CaddyConfig{})
	storageBlock, err := caddyAcmeStorageBlock(caddy, namespace, deploymentName)
	if err != nil {
		return "", err
	}
	trustedBlock, err := BuildTrustedProxiesBlock(cfg)
	if err != nil {
		return "", err
	}
	return BuildCaddyfileGlobalOptions(storageBlock, trustedBlock, lo.FromPtrOr(cfg.CaddyfilePrefix, ""))
}

// BuildTrustedProxiesBlock builds the Caddy servers { trusted_proxies ... } block
// from the CaddyConfig. Returns empty string if no trusted proxies are configured.
func BuildTrustedProxiesBlock(cfg k8s.CaddyConfig) (string, error) {
	if len(cfg.TrustedProxies) == 0 {
		return "", nil
	}

	for _, cidr := range cfg.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			if net.ParseIP(cidr) == nil {
				return "", errors.Errorf("invalid trusted proxy entry %q: must be a valid CIDR or IP address", cidr)
			}
		}
	}

	cidrs := strings.Join(cfg.TrustedProxies, " ")
	return fmt.Sprintf("  servers {\n    trusted_proxies static %s\n  }", cidrs), nil
}

// BuildCaddyfileGlobalOptions assembles everything that goes above the first site block:
// SC's own global options (the certificate store, trusted proxies) and whatever the
// operator put in `caddy.caddyfilePrefix`.
//
// A CADDYFILE ACCEPTS EXACTLY ONE KEYLESS BLOCK, AND IT MUST BE FIRST. Caddy answers a
// second one with `server block without any key is global configuration, and if used, it
// must be first` — a config-load failure, which on a deployed edge is a container that
// never becomes ready and a Deployment that rolls back into a crash loop. So the
// operator's prefix cannot simply be appended: `caddyfilePrefix` is documented as
// "content to inject at the top of the Caddyfile (e.g. storage configuration)", and
// storage configuration is a global option, so the natural thing to write there is a
// `{ ... }` block. Its contents are merged into SC's block instead; anything after it
// (snippet definitions, site blocks) is emitted unchanged below.
//
// Returns an error when the prefix's leading block has no closing brace, rather than
// emitting it and letting the edge fail to start — see SplitCaddyfilePrefix.
func BuildCaddyfileGlobalOptions(storageBlock, trustedProxiesBlock, userPrefix string) (string, error) {
	parts, err := SplitCaddyfilePrefix(userPrefix)
	if err != nil {
		return "", err
	}

	var globalOpts []string
	for _, block := range []string{storageBlock, trustedProxiesBlock, parts.Globals} {
		if strings.TrimSpace(block) != "" {
			globalOpts = append(globalOpts, strings.TrimRight(block, "\n"))
		}
	}

	var chunks []string
	if parts.Comments != "" {
		// Kept above the block, where the operator wrote them. Caddy's lexer strips
		// comments, so they do not make the global block "not first".
		chunks = append(chunks, parts.Comments)
	}
	if len(globalOpts) > 0 {
		chunks = append(chunks, fmt.Sprintf("{\n%s\n}", strings.Join(globalOpts, "\n")))
	}
	if parts.Rest != "" {
		chunks = append(chunks, parts.Rest)
	}
	return strings.Join(chunks, "\n\n"), nil
}

// CaddyfilePrefixParts is an operator-supplied `caddyfilePrefix` taken apart far enough
// to merge its global options into SC's own.
type CaddyfilePrefixParts struct {
	// Comments are the full-line comments the prefix opens with, verbatim and without
	// the trailing newline. Separated out only so that a prefix which opens with a
	// comment ABOVE its global block is still recognised as having one — the comment
	// itself is re-emitted where it was.
	Comments string
	// Globals is the CONTENTS of the prefix's leading keyless block, braces removed,
	// ready to be joined with SC's other global options inside one `{ ... }`. Empty when
	// the prefix has no leading keyless block.
	Globals string
	// Rest is everything after that block (or the whole prefix, when there was none),
	// emitted unchanged after the global block. Snippet definitions and site blocks live
	// here, and must NOT be merged into the global options.
	Rest string
}

// SplitCaddyfilePrefix separates a `caddyfilePrefix`'s leading keyless block from the
// rest of it. Exported so a caller can refuse an unparseable prefix early, before a
// Pulumi apply, where the error reaches the operator as a sentence rather than wrapped in
// a stack trace.
//
// "Leading" is the whole test. A `{` that opens the prefix's first directive is the
// global options block; `(snippet) { ... }`, `example.com { ... }` and a bare directive
// are not, and are returned in Rest untouched.
//
// This is a split, not a parse: SC needs to know where the operator's global block ends
// and nothing about what is inside it. A directive SC does not know stays a directive SC
// does not know, which is the point of the field.
func SplitCaddyfilePrefix(prefix string) (CaddyfilePrefixParts, error) {
	comments, body := splitLeadingComments(prefix)
	if !strings.HasPrefix(body, "{") {
		// No leading global block. The comments belong to whatever follows, so put them
		// back — splitting them off was only a way of looking past them.
		return CaddyfilePrefixParts{Rest: strings.TrimRight(joinNonEmpty(comments, body, "\n"), "\n")}, nil
	}

	end, err := matchingBraceIndex(body)
	if err != nil {
		return CaddyfilePrefixParts{}, errors.Wrapf(err,
			"`caddy.caddyfilePrefix` opens a block that is never closed. It is injected at the top of the "+
				"generated Caddyfile, so an unbalanced brace there swallows everything SC renders below it")
	}

	return CaddyfilePrefixParts{
		Comments: comments,
		Globals:  strings.Trim(body[1:end], "\n"),
		Rest:     strings.TrimSpace(body[end+1:]),
	}, nil
}

// splitLeadingComments returns the run of blank and full-line-comment lines the input
// opens with (trailing newline trimmed), and the remainder with leading whitespace
// removed.
func splitLeadingComments(s string) (comments, rest string) {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return strings.TrimRight(strings.Join(lines[:i], "\n"), "\n"), strings.TrimLeft(strings.Join(lines[i:], "\n"), " \t")
	}
	// Nothing but comments and blank lines.
	return "", strings.TrimSpace(s)
}

// matchingBraceIndex returns the index of the `}` that closes the `{` at s[0].
//
// Brace-counting, aware of the two places a brace is not structure: inside a
// double-quoted token (`header X "a{b}"`) and after a `#` comment marker, which Caddy's
// lexer recognises at the start of a line or of a token — a `#` mid-token is literal
// (`header X a#b`), and treating it as a comment would make this function read past the
// real closing brace.
func matchingBraceIndex(s string) (int, error) {
	depth, inQuote, inComment := 0, false, false
	atTokenStart := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inComment:
			if c == '\n' {
				inComment = false
			}
		case inQuote:
			switch c {
			case '\\':
				i++ // escaped: whatever follows is literal, including a quote
			case '"':
				inQuote = false
			}
		case c == '"':
			inQuote = true
		case c == '#' && atTokenStart:
			inComment = true
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
		if !inComment && !inQuote {
			atTokenStart = c == '\n' || c == ' ' || c == '\t'
		}
	}
	return 0, errors.Errorf("no `}` closes the `{` this block opens with")
}

func joinNonEmpty(a, b, sep string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + sep + b
}

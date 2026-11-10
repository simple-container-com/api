// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package inlinetag_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/simple-container-com/api/pkg/api/inlinetag"
)

// known is every named-inline field that exists today, with the reason it is
// not simply fixed here.
//
// All four want the same fix — embed the field — and embedding is source-
// compatible for three of them, because Go names an embedded field after its
// type (`Config Config` -> embedded `Config` keeps every `.Config` reference
// working). It is NOT a free change, which is why they are listed rather than
// fixed: embedding flips the JSON wire from `{"Site":{"domain":…}}` to flat
// `{"domain":…}`, and a plugin whose pinned api version still declares the
// field NAMED decodes that flat object into the zero value. Silently.
//
// So each of these comes out in a PR coordinated with the plugin repos that
// carry a `jsonRoundTrip` stopgap: api embeds, each plugin bumps its api
// dependency and drops the stopgap in the same release window. Until then the
// lint's job is to stop a FIFTH one appearing, which is the failure mode that
// actually hurt — seven production failures across the fleet on 2026-11-09,
// none of them announced by anything.
//
// Removing an entry is the point. Adding one needs a very good reason in the
// PR description.
var known = map[string]string{
	"StackClientDescriptor.Config": "pkg/api/client.go — embed is source-compatible; wire change needs the plugin bump",
	"StackConfigStatic.Site":       "pkg/api/client.go — THE one that caused the 2026-11-09 outages; 3 `.Site.` references in api, trivial edit, lockstep release",
	"StackParams.Timeouts":         "pkg/api/client.go — CLI params, not bus payload; embed is source-compatible",
	"ProvisionParams.Timeouts":     "pkg/api/server.go — CLI params, not bus payload; embed is source-compatible",
}

// moduleRoot walks up from this test's directory to the module root, so the
// scan covers the whole module however the test is invoked.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test's working directory")
		}
		dir = parent
	}
}

// No NEW named-inline field may appear. See the package doc for what the shape
// does and the `known` map for why the current four are not fixed here.
func TestNoNewNamedInlineFields(t *testing.T) {
	findings, err := inlinetag.Scan(moduleRoot(t))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	seen := map[string]bool{}
	for _, f := range findings {
		key := f.Struct + "." + f.Field
		seen[key] = true
		if _, ok := known[key]; !ok {
			t.Errorf(`NEW named-inline field — this marshals NESTED as JSON and FLAT as YAML, and the plugin bus does one of each, so the value silently arrives as the zero value:
  %s
Fix: make the field EMBEDDED (drop the field name). encoding/json promotes an embedded struct's fields with no tag help, yaml.v3 honours the ,inline, and the YAML an operator writes does not change.
If it genuinely cannot be embedded, add it to the 'known' map in this file WITH the reason, and raise it in the PR.`, f)
		}
	}

	// A `known` entry that no longer matches anything is a fix that landed —
	// delete the entry so the list keeps meaning something.
	for key, why := range known {
		if !seen[key] {
			t.Errorf("the 'known' map lists %s (%s) but the scan no longer finds it — the fix landed; delete the entry", key, why)
		}
	}
}

// The detector must not flag the CORRECT shape, or nobody will believe it.
func TestScan_DoesNotFlagEmbeddedOrRenamedFields(t *testing.T) {
	dir := t.TempDir()
	src := `package p

type Inner struct{ A string }

type Embedded struct {
	Inner ` + "`" + `json:",inline" yaml:",inline"` + "`" + `
}

type Renamed struct {
	Inner Inner ` + "`" + `json:"inner,inline" yaml:",inline"` + "`" + `
}

type NoTag struct {
	Inner Inner
}

type OtherOption struct {
	Inner Inner ` + "`" + `json:",omitempty"` + "`" + `
}

type Offender struct {
	Named Inner ` + "`" + `json:",inline" yaml:",inline"` + "`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	findings, err := inlinetag.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly the one offender, got %d: %v", len(findings), findings)
	}
	if findings[0].Struct != "Offender" || findings[0].Field != "Named" {
		t.Fatalf("flagged the wrong field: %+v", findings[0])
	}
}

// _test.go files are skipped: a test fixture may deliberately construct the
// broken shape (this file's own sibling test does).
func TestScan_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	src := `package p

type Inner struct{ A string }

type InATestFile struct {
	Named Inner ` + "`" + `json:",inline"` + "`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "p_test.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err := inlinetag.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a _test.go file must be skipped, got %v", findings)
	}
}

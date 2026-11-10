// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package inlinetag detects struct fields that are tagged `json:",inline"`
// while being NAMED rather than embedded — a shape that means two different
// things to the two codecs this repo's config travels through, and the single
// root cause behind seven production failures across the fleet on 2026-11-09.
//
// What goes wrong. yaml.v3 implements `,inline` and flattens the field's own
// fields into the parent. `encoding/json` has no `,inline` at all: on a NAMED
// field it reads the tag as "no rename, plus an option I do not recognise" and
// keeps the Go field name, so the value nests under that name — capitalised,
// because that is the Go identifier. The same struct therefore marshals flat as
// YAML and nested as JSON.
//
// Why that is fatal here specifically. SC's plugin bus marshals a config to
// JSON on the way out (`plugin.PlainToInputs`) and yaml-round-trips it on the
// way in (`plugin/runtime.DecodeInputs`). A named inline field is written
// nested and read as if flat, so it silently arrives as the zero value — no
// error, no warning, a deploy that goes green with an empty domain.
//
// An EMBEDDED field with the same tag is fine, and is the fix: `encoding/json`
// promotes an embedded struct's fields with no tag help at all, and yaml.v3
// honours the `,inline`. Both go flat, the YAML an operator writes is unchanged,
// and the two codecs agree.
//
// See forge/docs/roadmap/items/plugin-bus-json-yaml-inline-tag-asymmetry.md.
package inlinetag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
)

// Finding is one offending field.
type Finding struct {
	File   string
	Line   int
	Struct string
	Field  string
}

func (f Finding) String() string {
	return f.File + ":" + itoa(f.Line) + ": " + f.Struct + "." + f.Field +
		` is a NAMED field tagged json:",inline" — embed it instead (see package inlinetag)`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// Scan walks every non-test Go file under root and returns the offending
// fields, sorted by file then line.
//
// Deliberately AST-based rather than reflection-based: a reflection walk needs
// a root set of types to start from, and the fields that caused the outages
// were reachable only through registries populated at parse time. The AST sees
// every struct in the module whether or not anything has registered it yet.
func Scan(root string) ([]Finding, error) {
	var out []Finding
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Vendored and generated trees are not ours to fix.
			switch d.Name() {
			case "vendor", "testdata", "node_modules", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			// A file this package cannot parse is not evidence of a finding.
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				// No names means embedded, which is the correct shape.
				if len(field.Names) == 0 || field.Tag == nil {
					continue
				}
				tag := strings.Trim(field.Tag.Value, "`")
				if !hasInlineJSON(tag) {
					continue
				}
				for _, name := range field.Names {
					out = append(out, Finding{
						File:   path,
						Line:   fset.Position(name.Pos()).Line,
						Struct: spec.Name.Name,
						Field:  name.Name,
					})
				}
			}
			return true
		})
		return nil
	})
	return out, err
}

// hasInlineJSON reports whether the struct tag's json entry is exactly the
// `,inline` option — the shape encoding/json ignores. A tag that renames the
// field (`json:"site,inline"`) is not this bug: the rename takes effect and the
// value lands under a key the author chose.
func hasInlineJSON(tag string) bool {
	value, ok := reflect.StructTag(tag).Lookup("json")
	if !ok {
		return false
	}
	parts := strings.Split(value, ",")
	if parts[0] != "" {
		return false
	}
	for _, opt := range parts[1:] {
		if opt == "inline" {
			return true
		}
	}
	return false
}

// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// runtimeImageFields are the struct fields that decide which artifact actually
// runs. Feeding any of them a tag reopens the gap this package closed: the
// digest is what gets scanned, signed and verified, so a task definition or
// function that carries a tag runs whatever the tag points at by pull time.
var runtimeImageFields = map[string]bool{
	"Image":    true, // ecs.TaskDefinitionContainerDefinitionArgs
	"ImageUri": true, // lambda.FunctionArgs
}

// permittedRuntimeImageExprs is an exact allowlist rather than a substring
// match. A substring passes anything that merely mentions the field, including
// a helper that returns the tag and a literal string containing the name.
var permittedRuntimeImageExprs = map[string]bool{
	"image.DeployImageRef": true,
	"image.deployImageRef": true,
}

// allowedTagReferences are the runtime image references deliberately left on a
// tag, keyed by the expression as it appears in the source.
//
// The cloud-helpers image IS built and pushed here (alerts.go), so a digest
// does exist for it; it is simply not wired through resolveDeployImageRef and
// nothing signs it. TODO: point alerts.go at cfg.helpersImage.RepoDigest and
// delete this entry.
var allowedTagReferences = map[string]bool{
	"cfg.helpersImage.ImageName": true,
}

func TestRuntimeImageReferencesUseTheDigest(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	// Counted per field and excluding the allowlist: counting every occurrence
	// would let the allowlisted helpers entry alone keep the anti-vacuity guard
	// happy while both real assignments disappeared.
	seen := map[string]int{}

	check := func(src []byte, fieldName string, value ast.Expr, pos token.Pos) {
		start := fset.Position(value.Pos()).Offset
		end := fset.Position(value.End()).Offset
		expr := strings.TrimSpace(string(src[start:end]))
		if allowedTagReferences[expr] {
			return
		}
		seen[fieldName]++
		if permittedRuntimeImageExprs[expr] {
			return
		}
		p := fset.Position(pos)
		t.Errorf("%s:%d: %s is fed %q; a runtime image reference must be the digest "+
			"(one of the permittedRuntimeImageExprs), or be added to allowedTagReferences with a reason",
			p.Filename, p.Line, fieldName, expr)
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.KeyValueExpr:
				key, ok := node.Key.(*ast.Ident)
				if !ok || !runtimeImageFields[key.Name] {
					return true
				}
				check(src, key.Name, node.Value, node.Pos())
			case *ast.AssignStmt:
				// A composite literal is not the only way in: `cDef.Image = x`
				// after the fact is invisible to a KeyValueExpr-only walk.
				for i, lhs := range node.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || !runtimeImageFields[sel.Sel.Name] || i >= len(node.Rhs) {
						continue
					}
					check(src, sel.Sel.Name, node.Rhs[i], node.Pos())
				}
			}
			return true
		})
	}

	// A refactor that renames the fields would otherwise turn this into a test
	// that passes by checking nothing.
	for field := range runtimeImageFields {
		if seen[field] == 0 {
			t.Fatalf("found no non-allowlisted %s assignment; runtimeImageFields is stale", field)
		}
	}
}

// The consumer-side check above cannot see a producer that leaves the field
// unset: a zero sdk.StringOutput resolves as unknown, which fails the task
// definition at apply while reading as correct in the source. The prebuilt-image
// branch shipped exactly that.
func TestEveryECRImageSetsDeployImageRef(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	var found int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			ident, ok := lit.Type.(*ast.Ident)
			if !ok || ident.Name != "ECRImage" {
				return true
			}
			found++
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "DeployImageRef" {
					return true
				}
			}
			p := fset.Position(lit.Pos())
			t.Errorf("%s:%d: this ECRImage leaves DeployImageRef unset; "+
				"a zero StringOutput resolves as unknown and fails the task definition",
				p.Filename, p.Line)
			return true
		})
	}

	if found == 0 {
		t.Fatal("found no ECRImage literals; this test no longer checks anything")
	}
}

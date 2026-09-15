// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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

// allowedTagReferences are the runtime image references deliberately left on a
// tag, keyed by the expression as it appears in the source. The helpers image is
// a mirrored third-party image built outside BuildAndPushImage, so no digest is
// produced for it and nothing signs it.
var allowedTagReferences = map[string]bool{
	"cfg.helpersImage.ImageName": true,
}

func TestRuntimeImageReferencesUseTheDigest(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	var checked int
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
			kv, ok := n.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || !runtimeImageFields[key.Name] {
				return true
			}
			start := fset.Position(kv.Value.Pos()).Offset
			end := fset.Position(kv.Value.End()).Offset
			expr := strings.TrimSpace(string(src[start:end]))
			checked++
			if strings.Contains(strings.ToLower(expr), "deployimageref") || allowedTagReferences[expr] {
				return true
			}
			pos := fset.Position(kv.Pos())
			t.Errorf("%s:%d: %s is fed %q; a runtime image reference must be the digest "+
				"(DeployImageRef), or be added to allowedTagReferences with a reason",
				filepath.Base(pos.Filename), pos.Line, key.Name, expr)
			return true
		})
	}

	// A refactor that renames the fields would otherwise turn this into a test
	// that passes by checking nothing.
	if checked == 0 {
		t.Fatal("found no runtime image assignments to check; the field names in runtimeImageFields are stale")
	}
}

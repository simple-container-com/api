// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// kubernetesPkgPath is the import path of *this* package. The AST regression
// guard below resolves whichever local name a consumer file imports it under
// (default `kubernetes`, or any alias like `k8s "<path>"`) so the strict
// selector check also catches `<alias>.GenerateNamespaceName(...)` and not
// only the un-aliased form.
const kubernetesPkgPath = "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"

// kubernetesPkgAliases parses the file's import block and returns the set of
// local identifiers that bind to kubernetesPkgPath. Same-package files (no
// import of self) return an empty set — the caller falls back to matching the
// bare-identifier `GenerateNamespaceName(...)` form for those.
func kubernetesPkgAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != kubernetesPkgPath {
			continue
		}
		name := "kubernetes"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = true
	}
	return aliases
}

// TestDownstreamCallSitesDoNotRecomputeNamespace is a regression guard for #258.
//
// Before #258, four downstream Secret/Job call sites independently re-derived their
// k8s namespace via kubernetes.GenerateNamespaceName(stackName, stackEnv, parentEnv).
// That recomputation drifted from the live Namespace resource's metadata.Name on any
// migrated stack (where #255's IgnoreChanges keeps the Namespace in parent-shared
// state), scheduled an immutable-namespace Pulumi Replace, and failed because the
// isolated namespace doesn't exist on the cluster.
//
// #258 replaced the recomputation with Output-threading: NewSimpleContainer sets
// SimpleContainerArgs.NamespaceNameOutput from namespace.Metadata.Name().Elem()
// before RunPreProcessors fires, and the four downstream consumers now read that
// Output (preprocessor path) or sc.Namespace (postprocessor path) instead.
//
// This test fails if a future change reintroduces GenerateNamespaceName inside any
// of those consumer functions. If the test breaks, do NOT call GenerateNamespaceName
// from the listed functions — thread the namespace Output through
// SimpleContainerArgs / SimpleContainer instead. The recomputation hazard is
// documented in #258's commit history and in the long comment in simple_container.go
// next to the Namespace resource.
func TestDownstreamCallSitesDoNotRecomputeNamespace(t *testing.T) {
	type site struct {
		file      string
		forbidden []string
		reason    string
	}

	sites := []site{
		{
			file:      filepath.Join("..", "gcp", "compute_proc.go"),
			forbidden: []string{"createCloudsqlProxy", "createUserForDatabase"},
			reason: "GCP CSQL sidecar/init proxy consumers must consume the namespace Output threaded " +
				"via kubernetes.SimpleContainerArgs.NamespaceNameOutput (preprocessor path) or " +
				"SimpleContainer.Namespace (postprocessor path), not recompute it.",
		},
		{
			file:      "compute_proc_postgres.go",
			forbidden: []string{"createPostgresUserForDatabase"},
			reason: "On-cluster postgres init Job consumer must consume the namespace Output threaded " +
				"via SimpleContainerArgs.NamespaceNameOutput, not recompute it.",
		},
		{
			file:      "compute_proc_mongodb.go",
			forbidden: []string{"createMongodbUserForDatabase"},
			reason: "On-cluster mongodb init Job consumer must consume the namespace Output threaded " +
				"via SimpleContainerArgs.NamespaceNameOutput, not recompute it.",
		},
	}

	for _, s := range sites {
		s := s
		t.Run(filepath.Base(s.file), func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, s.file, nil, parser.SkipObjectResolution|parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse imports of %s: %v", s.file, err)
			}
			aliases := kubernetesPkgAliases(file)
			file, err = parser.ParseFile(fset, s.file, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse %s: %v", s.file, err)
			}

			forbidden := make(map[string]struct{}, len(s.forbidden))
			for _, fn := range s.forbidden {
				forbidden[fn] = struct{}{}
			}
			covered := make(map[string]bool, len(s.forbidden))

			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				if _, watched := forbidden[fn.Name.Name]; !watched {
					continue
				}
				covered[fn.Name.Name] = true

				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					// Match only the package-level function in the `kubernetes` pkg, not arbitrary
					// methods that happen to share the name (e.g. mockClient.GenerateNamespaceName()).
					// Two valid call shapes:
					//   - bare identifier:        GenerateNamespaceName(...)            // same-package
					//   - qualified selector:     <alias>.GenerateNamespaceName(...)    // cross-pkg
					// where <alias> is resolved from the file's import block via kubernetesPkgAliases.
					// Default name is `kubernetes` when no alias is set; the test still flags imports
					// renamed as e.g. `k8s "<path>"`.
					switch fun := call.Fun.(type) {
					case *ast.Ident:
						if fun.Name != "GenerateNamespaceName" {
							return true
						}
					case *ast.SelectorExpr:
						if fun.Sel.Name != "GenerateNamespaceName" {
							return true
						}
						pkg, ok := fun.X.(*ast.Ident)
						if !ok || !aliases[pkg.Name] {
							return true
						}
					default:
						return true
					}
					t.Errorf(
						"%s:%d: %s() calls kubernetes.GenerateNamespaceName — see PR #258. %s",
						s.file, fset.Position(call.Pos()).Line, fn.Name.Name, s.reason,
					)
					return true
				})
			}

			for _, want := range s.forbidden {
				if !covered[want] {
					t.Errorf("function %s not found in %s — was it renamed or removed? Update this test.", want, s.file)
				}
			}
		})
	}
}

// TestSimpleContainerArgsCarriesNamespaceNameOutput is a compile-time-style guard
// that SimpleContainerArgs exposes the NamespaceNameOutput field. The four
// downstream consumers (see TestDownstreamCallSitesDoNotRecomputeNamespace) read
// namespaces from this field; removing or renaming it silently regresses #258. If
// this test fails, preserve the field name (and the assignment in
// NewSimpleContainer) or update the four consumer call sites in lock-step.
func TestSimpleContainerArgsCarriesNamespaceNameOutput(t *testing.T) {
	var args SimpleContainerArgs
	_ = args.NamespaceNameOutput // compile-time field presence check
}

package kubernetes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

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
			file, err := parser.ParseFile(fset, s.file, nil, parser.SkipObjectResolution)
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
					var ident *ast.Ident
					switch fun := call.Fun.(type) {
					case *ast.Ident:
						ident = fun
					case *ast.SelectorExpr:
						ident = fun.Sel
					}
					if ident != nil && ident.Name == "GenerateNamespaceName" {
						t.Errorf(
							"%s:%d: %s() calls GenerateNamespaceName — see PR #258. %s",
							s.file, fset.Position(call.Pos()).Line, fn.Name.Name, s.reason,
						)
					}
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
